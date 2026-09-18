# Shorty

A self-hosted URL shortener focused on simplicity, speed, privacy-friendly analytics, and easy deployment.

## Vision

Single binary. Zero external dependencies. Deploy in under 60 seconds. Useful analytics.

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
- Rough country analytics using a local IP database
- Link expiration by date or click count
- Dashboard with search, filtering, and charts
- REST API
- QR code generation
- CLI
- Docker deployment
- CI

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
