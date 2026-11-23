# WatchWhat

A private, self-hosted tracker for the TV shows and movies a household is watching, queuing, finishing, and dropping. Server-rendered Go + HTMX, SQLite, TMDB metadata. One static binary in one container, meant to be reached over Tailscale. There is no authentication: everyone who can reach it shares one ledger.

## Configuration

| Variable | Default | Notes |
|---|---|---|
| `WATCHWHAT_TMDB_TOKEN` | — (required) | TMDB API **read access token** (v4 bearer token from your TMDB account's API settings). Startup fails without it. |
| `WATCHWHAT_ADDR` | `:8080` | Listen address. |
| `WATCHWHAT_DB_PATH` | `/data/watchwhat.db` | SQLite database file. WAL mode, so `-wal`/`-shm` files live next to it. |
| `WATCHWHAT_TMDB_LANGUAGE` | `en-US` | TMDB metadata language. |
| `TZ` | `UTC` | Defines "today" for Today's Lineup, air dates, and stale detection. Zone data is embedded in the binary. |
| `WATCHWHAT_DEBUG` | unset | Any value enables per-request debug logging. |

## Run with Docker

Build (single arch for the local machine):

```sh
docker build -t watchwhat:latest .
```

Multi-arch (amd64 + arm64):

```sh
docker buildx build --platform linux/amd64,linux/arm64 -t watchwhat:latest .
```

Run with a named volume (recommended; the volume inherits the image's `65532:65532` ownership of `/data`):

```sh
docker run -d --name watchwhat -p 8080:8080 \
  -e WATCHWHAT_TMDB_TOKEN=... -e TZ=America/Chicago \
  -v watchwhat-data:/data --restart unless-stopped watchwhat:latest
```

Migrations run automatically on start. Data survives `docker rm` / re-creating the container as long as the same volume is mounted.

### Bind mount instead of a named volume

The container runs as the distroless `nonroot` user (uid/gid 65532). A host directory must be writable by that user:

```sh
sudo mkdir -p /srv/watchwhat
sudo chown 65532:65532 /srv/watchwhat
docker run -d --name watchwhat -p 8080:8080 \
  -e WATCHWHAT_TMDB_TOKEN=... -e TZ=America/Chicago \
  -v /srv/watchwhat:/data --restart unless-stopped watchwhat:latest
```

### Health

`GET /healthz` returns 200 when the database is reachable and 503 otherwise. The image's `HEALTHCHECK` runs `/watchwhat healthcheck`, which calls that endpoint (distroless has no shell or curl). Check it with `docker inspect --format '{{.State.Health.Status}}' watchwhat`.

## Backups

The database runs in WAL mode, so **do not copy `watchwhat.db` alone while the app is running** — recent writes may still be in `watchwhat.db-wal`. Use SQLite's online backup instead; it is safe while the app is running.

The runtime image has no `sqlite3`, so run it from a throwaway container that mounts the same volume:

```sh
mkdir -p backups
docker run --rm -v watchwhat-data:/data -v "$PWD/backups":/backup alpine:3 \
  sh -c 'apk add --no-cache sqlite >/dev/null && sqlite3 /data/watchwhat.db ".backup /backup/watchwhat-$(date +%F).db"'
```

`VACUUM INTO` works too and produces a compacted copy:

```sh
docker run --rm -v watchwhat-data:/data -v "$PWD/backups":/backup alpine:3 \
  sh -c "apk add --no-cache sqlite >/dev/null && sqlite3 /data/watchwhat.db \"VACUUM INTO '/backup/watchwhat-\$(date +%F).db'\""
```

With a bind mount and `sqlite3` installed on the host, run the same `sqlite3 ... ".backup ..."` command directly against `/srv/watchwhat/watchwhat.db`.

**Restore:** stop the container, replace `watchwhat.db` in the volume with the backup (and delete any `watchwhat.db-wal`/`-shm`), then start it again.

## Development

Requires Go 1.26+. Everything else is fetched on demand:

| Target | What it does |
|---|---|
| `make generate` | `templ generate` (templ is pinned as a Go tool in `go.mod`). |
| `make css` | Builds `web/static/css/app.css` with the Tailwind standalone CLI, downloaded to `bin/` on first use. |
| `make sqlc` | Regenerates `internal/store` from `internal/db/{migrations,queries}` (pinned sqlc via `go run`; needs CGO). Output is committed. |
| `make build` | generate + css + `CGO_ENABLED=0` build to `bin/watchwhat`. |
| `make dev` | Watches templ and Tailwind, restarts the server; reads `.env` if present. Proxy at http://localhost:7331. |
| `make test` | generate + `go test ./...`. |

Generated `*_templ.go` files and `app.css` are gitignored; `internal/store` (sqlc output) is committed.

Local run:

```sh
make build
WATCHWHAT_TMDB_TOKEN=... WATCHWHAT_DB_PATH=./watchwhat.db TZ=America/Chicago ./bin/watchwhat
```
