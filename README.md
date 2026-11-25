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

## Deploying to the Mac mini

The mini runs OrbStack, Tailscale and Caddy on the host. WatchWhat listens on `127.0.0.1:8081` (wordler has 8080), and Caddy terminates TLS on the mini's tailnet name and proxies to it. Nothing is exposed on the LAN, which matters because WatchWhat has no auth.

Data goes in the `watchwhat-data` named volume, not a bind-mounted folder. On macOS a bind mount goes through OrbStack's file sharing, which is slower and makes SQLite's WAL locking unreliable if anything on the Mac side ever opens the file. The volume lives inside OrbStack's Linux VM. Back it up with the commands under [Backups](#backups).

### 1. Get the code onto the mini

```sh
git clone https://github.com/cdcasey/WhatchWhat.git ~/dev/WatchWhat
cd ~/dev/WatchWhat
```

### 2. Create `.env`

`.env` is gitignored and excluded from the image.

```sh
cat > .env <<'EOF'
WATCHWHAT_TMDB_TOKEN=<TMDB read access token>
TZ=America/Chicago
# WATCHWHAT_PORT=8081
EOF
chmod 600 .env
```

`TZ` matters: it defines "today" for the lineup, air dates and stale detection. Without it the app runs in UTC.

### 3. Build and start

```sh
docker compose up -d --build
docker compose ps                      # wait for "(healthy)", about 10s
curl -s http://127.0.0.1:8081/healthz  # → ok
```

### 4. Find the host Caddyfile

If Caddy came from Homebrew (the usual case), the config is **`/opt/homebrew/etc/Caddyfile`** and it runs as a `brew services` service. If you're not sure, check:

```sh
brew services list | grep caddy          # installed and running via Homebrew?
ps aux | grep '[c]addy run'              # the --config argument is the file in use
```

If neither shows anything, Caddy isn't installed yet: `brew install caddy`, then continue. You'll create the file in the next step.

### 5. Add WatchWhat to the Caddyfile

Tailscale gives the mini **one** name, `<mini>.<tailnet>.ts.net`, so each app gets its own HTTPS port on it rather than its own hostname. Find the name with `tailscale status --self` (on the GUI install the CLI is `/Applications/Tailscale.app/Contents/MacOS/Tailscale`).

WatchWhat can't be mounted under a path prefix (`handle_path /watchwhat*` won't work). Its links, assets, HTMX requests and `HX-Redirect` headers all use root-relative URLs like `/media/1`, which would miss the prefix. Give it its own site block on its own port. Add it alongside any existing blocks:

```caddyfile
<mini>.<tailnet>.ts.net:8443 {
	reverse_proxy 127.0.0.1:8081
}
```

Caddy fetches the `*.ts.net` certificate from the local Tailscale daemon automatically. For that to work, HTTPS certificates must be enabled for the tailnet (admin console → DNS → HTTPS Certificates).

### 6. Validate and reload Caddy

```sh
caddy fmt --overwrite /opt/homebrew/etc/Caddyfile
caddy validate --config /opt/homebrew/etc/Caddyfile
brew services restart caddy              # or: caddy reload --config /opt/homebrew/etc/Caddyfile
```

If the certificate fails (check `tail -f /opt/homebrew/var/log/caddy.log` or `brew services info caddy`), get one manually with `tailscale cert <mini>.<tailnet>.ts.net` and add `tls /path/to/<name>.crt /path/to/<name>.key` inside the block.

### 7. Check from another tailnet device

Open `https://<mini>.<tailnet>.ts.net:8443`. You should see Today's Lineup. Search for a show to confirm TMDB works.

### 8. Survive reboots

OrbStack starts at login, not at boot. The auto-login setting described in wordler's README covers every container, WatchWhat included (`restart: unless-stopped`).

### Upgrading

```sh
cd ~/dev/WatchWhat && git pull && docker compose up -d --build
```

Migrations run on start. Data survives `docker compose down`; **`down -v` deletes the volume**.

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
| `make dev` | Watches templ and Tailwind, restarts the server; reads `.env` if present. Uses `data/watchwhat.db` (gitignored) unless `WATCHWHAT_DB_PATH` is set. Proxy at http://localhost:7331. |
| `make test` | generate + `go test ./...`. |

Generated `*_templ.go` files and `app.css` are gitignored; `internal/store` (sqlc output) is committed.

Local run:

```sh
make build
WATCHWHAT_TMDB_TOKEN=... WATCHWHAT_DB_PATH=./watchwhat.db TZ=America/Chicago ./bin/watchwhat
```
