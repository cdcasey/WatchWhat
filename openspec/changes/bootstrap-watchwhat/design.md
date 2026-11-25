# Design

## Context

Greenfield. See proposal.md for scope and specs/ for behavior. Hard constraints:

- One static binary, no CGO, no Node at build or runtime.
- One container on a home server, reached only over Tailscale. No auth layer.
- Two people reading and writing the same data. Concurrency is near zero but not zero (two phones tapping at once).

## Goals / Non-Goals

**Goals:**
- Lay out a codebase where each spec capability maps to one package plus a set of templ components.
- Keep business rules (next episode, stale, caught up) in pure Go functions with an injected clock, so they are unit-testable without HTTP or SQLite.
- Keep HTMX usage to a small, consistent set of patterns.

**Non-Goals:**
- Horizontal scaling, multiple instances, or external DB.
- Real-time push between devices (SSE/websockets). Other devices see changes on their next request.
- Keeping duplicate cards on the same page in sync after a swap (see Risks).

## Decisions

### D1. Directory layout

```
cmd/server/main.go            config, wiring, migrations, http.Server, refresher, graceful shutdown
internal/
  config/                     env parsing -> Config struct
  db/
    db.go                     Open(): DSN + pragmas, pool settings
    migrate.go                goose with embedded FS
    migrations/*.sql          //go:embed migrations/*.sql
    queries/*.sql             sqlc input
    dbtest/                   New(t): migrated temp-file DB for tests
  store/                      sqlc output (package store), committed
  tmdb/                       HTTP client + response types
    tmdbtest/                 fake TMDB httptest server for catalog/handler tests
  catalog/                    add/refresh/remove media, background refresher
  tracking/                   status changes, mark/unmark, progress, next episode, stale (pure funcs)
  schedule/                   weekday assignment + lineup assembly
  server/
    app.go                    App struct (DI)
    routes.go                 all route registrations in one place
    handler.go                appHandler type + central error rendering
    middleware.go             logging, recover, htmx detection, Vary
    *_handlers.go             grouped per capability
  views/
    layout/                   Base, Nav, Toasts container
    pages/                    Home, List, Search, Show, Schedule, Error
    components/               MediaCard, SearchRow, ProgressCounter, EpisodeRow, SeasonBlock, DayColumn, Toast, StaleBadge
web/
  embed.go                    package web; //go:embed static
  static/js/htmx.min.js       vendored, pinned version noted in file header
  static/css/app.css          Tailwind output (gitignored, produced by build)
  tailwind/input.css          Tailwind source (not embedded)
sqlc.yaml
Makefile
Dockerfile
compose.yaml                  production run (see D14)
```

`//go:embed` can't use `..`, so each embedded tree gets its own `embed.go` next to it (`web/`, `internal/db/`). `*_templ.go` files are gitignored and generated in both dev and Docker. sqlc output is committed (see D12).

Alternative considered: one flat `internal/app` package. Rejected because the stale and next-episode logic would end up tangled with handlers.

### D2. Dependency injection and handler shape

```go
type App struct {
    DB      *sql.DB
    Q       *store.Queries
    TMDB    *tmdb.Client
    Catalog *catalog.Service
    Track   *tracking.Service
    Sched   *schedule.Service
    Cfg     config.Config
    Log     *slog.Logger
    Now     func() time.Time   // in Cfg.Location; overridden in tests
    Assets  web.Manifest       // hashed static URLs
}

type appHandler func(w http.ResponseWriter, r *http.Request) error
```

Handlers return `error`. One adapter (`a.h(fn)`) turns that into `http.Handler` and passes any error to `a.renderError`. Typed errors carry status and a user-safe message:

```go
type HTTPError struct { Status int; Msg string; Err error }
```

`tmdb.ErrUnavailable` maps to 502 "TMDB is unreachable". `sql.ErrNoRows` maps to 404. Anything else maps to 500 with a generic message, and the full error goes to slog. Panics are caught by recover middleware and go through the same path.

### D3. Routing (stdlib `net/http`, Go 1.22+ patterns)

| Method | Pattern | Returns |
|---|---|---|
| GET | `/{$}` | Home (lineup + continue watching) |
| GET | `/watching`, `/backlog`, `/completed`, `/dropped` | List page; query `type=tv\|movie`, `stale=1`, `sort=recent\|stale` |
| GET | `/search` | Search page |
| GET | `/search/results?q=` | SearchRow list fragment |
| POST | `/media` | form `tmdb_id, media_type, status`; returns updated SearchRow |
| GET | `/media/{id}` | Show/movie detail page (seasons, episodes) |
| DELETE | `/media/{id}` | `HX-Redirect` to the list for the item's status (`/watching`, ...) |
| POST | `/media/{id}/status` | form `status, ctx`; returns MediaCard or empty (see D6) |
| POST | `/media/{id}/refresh` | detail content fragment |
| POST | `/media/{id}/watched` | movie watched; MediaCard |
| PUT / DELETE | `/episodes/{id}/watched` | `ctx` selects card or EpisodeRow fragment |
| PUT | `/media/{id}/seasons/{n}/watched` | SeasonBlock fragment |
| GET | `/schedule` | Planner page |
| POST | `/schedule/{day}` | form `media_id`; DayColumn fragment |
| DELETE | `/schedule/{day}/{media_id}` | DayColumn fragment |
| POST | `/schedule/{day}/{media_id}/move` | form `dir=up\|down`; DayColumn fragment |
| GET | `/static/` | embedded FS |
| GET | `/healthz` | `PingContext` |

`{day}` is the `time.Weekday` number (`0`–`6`, Sunday = 0), matching the schema; anything else is a 404.

There are only ~20 routes, so chi adds nothing. Method-scoped patterns give automatic 405s. The mux's 404 is wrapped so it renders the HTML error page.

### D4. SQLite configuration

- Driver `modernc.org/sqlite`, DSN: `file:/data/watchwhat.db?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)&_txlock=immediate`. DSN pragmas apply to every pooled connection, which matters for `foreign_keys` because it is per-connection.
- `db.SetMaxOpenConns(1)`. With two users, serializing all access through one connection removes `SQLITE_BUSY` and lock-upgrade deadlocks entirely. Read concurrency is irrelevant at this scale. Alternative: separate read pool (N conns) and write pool (1 conn). Deferred until there is a reason.
- Never hold a transaction across a TMDB call. Catalog add and refresh fetch first, then write in one short `BEGIN IMMEDIATE` transaction.
- Migrations run with `goose.Up` at startup from the embedded FS, before the listener starts. A failure exits non-zero.

### D5. Schema (`internal/db/migrations/00001_init.sql`)

```sql
-- +goose Up
CREATE TABLE media (
  id              INTEGER PRIMARY KEY,
  tmdb_id         INTEGER NOT NULL,
  media_type      TEXT    NOT NULL CHECK (media_type IN ('tv','movie')),
  title           TEXT    NOT NULL,
  overview        TEXT    NOT NULL DEFAULT '',
  poster_path     TEXT,                 -- TMDB path, e.g. /abc.jpg; hotlinked
  release_date    TEXT,                 -- YYYY-MM-DD (first_air_date for tv)
  tmdb_status     TEXT,                 -- 'Returning Series', 'Ended', 'Released', ...
  runtime_minutes INTEGER,
  refreshed_at    INTEGER NOT NULL,     -- unix seconds
  UNIQUE (media_type, tmdb_id)          -- tv and movie ids are separate namespaces
);

CREATE TABLE tracking (
  media_id          INTEGER PRIMARY KEY REFERENCES media(id) ON DELETE CASCADE,
  status            TEXT    NOT NULL CHECK (status IN ('backlog','watching','completed','dropped')),
  status_changed_at INTEGER NOT NULL,
  added_at          INTEGER NOT NULL,
  watched_at        INTEGER             -- movies only
);
CREATE INDEX tracking_status ON tracking(status);

CREATE TABLE episodes (
  id              INTEGER PRIMARY KEY,
  media_id        INTEGER NOT NULL REFERENCES media(id) ON DELETE CASCADE,
  tmdb_id         INTEGER NOT NULL UNIQUE,
  season_number   INTEGER NOT NULL,     -- 0 = specials
  episode_number  INTEGER NOT NULL,
  name            TEXT    NOT NULL DEFAULT '',
  air_date        TEXT,                 -- YYYY-MM-DD, NULL = TBA
  runtime_minutes INTEGER
);
CREATE INDEX episodes_order ON episodes(media_id, season_number, episode_number);

CREATE TABLE episode_watches (
  episode_id INTEGER PRIMARY KEY REFERENCES episodes(id) ON DELETE CASCADE,
  watched_at INTEGER NOT NULL
);

CREATE TABLE weekly_schedule (
  weekday  INTEGER NOT NULL CHECK (weekday BETWEEN 0 AND 6),  -- time.Weekday, 0 = Sunday
  media_id INTEGER NOT NULL REFERENCES media(id) ON DELETE CASCADE,
  position INTEGER NOT NULL,
  PRIMARY KEY (weekday, media_id)
);

-- +goose Down
DROP TABLE weekly_schedule; DROP TABLE episode_watches; DROP TABLE episodes;
DROP TABLE tracking; DROP TABLE media;
```

Notes:
- **`media` vs `tracking`.** The split lets a refresh overwrite `media` without touching household state. This replaces the brief's `user_media_progress`, since there are no users.
- **Watches are a row's presence, not a boolean.** `INSERT ... ON CONFLICT DO NOTHING` makes marking idempotent, and `DELETE` unmarks. That covers the double-tap scenario.
- **Episodes upsert on `tmdb_id` and are never deleted by refresh** (spec: "Episode removed upstream"). The ordering index is deliberately not UNIQUE, so a TMDB renumbering can't make an upsert fail.
- **Timestamps vs air dates.** Timestamps are UTC unix seconds. Air dates stay as TMDB calendar-date strings. "Aired" is `air_date IS NOT NULL AND air_date <= :today`, where `:today` is passed from Go as the server-TZ local date. SQLite's `date('now')` is never used because it is UTC and would flip "today" at the wrong hour.
- **Weekday encoding.** `weekday` uses Go's `time.Weekday` encoding to avoid conversion. Display order (Mon to Sun) is a view concern.

### D6. Progress, next episode, stale

sqlc queries return one aggregate row per show: `aired_total`, `watched_total`, `last_watched_at`, `next_episode_*` (correlated subquery: min season/episode among aired, season>0, unwatched), and `next_upcoming_air_date`. The `tracking` package turns that into a view model:

```go
type Progress struct {
    Next        *EpisodeRef  // nil => caught up
    Upcoming    *time.Time   // next known air date when caught up
    Watched, Aired int
    LastActivity time.Time   // max(last_watched_at, status_changed_at if no watches)
    DaysIdle    int          // calendar days in now.Location()
    Stale       bool
    CanComplete bool         // offer "Mark completed" (see below)
}
func Compute(row ProgressRow, now time.Time) Progress  // now carries cfg.Location
```

`CanComplete = status == watching && Next == nil && Upcoming == nil && Aired > 0 && tmdb_status IN ('Ended','Canceled')`.

`Stale = status == watching && Next != nil && DaysIdle > 14`. Using calendar days, not 24h multiples, makes "14 days ago" mean the same thing at 08:00 and 23:00. The threshold is a package constant, not config. It is computed at read time (spec: stale-detection). "Most stale first" and "stale only" are applied in Go after the query, because the list is at most dozens of rows.

### D7. templ structure and HTMX patterns

**Full page vs fragment.** Page handlers build a content component and call `a.render(w, r, title, active, content)`:
- Full document: no `HX-Request`, or `HX-History-Restore-Request: true`, or `HX-Boosted: true`. Rendered as `layout.Base(title, active, content)`.
- Content only: everything else. Nav links use `hx-get hx-target="#main" hx-push-url="true"`.
- `Vary: HX-Request` is always set so browser caches don't serve a fragment for a direct load.

**Cards.**
- `MediaCard(it tracking.Item, listCtx string, undo int64)` is the single swap unit. (The parameter can't be named `ctx` because templ reserves that name for its context.) Its root is `<article>`, and every action inside it uses `hx-target="closest article" hx-swap="outerHTML"`. No DOM ids are needed, so the same show can appear in Lineup and in Continue watching without id collisions.
- `ctx` (`home|watching|backlog|...`) is sent with `hx-vals`. On a status change, if the new status doesn't belong in `ctx`, the server returns an empty 200 body and the card disappears in place.

**Mark next.**
- The card renders `hx-put="/episodes/{nextID}/watched"` with the specific episode id, not a "next" endpoint. Two taps then mark the same episode twice (a no-op), and an episode can never be skipped.
- Also set: `hx-disabled-elt="this"` and `hx-sync="closest article:drop"`.
- The response card includes an Undo button (`hx-delete` on the same id). It's present only in that response, so it vanishes on the next render.

**Search.**
- Input: `hx-get="/search/results" hx-trigger="input changed delay:300ms, search" hx-target="#results" hx-sync="this:replace"`.
- An empty `q` returns an empty fragment without calling TMDB.
- Each SearchRow has its own root and swaps itself after an add.

**Episode list.** The detail page renders `SeasonBlock` > `EpisodeRow`. Toggling a row swaps the row and also returns an out-of-band `ProgressCounter` (`hx-swap-oob="true"`, id `progress-{mediaID}`). The id is unique because the detail page shows one show.

**Page fragments.** An HTMX page response also carries `<title>` and an out-of-band `Nav`, so the active section and tab title follow navigation. Those and the progress counter are the only OOB swaps.

**Errors.**
- htmx 2 does not swap 4xx/5xx responses by default. The base layout sets `<meta name="htmx-config">` with `responseHandling` so `[45]..` responses swap with `error: true`.
- `renderError` always sends `HX-Retarget: #toasts` and `HX-Reswap: beforeend` with a `Toast` fragment on HTMX requests. Error HTML can therefore never land in a card.
- Non-HTMX requests get `pages.Error` in the full layout.

**Profile switching.** None. There is no per-visitor state anywhere.

### D8. TMDB client (`internal/tmdb`)

- **Transport.** API v3 with Bearer read-access token. `BaseURL` is injectable so tests can point it at an `httptest.Server`. `http.Client{Timeout: 10s}`. `language` comes from config (default `en-US`).
- **Search.** `Search(ctx, q)` uses `/search/multi`, drops `person` results, and maps the rest to `{TMDBID, Type, Title, Year, PosterPath}`.
- **Movies.** `Movie(ctx, id)` uses `/movie/{id}`.
- **Shows.**
  - `Show(ctx, id)` uses `/tv/{id}` for `number_of_seasons` and the season list.
  - Seasons are fetched with `append_to_response=season/1,...`, chunked 20 per request (TMDB's limit). Season 0 is included when listed.
  - A typical show costs 1–2 requests.
- **Errors.**
  - Non-2xx statuses become typed errors.
  - A 429 is retried once, honoring `Retry-After` (capped at 5s).
  - Network errors and 5xx wrap `ErrUnavailable`.
- **Images.** `PosterURL(path, size)` builds `https://image.tmdb.org/t/p/{w92|w185|w342}{path}`. Templates emit `srcset` with w185/w342 for cards and w92 for search rows.

### D9. Background refresher (`catalog.Refresher`)

- **Loop.** A goroutine started by main runs once at startup (after a 30s delay) and then every 6h.
- **Selection.** Each run selects shows where `tmdb_status IN ('Returning Series','In Production','Planned','Pilot')`, tracking status is `backlog` or `watching`, and `refreshed_at < now-24h`.
- **Execution.** Shows are refreshed sequentially. Each failure is logged at warn and the show stays eligible for the next run.
- **Shutdown.** The loop stops when the root context is cancelled.
- **Shared code.** Manual refresh uses the same `catalog.Refresh(ctx, mediaID)`.

A 6h tick with a 24h eligibility window gives "at least daily" without a cron dependency.

### D10. Assets and Tailwind

- **Versioned URLs.** At startup, `web` walks the embedded `static/` FS, hashes each file (first 8 hex chars of SHA-256), and builds a manifest. Templates call `assets.URL("css/app.css")`, which returns `/static/css/app.css?v=ab12cd34`. A request with `?v=` gets `Cache-Control: public, max-age=31536000, immutable`; without it, `no-cache`.
- **Tailwind.** v4 standalone CLI with a CSS-first config. `web/tailwind/input.css` contains `@import "tailwindcss";` plus `@source "../../internal/views";`. The dark palette is set via `@theme` tokens, and `<html class="dark">` with `color-scheme: dark` is the default.
- **htmx.** Vendored htmx 2.x, pinned, with no extensions.

### D11. Configuration (env)

| Var | Default | Notes |
|---|---|---|
| `WATCHWHAT_ADDR` | `:8080` | |
| `WATCHWHAT_DB_PATH` | `/data/watchwhat.db` | |
| `WATCHWHAT_TMDB_TOKEN` | required | startup fails if empty |
| `WATCHWHAT_TMDB_LANGUAGE` | `en-US` | |
| `TZ` | `UTC` | defines "today"; tzdata embedded via `import _ "time/tzdata"` |
| `WATCHWHAT_DEBUG` | unset | any value enables per-request debug logging |

### D12. Build tooling and Dockerfile

- **Go-based tools.** `templ` is pinned as a Go tool (`go get -tool github.com/a-h/templ/cmd/templ@vX`) and run with `go tool templ generate`, so the generator version always matches the runtime library.
- **sqlc.** Run locally via `make sqlc`, with output committed. sqlc's build pulls in a CGO Postgres parser, so it stays out of the image build. CI can check for drift with `sqlc diff`.
- **Go version.** Go 1.26. The pinned templ and goose releases require it.
- **Dockerfile.** Multi-stage. See `Dockerfile` for the full file.
  - The build stage runs `FROM --platform=$BUILDPLATFORM golang:1.26-alpine` and cross-compiles with `GOOS=$TARGETOS GOARCH=$TARGETARCH CGO_ENABLED=0`, so a multi-arch buildx build needs no emulation.
  - Tailwind therefore runs on the build host, and `BUILDARCH` (not `TARGETARCH`) selects the binary. It is the v4.3.5 musl build, checked against a pinned SHA-256 per arch.
  - The musl Tailwind binary links `libstdc++` and `libgcc` dynamically, so the build stage installs both.
  - Go module and build caches use `RUN --mount=type=cache`.
  - The runtime image is `gcr.io/distroless/static-debian12:nonroot` with `/data` pre-created as 65532:65532.
  - The image sets `WATCHWHAT_ADDR`, `WATCHWHAT_DB_PATH`, `VOLUME /data`, `EXPOSE 8080`, and `HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["/watchwhat","healthcheck"]`.

- **Healthcheck.** distroless has no shell or curl, so the binary accepts a `healthcheck` subcommand that GETs `http://127.0.0.1:8080/healthz` and exits 0/1.
- **Volume ownership.** A named volume (`-v watchwhat-data:/data`) inherits the 65532 ownership from the image. A bind mount needs `chown 65532:65532` on the host directory, which the README documents.
- **Running it.** `compose.yaml` is the primary path (see D14). The README keeps a `docker run` equivalent.

### D13. Testing approach

- **`tracking.Compute`.** Table-driven tests with a fixed `today` cover every stale and next-episode scenario in the specs.
- **Store.** Store tests use a temp-file DB (not `:memory:`, so WAL and the pragmas are exercised) with migrations applied.
- **TMDB.** The client is tested against `httptest.Server` with JSON fixtures in `internal/tmdb/testdata`.
- **Handlers.**
  - Tests build `App` with a fake TMDB server and a fixed `Now`, then assert status, HX response headers, and fragment-vs-document output (presence of `<html`).
  - An integration test hits a running server end to end.

### D14. Deployment (home server)

The target is a Mac mini running OrbStack, with Tailscale and Caddy on the host.

- **Compose.** One service. `env_file: .env` supplies the token and `TZ`, so nothing has to be exported in a shell. `restart: unless-stopped`.
- **Loopback only.** The port is published as `127.0.0.1:${WATCHWHAT_PORT:-8081}:8080`. The app has no auth, so the host's Caddy is the only way in, and the app is never exposed on the LAN. 8081 is the default because another app already holds 8080 on the host.
- **Named volume.** The volume is explicitly named `watchwhat-data`, so the README backup commands work without the Compose project prefix.
  - Prefer it to a bind mount on macOS. A bind mount goes through OrbStack's file sharing, which ignores ownership and makes SQLite WAL locking unreliable if the host ever opens the file.
  - On a Linux host a bind mount works if the directory is `chown 65532:65532`. This was verified: without the chown, startup fails with `unable to open database file (14)`.
- **Caddy.** Host Caddy terminates TLS on the machine's single `*.ts.net` name, using certificates from the local tailscaled, and gives WatchWhat its own site on a separate port (e.g. `:8443`) proxying to `127.0.0.1:8081`.
  - Path-prefix mounting (`handle_path /watchwhat*`) is not supported, because links, assets, HTMX attributes and `HX-Redirect` are all root-relative. Supporting a prefix would mean threading a base path through every URL. That isn't worth it while ports are free.
- **No private details in the repo.** Tailnet hostnames and IPs stay out of the repo, so docs use placeholders.

## Risks / Trade-offs

- [Stale duplicate cards on Home: marking from Lineup doesn't update the same show in Continue watching] → Accepted. The next navigation fixes it. If it bothers us, Home can exclude lineup shows from Continue watching.
- [Two devices act on the same show concurrently] → Idempotent episode-specific writes and last-write-wins status. No data loss, worst case a stale card until refresh.
- [Single DB connection serializes requests] → Every query is sub-millisecond at this size, and no transaction spans network I/O. Split into read and write pools if latency ever shows up.
- [TMDB renumbers or merges episodes] → Upsert by `tmdb_id`, never delete. Orphaned watched episodes stay visible. Manual cleanup is acceptable.
- [WAL file not captured by naive backups] → Document backing up with `sqlite3 /data/watchwhat.db ".backup ..."` or `VACUUM INTO`, run from a throwaway alpine container that mounts the volume (the distroless image has no `sqlite3`). Do not copy the `.db` file alone while it's running.
- [Hotlinked posters fail when TMDB images are slow or down] → The `<img>` sits in a fixed-aspect box whose background shows the title, so a failed load degrades to the placeholder without JS. Thumbnail caching is a later change, and `poster_path` is already stored for it.
- [htmx `responseHandling` override changes default semantics] → Confined to one meta tag and covered by a handler test asserting `HX-Retarget` on every error.
- [Tailwind standalone binary download in the build] → Pinned version and SHA-256. The build fails loudly on mismatch.
- [OrbStack starts at login, not boot] → Host-level auto-login brings the containers back after a reboot. This is shared with the other apps on the mini.
- [Caddy on a non-443 port gets plain-HTTP requests when users omit `https://`] → Bookmark the `https://` URL, or enable Caddy's `http_redirect` listener wrapper for that port.

## Migration Plan

Greenfield deploy (the README has the exact steps):
1. Create `.env` with the token and `TZ`.
2. `docker compose up -d --build`. Migrations run on first start.
3. Add the Caddy site block and reload Caddy.

Rollback: stop the container, restore the `.backup` copy, run the previous image tag. Future migrations stay forward-only in practice; goose Down exists only for local dev.
