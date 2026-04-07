# Goalden Backend — Release Guide

## Prerequisites

- Go 1.22+
- A Supabase project for auth and cloud database
- `make` (optional — commands are documented as plain `go` equivalents where needed)

**Local-only / optional tools:**

| Tool | When needed |
|------|-------------|
| Docker + Docker Compose | Only if running a fully local Postgres instance instead of Supabase |
| [`golang-migrate`](https://github.com/golang-migrate/migrate) | Only if running migrations manually; the server auto-migrates on startup |
| `sqlc` | Only when modifying SQL queries |
| [`air`](https://github.com/air-verse/air) | Only for live reload during local development (`make dev`) |

---

## Local development

The server connects to Supabase PostgreSQL by default. Configure `.env` and run:

```bash
go mod download
cp .env.example .env     # fill in DATABASE_URL, SUPABASE_URL, SUPABASE_SERVICE_ROLE_KEY
make build
./bin/goalden-api
```

The server **automatically runs all embedded SQL migrations at startup** before accepting requests. No separate migration step is needed for the common Supabase-hosted setup.

### Optional: fully local Postgres

If you want to run against a local Postgres instance instead of Supabase:

```bash
docker compose up -d     # start local Postgres + Redis containers
```

Then set `DATABASE_URL` in `.env` to the local connection string and run the server as above.

> Manual migrations via `make migrate-up` are also available if you need to apply or roll back schema changes explicitly.

### Live reload

```bash
make dev     # requires air — https://github.com/air-verse/air
```

---

## Build

```bash
make build
# Output: bin/goalden-api

# Or directly with Go:
go build -o bin/goalden-api ./cmd/server
```

---

## Release

### Docker build

```bash
make docker-build
# Equivalent: docker build -t goalden-api .
```

The Dockerfile uses a multi-stage build:
- Builder stage: `golang:1.22-alpine` — compiles the binary
- Runtime stage: `alpine:3.19` — minimal image with only the binary

### Deploy (Railway)

Goalden is deployed on [Railway](https://railway.app) via the Docker image.
Railway auto-deploys on push to `main` when connected to the repository.

**Required environment variables on Railway:**

| Variable | Description |
|----------|-------------|
| `DATABASE_URL` | Supabase PostgreSQL connection string — use the **Transaction** pooler URL for Railway |
| `SUPABASE_URL` | Supabase project URL (e.g. `https://<ref>.supabase.co`) |
| `SUPABASE_SERVICE_ROLE_KEY` | Supabase service role key (server-side secret) |

**Optional variables:**

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | Railway sets this automatically |
| `ENV` | `development` | Set to `production` on Railway |
| `SUPABASE_ANON_KEY` | — | Public anon key (not required server-side) |
| `ALLOWED_ORIGINS` | `http://localhost:3000` | Set to your frontend domain in production |

> Do not expose `SUPABASE_SERVICE_ROLE_KEY` or `DATABASE_URL` in client-side code.

---

## Supabase setup

1. Create a new Supabase project
2. Enable the auth providers you need (Email, Google, Apple) in Authentication → Providers
3. Get your `DATABASE_URL` from Project Settings → Database → Connection string
4. Get your `SUPABASE_URL` and keys from Project Settings → API

The server applies all migrations automatically on first startup — no manual migration step is needed after deploy.

---

## Running migrations manually

```bash
make migrate-up      # apply all pending migrations (requires golang-migrate)
make migrate-down    # rollback last migration
```

> Manual migration is optional. The server runs all embedded migrations on startup automatically. Use manual migration only when you need explicit control over which migrations are applied.
