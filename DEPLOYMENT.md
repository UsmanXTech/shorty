# Shorty deployment guide

Shorty is a self-hosted URL shortener that runs as a single Go binary and stores application data in SQLite. The Docker image runs the binary as a non-root user and persists SQLite data under `/data`.

## Prerequisites

- A host with Go 1.25+ for bare-metal builds, or Docker with Compose support for container deployment.
- Persistent storage for the SQLite database.
- A public hostname or URL if generated QR codes should use a public address.
- An operator-supplied, legally licensed GeoIP CSV if country analytics are required.

## Configuration

Shorty reads these environment variables:

| Variable | Default | Purpose |
| --- | --- | --- |
| `SHORTY_ADDR` | `:8080` | HTTP listen address. |
| `SHORTY_DB` | `shorty.db` | SQLite database path. |
| `SHORTY_BASE_URL` | empty | Public base URL used when generating QR codes. |
| `SHORTY_GEOIP_DB` | empty | Local GeoIP CSV path for country analytics. |

Keep the SQLite database and any GeoIP data on persistent storage. Do not put secrets in the repository or image; Shorty currently does not require a runtime secret for its core deployment.

## Docker deployment

Build the image from a checked-out release:

```bash
docker build -t shorty:current .
```

Start it with persistent application data:

```bash
docker run -d --name shorty \
  --restart unless-stopped \
  -p 8080:8080 \
  -v shorty-data:/data \
  -e SHORTY_BASE_URL=https://short.example.com \
  shorty:current
```

The container stores SQLite at `/data/shorty.db`. Keep the `/data` volume when replacing the container. The image uses a non-root runtime user.

### Docker Compose

The repository includes `docker-compose.yml` with a persistent `shorty-data` volume:

```bash
export SHORTY_BASE_URL=https://short.example.com
docker compose up -d --build
```

Inspect the service with:

```bash
docker compose ps
docker compose logs --tail=100 shorty
```

For GeoIP, mount a local CSV read-only and set `SHORTY_GEOIP_DB=/data/geoip.csv` in the Compose environment.

## Bare-metal deployment

Build the production binary:

```bash
go build -trimpath -ldflags='-s -w' -o shorty ./cmd/shorty
```

Create a dedicated data directory and run the binary with a persistent database path:

```bash
sudo install -d -m 0750 /var/lib/shorty
sudo install -m 0755 ./shorty /usr/local/bin/shorty
SHORTY_DB=/var/lib/shorty/shorty.db \
SHORTY_ADDR=:8080 \
SHORTY_BASE_URL=https://short.example.com \
/usr/local/bin/shorty
```

For a production service manager, run Shorty under a dedicated unprivileged account and grant that account write access only to its data directory. Put TLS termination and public HTTP exposure behind a reverse proxy or load balancer when required by the deployment environment.

## Backups

The critical state is the SQLite database at the configured `SHORTY_DB` path. Back up that file to storage that is separate from the host.

For a simple consistent file backup, stop Shorty before copying the database:

```bash
sudo systemctl stop shorty
sudo cp /var/lib/shorty/shorty.db /var/backups/shorty-$(date +%Y%m%d-%H%M%S).db
sudo systemctl start shorty
```

For Docker Compose:

```bash
docker compose stop shorty
mkdir -p backups
docker run --rm \
  -v shorty-data:/data:ro \
  -v "$PWD/backups:/backup" \
  alpine:3.22 \
  cp /data/shorty.db /backup/shorty-$(date +%Y%m%d-%H%M%S).db
docker compose start shorty
```

Test backups by restoring a copy on a separate Shorty instance before relying on them for disaster recovery. Retain multiple generations according to your operational requirements.

Litestream is mentioned as an optional future backup component; it is not currently configured or required by Shorty. Do not assume continuous replication is enabled unless you have configured and tested it separately.

## Updates

Always keep a known-good version before updating.

For a source-based deployment:

```bash
git fetch --tags origin
git checkout <release-or-commit>
go test ./...
go build ./...
```

For Docker, build a new immutable image tag instead of overwriting the known-good tag:

```bash
docker build -t shorty:next .
docker run --rm shorty:next --help
```

After validation, replace the running container while preserving `shorty-data`:

```bash
docker compose up -d --build
```

Monitor logs and the application after the update. Keep the previous image or binary until the new version has been verified.

## Rollback

Rollback means restoring both the application version and, when necessary, the compatible database backup.

For Docker, keep the previous image tag and recreate the service from it while using the same persistent volume:

```bash
docker tag shorty:current shorty:previous
# Build and deploy the new version as shorty:current.
# If validation fails, restore the previous image tag and recreate the container.
docker compose up -d
```

For bare metal, retain the previous binary and restore it if the new version fails health checks or otherwise cannot serve traffic:

```bash
sudo cp /usr/local/bin/shorty /usr/local/bin/shorty.failed
sudo cp /usr/local/bin/shorty.previous /usr/local/bin/shorty
sudo systemctl restart shorty
```

If application changes require a database rollback, stop the service first and restore a known-good SQLite backup to the configured database path. Never overwrite a live database while Shorty is running.

## Operational checklist

Before production:

- [ ] Set `SHORTY_BASE_URL` to the real public URL.
- [ ] Put `SHORTY_DB` on persistent storage.
- [ ] Run the service as an unprivileged user.
- [ ] Configure restart behavior and log collection.
- [ ] Create and periodically verify SQLite backups.
- [ ] Keep at least one known-good application version for rollback.
- [ ] If using GeoIP, verify the dataset license and mount it read-only.
- [ ] Test an update and rollback procedure before making a production change.

After deployment, verify the service is reachable, create a test short link, follow the redirect, and confirm analytics are being recorded as expected.
