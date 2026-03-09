# FB Spend Tracker

A self-hosted tool for collecting and monitoring Facebook Ads spend data across
multiple ad accounts and access tokens.

## Overview

Farmers provide Facebook access tokens (each covering multiple Business Managers
and ad accounts). The service discovers all ad accounts accessible via each
token, fetches daily spend data, and presents it through a web dashboard.

## Features

- **Multi-token support** — manage tokens from multiple farmers
- **Account discovery** — automatically discovers ad accounts via `/me/adaccounts`
- **Scheduled sync** — yesterday's spend at 10:00 AM, today's spend every 2 hours
- **Rate limit handling** — respects Facebook API throttle headers per token
- **Token encryption** — access tokens stored with AES-256 encryption
- **Web UI** — dashboard, token CRUD, account list, spend table with filters
- **Dark theme** — dark / dark-electric / dark-cyber
- **Authentication** — simple username/password login (HMAC-signed session cookie)
- **SQLite + PostgreSQL** — SQLite for dev, PostgreSQL for production (same codebase)

## Requirements

- Go 1.22+
- SQLite (dev) or PostgreSQL (prod)

## Getting Started

### 1. Clone and build

```sh
git clone <repo>
cd facebook-account-parser
go build -o fb-spend ./cmd/server
```

### 2. Create config

```sh
cp config.default.yml config.yml
```

Edit `config.yml`:

```yaml
db:
  dsn: "./data.db"              # SQLite (dev)
  # dsn: "postgres://user:pass@host:5432/dbname?sslmode=disable"  # PostgreSQL

security:
  encryption_key: "<output of: openssl rand -hex 32>"

auth:
  username: "admin"
  password: "your-password"
```

### 3. Run

```sh
./fb-spend -config config.yml
```

The server starts on `:8080` by default. Open `http://localhost:8080`.

## Configuration

| Key | Default | Description |
|-----|---------|-------------|
| `server.addr` | `:8080` | Listen address |
| `db.dsn` | `./data.db` | SQLite file path or PostgreSQL DSN |
| `facebook.api_version` | `v20.0` | Facebook Graph API version |
| `sync.schedule_yesterday` | `0 10 * * *` | Cron for yesterday's spend sync |
| `sync.schedule_today` | `0 */2 * * *` | Cron for today's spend sync |
| `security.encryption_key` | — | 64 hex chars (32 bytes), **required** |
| `auth.username` | `admin` | Web UI username |
| `auth.password` | — | Web UI password (leave empty to disable auth) |

Generate encryption key:

```sh
openssl rand -hex 32
```

## Project Structure

```
cmd/server/         — entry point
internal/
  config/           — YAML config loader
  db/               — store interface, SQLite/PostgreSQL impl, migrations
  facebook/         — Graph API client (accounts, insights, rate limits)
  token/            — token manager, quota tracking
  queue/            — goroutine worker pool with per-token semaphore
  sync/             — pipeline (discover → fetch), cron scheduler
  web/              — HTTP handlers, HTML templates, static assets
```

## Sync Schedule

| Job | Default schedule | What it does |
|-----|-----------------|--------------|
| Yesterday | `0 10 * * *` | Rediscover accounts + sync previous day |
| Today | `0 */2 * * *` | Sync current day spend |

Both schedules are configurable via `config.yml`.

## Facebook API Notes

- Requires a token with `ads_read` scope and Marketing API access
- Rate limits tracked via `X-App-Usage`, `X-Business-Use-Case-Usage`,
  and `X-FB-Ads-Insights-Throttle` headers
- Error codes: `4` → app-level, `17` → token, `80000/80004` → ad account

## License

MIT
