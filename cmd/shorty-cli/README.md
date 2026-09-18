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
