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

## UTM builder

Build a campaign URL without modifying the original destination through:

```text
POST /api/v1/utm
Content-Type: application/json
```

Example request:

```json
{
  "url": "https://example.com/docs?ref=home",
  "utm_source": "newsletter",
  "utm_medium": "email",
  "utm_campaign": "fall-launch",
  "utm_term": "short links",
  "utm_content": "hero button"
}
```

The response is JSON containing the generated URL. Existing query parameters are preserved, UTM values are URL-encoded, and empty UTM fields are ignored. Only absolute `http` and `https` URLs are accepted.

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

## Password-protected links

Protect a link with a password at creation time:

```json
POST /api/v1/links
{ "url": "https://example.com", "slug": "secret", "password": "s3cretpw" }
```

Passwords are stored as bcrypt hashes and never returned by the API. Visitors hitting the short URL see an unlock form; a correct password sets a signed, HTTP-only cookie (`shorty_unlock_<slug>`, 24h) and redirects. Update with `{"password": "..."}` to change it or `{"password": ""}` to remove protection. Clicks are only counted after a successful unlock.

On `PUT /api/v1/links/{id}`, an omitted `expires_at`, `max_clicks`, or `domain_id` leaves the current value untouched; send an explicit `null` to clear it. `domain`/`domain_id` must belong to your team. Unlock attempts are rate-limited to 10/minute per client; the signed unlock token also expires server-side (24h), so a captured token cannot be replayed forever.

Set `SHORTY_SECRET` to a stable secret so unlock cookies survive restarts; otherwise a random secret is generated at startup.

## A/B testing

Give a link multiple weighted destinations and let Shorty split traffic:

```text
POST /api/v1/links/{id}/variants      {"url": "https://b.example", "weight": 3}
GET  /api/v1/links/{id}/variants
PUT  /api/v1/links/{id}/variants/{variant_id}
DELETE /api/v1/links/{id}/variants/{variant_id}
```

Each redirect picks a variant by weighted random selection and increments both the link and the variant click counters, so you can compare performance per variant. A `weight` of `0` disables a variant (it is never picked); omitting `weight` keeps the current value on update (defaults to `1` on create), and an explicit `null` resets it to `1`.

## Custom domains

Serve short links from your own domains:

```text
POST /api/v1/domains        {"domain": "go.example.com"}   (requires API key)
GET  /api/v1/domains
DELETE /api/v1/domains/{id}                  (requires API key)
```

Point the domain's DNS at Shorty, register it, then assign links with `{"domain": "go.example.com"}` or `{"domain_id": 1}` on create/update. The redirect handler routes by the request `Host` header: `go.example.com/promo` resolves only links assigned to that domain.

## Webhooks

Get notified when links are created or clicked:

```text
POST /api/v1/webhooks        {"url": "https://example.com/hook", "events": ["link.created", "link.clicked"]}  # team-scoped
GET  /api/v1/webhooks
PUT  /api/v1/webhooks/{id}   {"active": false}
POST /api/v1/webhooks/{id}/rotate-secret   # new secret shown once
DELETE /api/v1/webhooks/{id}
GET  /api/v1/webhooks/{id}/deliveries
```

Events: `link.created`, `link.clicked`, `link.expired`. Deliveries are async with one retry, signed with `X-Shorty-Signature: sha256=<hmac>` (HMAC-SHA256 over the body), and every attempt is logged under `/deliveries`. Webhook URLs are SSRF-hardened: only absolute `http(s)` URLs whose host resolves to a public IP are accepted (no private/loopback/link-local addresses, no embedded credentials), and deliveries dial resolved public IPs directly, so DNS-rebinding can't redirect a payload into your private network. The signing secret is shown exactly once — in the create response (or after rotation) — and never on reads.

## Teams and API keys

Links, domains, and webhooks belong to teams. Pass an API key as `Authorization: Bearer <key>` or `X-API-Key: <key>`; requests without a key use the `Default` team. Managing teams and API keys is admin-only.

```text
POST /api/v1/teams                 {"name": "Acme"}                       # admin only
GET  /api/v1/teams                                                        # admin only
POST /api/v1/teams/{id}/keys       {"name": "ci", "admin": true}          # key shown once; admin:true needs an admin key
GET  /api/v1/teams/{id}/keys
DELETE /api/v1/teams/{id}/keys/{key_id}
```

Keys are stored as SHA-256 hashes; only the plaintext returned at creation can authenticate. The CLI reads `SHORTY_API_KEY` automatically.

**Bootstrap:** on a fresh database there are no admin keys yet, and team management requires one. Create the first admin key directly against the database:

```text
shorty-cli bootstrap-admin --name ops [--team-id 1] [--db shorty.db]
```

It prints the key once — export it as `SHORTY_API_KEY` and use it to manage teams and mint further keys.

## CSV import/export

```text
GET  /api/v1/links/export     # text/csv download
POST /api/v1/links/import     # multipart form, field name "file"
```

The import CSV needs a `url` column; `slug`, `expires_at` (RFC3339 or `YYYY-MM-DD`), and `max_clicks` are optional. The response reports `{created, skipped, errors}` per row.

## Roadmap

### v0.1.0 — MVP

Core link management, redirect engine, analytics, dashboard, API, CLI, Docker, and documentation.

### v0.2.0 — Differentiators (implemented)

UTM builder, CSV import/export, custom domains, password-protected links, A/B redirects, teams and webhooks.

### v1.0.0 — Production

Hardening, observability, security review, performance testing, release automation, and production deployment documentation.

## Feature map

[docs/FEATURE_MAP.md](docs/FEATURE_MAP.md) maps every feature, security
control, and performance optimization to its API routes, implementation,
tests, and docs — the fastest way to find where something lives.

## Contributing

Issues and pull requests are welcome. See CONTRIBUTING.md for development and contribution guidelines.

## License

MIT
