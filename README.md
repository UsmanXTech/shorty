# Shorty

A self-hosted URL shortener focused on simplicity, speed, privacy-friendly analytics, and easy deployment.

## Vision

Single binary. Zero external runtime APIs. Deploy in under 60 seconds. Useful analytics.

## Planned stack

- Go
- SQLite
- In-memory cache
- Local GeoIP database
- Lightweight web UI
- REST API
- Optional Litestream backups

## MVP

- Short link creation with custom or generated slugs
- Fast redirects with memory cache
- Click analytics
- Referrer, device, browser, timestamp
- Country analytics using a local offline IP database
- Link expiration by date or click count
- Dashboard with search, filtering, and charts
- REST API
- QR code generation
- CLI
- Docker deployment
- CI

## Docker deployment

Build and run the production-oriented image:

```bash
docker build -t shorty .
docker run -d --name shorty \
  -p 8080:8080 \
  -v shorty-data:/data \
  -e SHORTY_BASE_URL=http://localhost:8080 \
  shorty
```

Or use Docker Compose:

```bash
docker compose up -d --build
```

The container runs the single Shorty binary as a non-root user. SQLite is stored at `/data/shorty.db`; keep `/data` on a persistent volume. `SHORTY_BASE_URL` should be set to the public URL used for generated QR codes. `SHORTY_GEOIP_DB` can point to a mounted local GeoIP CSV when country analytics are enabled.

For complete production deployment, backup, update, and rollback procedures, see [DEPLOYMENT.md](DEPLOYMENT.md).

## CLI

The `shorty-cli` binary uses the same REST API as the web application, so it does not need direct access to the SQLite database. Build it with:

```bash
go build -o shorty-cli ./cmd/shorty-cli
```

Set `SHORTY_URL` when the server is not at the default `http://localhost:8080`:

```bash
export SHORTY_URL=https://short.example.com
```

Create a link:

```bash
./shorty-cli create --url https://example.com --slug docs
```

View analytics:

```bash
./shorty-cli stats --id 1 --interval day
```

See [cmd/shorty-cli/README.md](cmd/shorty-cli/README.md) for all CLI options and examples.

## QR codes

The API generates a PNG QR code for any existing link:

```text
GET /api/v1/links/{id}/qr
```

Optional `size` controls the requested image size. The server returns `image/png` and rejects invalid sizes with a `400` response. The dashboard's **Open QR** and per-link **QR** actions use this endpoint directly.

When `SHORTY_BASE_URL` is configured, that public URL is used as the QR destination. Otherwise the server derives the URL from the incoming request host and scheme.

## Local country database

Shorty can resolve visitor countries without calling a third-party geolocation API. Set `SHORTY_GEOIP_DB` to a local CSV file containing CIDR ranges and ISO country codes:

```csv
# CIDR,country code
1.0.0.0/8,AU
8.0.0.0/8,US
2001:db8::/32,ZZ
```

The database is loaded at startup and lookups happen locally during redirects. If `SHORTY_GEOIP_DB` is not set, country analytics remain empty and redirects continue normally.

The IP database itself is intentionally not bundled by Shorty; operators should supply a legally licensed dataset appropriate for their deployment.

## Roadmap

### v0.1.0 — MVP

Core link management, redirect engine, analytics, dashboard, API, CLI, Docker, and documentation.

### v0.2.0 — Differentiators

UTM builder, CSV import/export, custom domains, password-protected links, A/B redirects, teams and webhooks.

### v1.0.0 — Production

Hardening, observability, security review, performance testing, release automation, and production deployment documentation.

## Contributing

Issues and pull requests are welcome. See CONTRIBUTING.md for development and contribution guidelines.

## License

MIT
