# Goalden Backend — Release Guide

## Prerequisites

- Go 1.22+
- Docker + Docker Compose (for local dev with Postgres + Redis)
- [`golang-migrate`](https://github.com/golang-migrate/migrate) for running migrations
- `sqlc` (only needed when modifying SQL queries)
- A Supabase project for auth and cloud database

---

## Local development

```bash
docker-compose up -d        # start Postgres + Redis
make migrate-up             # apply migrations against DATABASE_URL
make dev                    # start the API server with live reload (requires air)
```

Or without live reload:

```bash
make build
./bin/goalden-api
```

---

## Build

```bash
make build
# Output: bin/goalden-api
```

---

## Release

### Docker build

```bash
docker build -t goalden-api .
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
| `REDIS_URL` | Upstash Redis URL (from Railway Redis add-on or Upstash) |
| `SUPABASE_URL` | Supabase project URL (e.g. `https://<ref>.supabase.co`) |
| `SUPABASE_SERVICE_ROLE_KEY` | Supabase service role key (server-side secret) |

**Optional variables:**

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | Railway sets this automatically |
| `ENV` | `development` | Set to `production` on Railway |
| `SUPABASE_ANON_KEY` | — | Public anon key (not used server-side but may be logged) |
| `ALLOWED_ORIGINS` | `http://localhost:3000` | Set to your frontend domain in production |

> Do not expose `SUPABASE_SERVICE_ROLE_KEY` or `DATABASE_URL` in client-side code.

---

## Supabase setup

1. Create a new Supabase project
2. Enable the auth providers you need (Email, Google, Apple) in Authentication → Providers
3. Get your `DATABASE_URL` from Project Settings → Database → Connection string
4. Get your `SUPABASE_URL` and keys from Project Settings → API

### Running migrations against Supabase

Migrations live in `sql/migrations/`. Apply them against the Supabase database before first deploy:

```bash
export DATABASE_URL="<your-supabase-connection-string>"
make migrate-up
```

> Never deploy a new version without running migrations first if the release includes schema changes.

---

## Running migrations

```bash
make migrate-up             # apply all pending migrations
make migrate-down           # rollback last migration
```
