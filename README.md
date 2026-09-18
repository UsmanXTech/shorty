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
