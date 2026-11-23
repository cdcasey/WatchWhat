# Proposal

## Why

Our household wants a private, low-friction way to track which TV shows and movies we're watching, queuing, finishing, and abandoning. A spreadsheet would work but lacks TMDB metadata, episode-level progress, and "what's on tonight" views. A small self-hosted app over Tailscale gives us that with no accounts and no third-party service holding our data.

## What Changes

- New Go web application (single static binary, single Docker container) serving server-rendered HTML with HTMX partial updates; no JS framework, no Node toolchain.
- TMDB-backed search and ingestion of shows and movies, with metadata, seasons, episodes, and air dates cached in SQLite.
- One shared household ledger: every item has one status (backlog, watching, completed, dropped) and each episode is either watched or not. No users, profiles, or login.
- Episode-level progress with a one-tap "mark next episode watched" action that updates the card in place.
- Stale indicator, filter, and sort for watching shows with unwatched aired episodes and no activity in 14+ days.
- Household weekly schedule assigning shows to weekdays, surfaced as "Today's Lineup" on the home view.
- Automatic daily metadata refresh for airing shows that are being tracked, plus manual refresh.

**Out of scope**: authentication, multiple users/profiles, local poster caching (posters are hotlinked from TMDB; a simple thumbnail cache may follow later), rewatch tracking, ratings/reviews, import from Trakt/Serializd.

## Capabilities

### New Capabilities
- `app-shell`: Page layout and navigation, mobile-first dark UI, full-page vs. partial (HTMX) responses, static asset serving, and HTML error responses.
- `media-catalog`: Searching TMDB, adding shows/movies to the local catalog, storing metadata/seasons/episodes, and refreshing that metadata manually and on a schedule.
- `watch-tracking`: Per-item status lifecycle, episode watched/unwatched state, next-episode calculation, mark-next action, movie watched state, and status-filtered list views.
- `stale-detection`: Rules for when a watching show counts as stale, the stale badge, and stale filter/sort.
- `weekly-schedule`: Assigning shows to weekdays, editing the weekly planner, and the "Today's Lineup" home section.

### Modified Capabilities
<!-- None: greenfield project. -->

## Impact

- New repository contents: Go module, `cmd/server`, `internal/*` packages, templ components, SQL migrations and sqlc queries, Tailwind input CSS, vendored `htmx.min.js`, Dockerfile.
- External dependency: TMDB API v3 (requires an API read access token supplied via environment).
- Runtime: one container with a persistent volume for the SQLite database; reached only over Tailscale.
- Go dependencies: `modernc.org/sqlite`, `github.com/pressly/goose/v3`, `github.com/a-h/templ`; build-time tools: `templ`, `sqlc`, Tailwind standalone CLI.
