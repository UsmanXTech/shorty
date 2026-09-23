# Shorty CLI

Small REST client for creating links and viewing analytics without exposing a database connection.

## Build

```bash
go build -o shorty-cli ./cmd/shorty-cli
```

## Server URL

Set `SHORTY_URL` when the Shorty server is not running at `http://localhost:8080`.

```bash
export SHORTY_URL=https://short.example
```

## Create a link

```bash
./shorty-cli create --url https://example.com --slug docs
```

Optional expiration and click limit:

```bash
./shorty-cli create --url https://example.com --expires-at 2026-12-31T23:59:59Z --max-clicks 100
```

## View stats

```bash
./shorty-cli stats --id 1
./shorty-cli stats --id 1 --interval day
./shorty-cli stats --id 1 --from 2026-09-01T00:00:00Z --to 2026-09-18T00:00:00Z --interval day
```

The CLI prints the same JSON response returned by the REST API, making it suitable for shell scripts and other automation.

## API keys

Set `SHORTY_API_KEY` to act as a team; without it the CLI uses the server's `Default` team.

```bash
export SHORTY_API_KEY=shrt_...
```

## Password-protected links

```bash
./shorty-cli create --url https://example.com --password s3cretpw
./shorty-cli protect --id 1 --password newpw   # empty --password clears protection
```

## CSV import/export

```bash
./shorty-cli export --file links.csv
./shorty-cli import --file links.csv
```

## A/B testing

```bash
./shorty-cli variant add --id 1 --url https://b.example --weight 3
./shorty-cli variant list --id 1
./shorty-cli variant rm --id 1 --variant-id 2
```

## Custom domains

```bash
./shorty-cli domain add --domain go.example.com
./shorty-cli domain list
./shorty-cli create --url https://example.com --domain go.example.com
./shorty-cli domain rm --id 1
```

## Webhooks

```bash
./shorty-cli webhook add --url https://example.com/hook --events link.created,link.clicked
./shorty-cli webhook list
./shorty-cli webhook deliveries --id 1
./shorty-cli webhook rm --id 1
```

## Teams and API keys

Team management requires an admin API key. On a fresh database, mint the first one directly:

```bash
./shorty-cli bootstrap-admin --name ops   # admin key printed once, needs DB file access
export SHORTY_API_KEY=<the key>
```

```bash
./shorty-cli team create --name Acme
./shorty-cli team list
./shorty-cli key create --team-id 2 --name ci [--admin]   # key printed once; --admin needs an admin key
./shorty-cli key list --team-id 2
./shorty-cli key rm --team-id 2 --id 1
```
