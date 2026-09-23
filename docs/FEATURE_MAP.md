# Shorty Feature Map

Every capability in Shorty, mapped to its API surface, implementation,
tests, and documentation. If you want to know where something lives, how
it's protected, or how it's verified — start here.

Legend for the tables:

- **API** — HTTP route, served by `cmd/shorty`
- **CLI** — `shorty-cli` subcommand (`cmd/shorty-cli/`)
- **Code** — primary implementation file(s)
- **Tests** — test file(s) that verify it
- **Docs** — where it's documented for users/operators

---

## 1. Core features

| Feature | API | CLI | Code | Tests | Docs |
|---|---|---|---|---|---|
| Create link (custom slug, expiry, max clicks) | `POST /api/v1/links` | `shorty-cli create --url --slug --expires-at --max-clicks` | `internal/api/links.go`, `internal/links/sqlite_repository.go` | `internal/api/links_test.go` | README §MVP, `cmd/shorty-cli/README.md` §Create a link |
| List / search / filter links | `GET /api/v1/links` | — | `internal/api/links.go` | `internal/api/links_test.go` | README §MVP |
| Get / update / delete link | `GET·PUT·DELETE /api/v1/links/{id}` | — | `internal/api/links.go` | `internal/api/links_test.go` | README §MVP |
| Redirect (302) with click counting | `GET /{slug}`, `POST /{slug}` (unlock form) | — | `internal/redirect/redirect.go` | `internal/redirect/redirect_test.go` | README §MVP |
| Click analytics (timeseries, breakdowns) | `GET /api/v1/links/{id}/analytics` | `shorty-cli stats` | `internal/api/analytics.go`, `internal/analytics/` | `internal/analytics/*_test.go`, `internal/api/features_test.go` | README §MVP, `cmd/shorty-cli/README.md` §View stats |
| Geo breakdown (country) | part of analytics | — | `internal/geoip/geoip.go` | `internal/geoip/*_test.go` | README §Local country database |
| QR codes | `GET /api/v1/links/{id}/qr` | — | `internal/api/qr.go`, `internal/qr/qr.go` | `internal/api/qr_test.go` | README §QR codes |
| UTM builder | `POST /api/v1/utm` | — | `internal/api/utm.go`, `internal/utm/utm.go` | `internal/api/utm_test.go` | README §UTM builder |
| CSV import / export | `POST /api/v1/links/import`, `GET /api/v1/links/export` | `shorty-cli import`, `shorty-cli export` | `internal/api/csv.go`, `internal/cli/resources.go` | `internal/api/features_test.go` | README §CSV import/export |
| Password-protected links | `password` field on create/update; unlock via `POST /{slug}` form | `shorty-cli create --password`, `shorty-cli protect` | `internal/password/password.go`, `internal/redirect/redirect.go` | `internal/password/*_test.go`, `internal/redirect/redirect_features_test.go` | README §Password-protected links |
| A/B redirect variants (weighted) | `GET·POST /api/v1/links/{id}/variants`, `PUT·DELETE …/variants/{variant_id}` | `shorty-cli variant add\|list\|rm` | `internal/api/variants.go`, `internal/variants/variants.go` | `internal/api/features_test.go` | README §A/B testing |
| Custom domains (Host routing) | `GET·POST /api/v1/domains`, `DELETE /api/v1/domains/{id}`; `domain`/`domain_id` on links | `shorty-cli domain add\|list\|rm` | `internal/api/domains.go`, `internal/domains/`, `internal/redirect/redirect.go` (routing) | `internal/api/features_test.go` | README §Custom domains |
| Webhooks (`link.created`, `link.clicked`, `link.expired`) | `GET·POST /api/v1/webhooks`, `GET·PUT·DELETE /api/v1/webhooks/{id}`, `POST …/rotate-secret`, `GET …/deliveries` | `shorty-cli webhook add\|list\|rm\|deliveries` | `internal/api/webhooks.go`, `internal/webhooks/` | `internal/webhooks/*_test.go`, `internal/api/features_test.go` | README §Webhooks |
| Teams + API keys (admin-only management) | `GET·POST /api/v1/teams`, `GET·POST /api/v1/teams/{id}/keys`, `DELETE …/keys/{key_id}` | `shorty-cli team create\|list`, `shorty-cli key create\|list\|rm` | `internal/api/teams.go`, `internal/teams/` | `internal/api/features_test.go` | README §Teams and API keys |
| First-run admin bootstrap | — | `shorty-cli bootstrap-admin --team-id --name` | `cmd/shorty-cli/main.go` (`bootstrapAdmin`) | — | DEPLOYMENT.md §First-run admin key |
| Web dashboard | `GET /` | — | `internal/dashboard/` | `internal/dashboard/*_test.go` | README §MVP |

Auth model: requests carry `Authorization: Bearer <key>` or `X-API-Key: <key>`
(`internal/teams/` middleware, `internal/server/server.go`). Keyless requests
are scoped to the `Default` team. Team/key management is admin-only
(`api_keys.is_admin`, schema v7).

---

## 2. Security controls

Each control below was added or hardened during the security audit. "Threat"
is what it stops; "Code" is where it lives.

| Control | Threat | Code | Tests | Docs |
|---|---|---|---|---|
| Unlock rate limit — 10 attempts/min per client IP (fixed window, bounded state) | Password brute force; bcrypt CPU exhaustion | `internal/redirect/redirect.go` (`attemptLimiter`) | `TestUnlockRateLimited` (`internal/redirect/`) | README §Password-protected links |
| Unlock tokens carry server-validated expiry (24h); `Secure` flag on TLS | Cookie replay after capture | `internal/password/password.go` (`CookieSigner`), `internal/redirect/redirect.go` | `TestSignedCookieExpiry` | README §Password-protected links |
| Domain create/delete require an authenticated API key | Anonymous domain hijack / deletion breaking others' links | `internal/api/domains.go` | `TestDomainAPILifecycle` (401 cases) | README §Custom domains |
| Team keys can't delete admin keys unless admin | Privilege escalation by non-admin team key | `internal/api/teams.go` | `internal/api/features_test.go` | README §Teams and API keys |
| Request body caps — 1 MiB API, 32 MiB CSV import (`http.MaxBytesReader`) | Memory-exhaustion DoS via huge payloads | `internal/server/server.go` (`maxBodyLimit`) | `TestMaxBodyLimit` | — |
| Webhook SSRF protection — URL validation, public-IP-only dialer, redirect revalidation, TCP-only, SNI-preserving TLS | Server-side request forgery via webhook URLs | `internal/webhooks/` | `TestValidateURL` | README §Webhooks |
| Webhook signing secret shown once; rotation via `POST …/rotate-secret` | Secret leakage via API reads | `internal/api/webhooks.go`, `internal/webhooks/` | `internal/api/features_test.go` | README §Webhooks |
| `link.expired` emission cooldown — once per slug per 10 min | Webhook flood from hot-looped expired links | `internal/redirect/redirect.go` (`expiredEmitted`) | `TestExpiredEmissionCooldown` | — |
| DB directory `0700`, DB file `0600` | Credential/hash/PII exposure on shared hosts | `internal/database/database.go` | `TestOpenRestrictsPermissions` | DEPLOYMENT.md §Prerequisites |
| bcrypt password hashing (never returned by API) | Password disclosure | `internal/password/password.go` | `internal/password/*_test.go` | README §Password-protected links |
| Graceful shutdown — dispatcher drains before DB close; `ReadHeaderTimeout`/`ReadTimeout`/`IdleTimeout` | Slowloris; data loss on SIGINT/SIGTERM | `cmd/shorty/main.go`, `internal/server/server.go` | `internal/server/*_test.go` | DEPLOYMENT.md §Bare-metal deployment |
| Analytics recorder drops (never panics) after `Close`; dispatcher `Close` idempotent, `Emit` drops after close | Panic-on-closed-channel crashes during shutdown races | `internal/analytics/analytics.go`, `internal/webhooks/webhooks.go` | `TestRecordAfterCloseDoesNotPanic` | — |
| CSV import row cap (50,000); invalid `expires_at`/`max_clicks` reported, not silently dropped | Request-handler exhaustion; silent "never expires" misconfiguration | `internal/api/csv.go` | `internal/api/features_test.go` | README §CSV import/export |
| SQLite WAL mode + `busy_timeout`; `PRAGMA foreign_keys = ON` | Write contention; dangling references | `internal/database/database.go`, `internal/database/migrations.go` | — | — |

---

## 3. Performance optimizations

Measured on AMD EPYC 9D25 (2 cores), SQLite backend.

| Optimization | Effect | Code | Benchmark / Test |
|---|---|---|---|
| `IncrementClicks` as single `UPDATE … RETURNING` (was UPDATE + SELECT) | 1 DB round-trip per redirect instead of 2 | `internal/links/sqlite_repository.go` | `BenchmarkRedirectSQLite` |
| Shared bounded variant-list cache (5,000 entries) between API and redirect handler; API invalidates on create/update/delete | Redirects for links without variants skip the `List()` query entirely | `internal/variants/variants.go` (`ListCache`), `internal/redirect/redirect.go`, `internal/api/variants.go`, `internal/server/server.go` | `BenchmarkRedirectSQLite/with-list-cache`: **70,824 ns/op** vs `no-list-cache`: 162,284 ns/op (**2.3× faster**, ~14k redirects/s) |
| Bounded in-memory link cache (10,000 entries, eviction on overflow) | Hot slugs served without DB; memory can't grow unbounded | `internal/cache/cache.go` | `TestCacheBounded` |
| Analytics breakdowns aggregated in SQL (`GROUP BY`, capped at 10 referrers/countries, 100 user agents) | No per-click Go-side materialization of millions of rows | `internal/analytics/query.go` | `internal/analytics/*_test.go` |
| Locked RNG for A/B variant selection | Race-free weighted picks under concurrency | `internal/redirect/redirect.go`, `internal/variants/variants.go` | `go test -race` |
| Weighted-pick saturating addition | Adversarial weights can't overflow the `int64` total | `internal/variants/variants.go` (`Pick`) | `TestVariantPickSaturates` |

### Test-suite efficiency

| Technique | Effect | Code |
|---|---|---|
| bcrypt cost is a variable; tests use `bcrypt.MinCost` via `SetTestCost()` (production: `DefaultCost`) | `redirect` 2.16s → 0.05s, `password` 0.46s → 0.01s | `internal/password/password.go`, `TestMain` in `password`/`redirect` |
| Webhook retry backoff is a variable; tests use 1ms (production: 1s) | `webhooks` 1.07s → 0.07s | `internal/webhooks/webhooks.go`, `TestMain` in `webhooks` |
| Full suite `go test ./...` | ~4s → **<1s**; `go test -race ./...` 61s → **19s** | — |

---

## 4. Operations reference

| Item | Value / Location |
|---|---|
| Schema version | v7 (`PRAGMA user_version`, `internal/database/migrations.go`) |
| Database file | `SHORTY_DB` (default `shorty.db`); dir `0700`, file `0600` |
| Unlock cookie secret | `SHORTY_SECRET` (random per boot if unset — cookies don't survive restarts) |
| Server address / base URL / GeoIP DB | `SHORTY_ADDR`, `SHORTY_BASE_URL`, `SHORTY_GEOIP_DB` |
| CLI auth | `SHORTY_API_KEY` (`cmd/shorty-cli/README.md`) |
| Hard limits | Body 1 MiB / 32 MiB import · unlock 10/min/IP · CSV 50k rows · cache 10k links / 5k variant lists · webhook deliveries page ≤ 200 · `link.expired` 1 per slug per 10 min |
| Update semantics | `PUT /api/v1/links/{id}`: omitted `expires_at`/`max_clicks`/`domain_id` preserves; explicit `null` clears (README §Password-protected links) |
| Variant weight semantics | `0` = disabled (never picked); omitted preserves on update / defaults to `1` on create; `null` resets to `1` (README §A/B testing) |
| Deployment | `DEPLOYMENT.md` — prerequisites, first-run admin key, Docker / bare-metal, backups, updates, rollback, operational checklist |
| Roadmap | README §Roadmap — v0.1.0 MVP, v0.2.0 differentiators (implemented), v1.0.0 production |

---

## 5. How the map was built

This document was researched from the repository at `~/workspace/shorty`
(schema v7, uncommitted audit + optimization work as of 2026-09-23):

- Routes enumerated from `mux.Handle` registrations in `internal/api/*.go`
  and `internal/server/server.go` — no endpoint listed here is invented.
- Controls and optimizations cross-checked against the audit fix list and
  the benchmark output above.
- When code and this map disagree, the code wins — update the map with it.
