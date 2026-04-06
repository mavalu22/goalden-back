# Goalden Backend — Release Guide

## Prerequisites

- Go 1.22+
- Docker + Docker Compose (for local dev)
- `sqlc` for query generation
- `golang-migrate` for running migrations
- `.env` file with required environment variables (see `.env.example` if present)

## Local development

```bash
docker-compose up -d        # start Postgres + Redis
make run                    # start the API server
```

## Build

```bash
make build
# Output: bin/server
```

## Release

### Docker build

```bash
docker build -t goalden-api .
```

### Deploy (Railway)

Push to `main` — Railway auto-deploys from the Docker image on merge.

Required environment variables on Railway:
- `DATABASE_URL` — Supabase PostgreSQL connection string
- `REDIS_URL` — Upstash Redis URL
- `SUPABASE_JWT_SECRET` — from Supabase project settings
- `PORT` — default `8080`

### Running migrations

```bash
make migrate-up             # apply all pending migrations
make migrate-down           # rollback last migration
```

> Never deploy without running migrations first against the target database.
