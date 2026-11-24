# Tasks

## 1. Project scaffolding

- [x] 1.1 `go mod init`, add deps (`modernc.org/sqlite`, `goose/v3`, `templ`) and `templ` as a Go tool; verify `go build ./...` and `go tool templ version` succeed
- [x] 1.2 Create the directory layout from design D1 with `.gitignore` (`*_templ.go`, `web/static/css/app.css`, `*.db*`); verify the tree matches D1
- [x] 1.3 Vendor pinned `htmx.min.js` into `web/static/js/` and add `web/tailwind/input.css` with dark theme tokens; verify the standalone Tailwind CLI produces `web/static/css/app.css`
- [x] 1.4 Add Makefile targets `generate`, `css`, `sqlc`, `build`, `dev`, `test`; verify `make build` produces a binary with `CGO_ENABLED=0`

## 2. Config, database, migrations

- [x] 2.1 Implement `internal/config` env parsing (D11) with required-token check; verify unit test for defaults and missing-token error
- [x] 2.2 Implement `db.Open` with DSN pragmas and `SetMaxOpenConns(1)` (D4); verify test asserts `PRAGMA foreign_keys`=1 and `journal_mode`=wal on a temp-file DB
- [x] 2.3 Write `00001_init.sql` (D5) and embedded goose runner; verify migrate up/down/up succeeds in a test and cascade delete from `media` removes episodes, watches, tracking, schedule rows
- [x] 2.4 Configure sqlc and write queries for catalog upserts, tracking, watches, schedule, and progress aggregates; verify `make sqlc` generates `internal/store` and store tests cover upsert idempotency and `ON CONFLICT DO NOTHING` watches

## 3. App shell and HTTP foundation

- [x] 3.1 Implement `web` embed + asset manifest with hashed URLs and cache headers (D10); verify test: `?v=` requests get `immutable`, URL changes when file content changes
- [x] 3.2 Implement `App` struct, `appHandler` adapter, `HTTPError`, recover/logging middleware, and `renderError` (D2, D7 errors); verify tests: 404 page is full HTML, HTMX error returns toast with `HX-Retarget: #toasts`, 500 hides error detail
- [x] 3.3 Build `layout.Base`, `Nav`, toasts container, htmx-config meta, and `a.render` full-vs-fragment logic with `Vary: HX-Request`; verify tests for plain, `HX-Request`, `HX-Boosted`, and `HX-History-Restore-Request` requests
- [x] 3.4 Add `/healthz`, `healthcheck` subcommand, graceful shutdown in `cmd/server`; verify `/healthz` returns 200, and 503 after the DB is closed
- [x] 3.5 Check the layout at 360px and 1280px in a browser; verify no horizontal scroll and grid column count increases (app-shell spec)

## 4. TMDB client and media catalog

- [x] 4.1 Implement `internal/tmdb` (search/multi, movie, tv + chunked `append_to_response` seasons, 429 retry, `ErrUnavailable`, `PosterURL`) (D8); verify tests against `httptest.Server` fixtures including a show with >20 seasons and a 429 response
- [x] 4.2 Implement `catalog.Add` and `catalog.Refresh` (fetch outside tx, single write tx, upsert by `tmdb_id`, never delete episodes); verify tests for idempotent add, specials stored, new episode added on refresh, removed-upstream episode retained with watch state
- [x] 4.3 Implement `catalog.Remove`; verify test that schedule and watches are gone after removal
- [x] 4.4 Build Search page, `SearchRow`, live search fragment, and `POST /media` row swap; verify handler tests: empty `q` makes no TMDB call, added item shows status, TMDB outage returns toast
- [x] 4.5 Implement `catalog.Refresher` (D9) with injected clock and interval; verify test selects only airing backlog/watching shows older than 24h and skips Ended shows
- [x] 4.6 Build the detail page with refresh and remove actions; verify manual refresh updates the page fragment

## 5. Watch tracking

- [x] 5.1 Implement `tracking.Compute` (next episode, caught up, upcoming, progress, days idle, stale) (D6); verify table-driven tests covering every watch-tracking and stale-detection scenario
- [x] 5.2 Implement status changes, episode mark/unmark, season mark (aired only), movie watched → completed; verify service tests
- [x] 5.3 Build `MediaCard`, `ProgressCounter`, `StaleBadge`, and the episode-specific mark/undo actions with `hx-disabled-elt`/`hx-sync` (D7); verify handler test that two PUTs to the same episode produce one watch row and the card advances once
- [x] 5.4 Build the "Mark completed" prompt for ended shows with no remaining episodes; verify status stays watching until tapped
- [x] 5.5 Build list pages (`/watching`, `/backlog`, `/completed`, `/dropped`) with type filter, recent sort, stale filter and most-stale sort, reflected in URL; verify handler tests for each query param
- [x] 5.6 Implement `ctx`-aware status-change responses (empty body when the card leaves the list); verify handler test
- [x] 5.7 Build `SeasonBlock`/`EpisodeRow` toggles on the detail page with OOB `ProgressCounter`; verify response contains both the row and the `hx-swap-oob` counter

## 6. Weekly schedule and Home

- [x] 6.1 Implement schedule service (assign idempotently, remove, reorder by swapping positions, lineup for a weekday excluding completed/dropped); verify tests including duplicate assignment and inactive shows retained
- [x] 6.2 Build Schedule page with Monday-first `DayColumn` fragments and add/remove/move actions; verify handler tests that only the affected day fragment is returned
- [x] 6.3 Build Home with Today's Lineup (server TZ via `App.Now`) and Continue watching (≤8, recent activity); verify handler test with a fixed Tuesday clock and the empty-day state linking to Schedule

## 7. Container and deployment

- [x] 7.1 Write the multi-stage Dockerfile (D12) with pinned Tailwind version + SHA-256 and `TARGETARCH`; verify `docker buildx build --platform linux/amd64,linux/arm64` succeeds
- [ ] 7.2 Write README sections: env vars, `docker run` with named volume, bind-mount `chown 65532`, and backup via `.backup`/`VACUUM INTO`; verify by running the documented command on a clean host and confirming data persists across container recreation
- [x] 7.3 End-to-end check in the container: search, add a show, mark next episode, assign to today, see it in Today's Lineup; verify `HEALTHCHECK` reports healthy

## Workflow follow-up

- Archive the change after implementation is reviewed and deployed.
