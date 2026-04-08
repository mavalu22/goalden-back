# Goalden API

The Go REST API backend for [Goalden](https://github.com/mavalu22/goalden-front) — a minimal daily and weekly task management app.

## Engineering highlights

- **Bidirectional sync** — single `POST /tasks/sync` endpoint handles full two-way task synchronization with last-write-wins conflict resolution based on `updated_at`
- **Offline-first** — the Flutter client writes to SQLite first and syncs in the background; the server only sees net changes
- **Soft-delete tombstoning** — deletions are propagated across devices via `deleted_at` timestamps rather than hard deletes
- **Batch operations** — `BatchUpsertTasks` uses a single PostgreSQL `INSERT … SELECT * FROM unnest(…)` to avoid N-round-trips; `BatchDeleteTasks` uses `UPDATE … WHERE id = ANY($1)`
- **In-memory JWT cache** — 5-minute TTL cache eliminates per-request Supabase auth calls (~40–50ms saved per request)
- **Auto-migration on startup** — embedded SQL migrations run automatically at boot; no external migration tool required
- **Regression test coverage** — including soft-delete sync tests for recurring task deletion to prevent silent re-creation on other devices

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the full system design.

---

## Tech stack

| Layer | Technology |
|---|---|
| Language | Go 1.22+ |
| HTTP framework | Chi v5 |
| Database | PostgreSQL 16 (Supabase) |
| Query layer | Hand-written SQL via pgx |
| Auth | Supabase JWT validation (in-memory cache) |
| Containerization | Docker + Docker Compose (local-only, optional) |
| Hosting | Railway |

---

## Architecture

Clean Architecture with layered separation:

```
cmd/server/         # Entry point
internal/
├── config/         # Environment config loading
├── database/       # Database connection and embedded migrations
├── handler/        # HTTP handlers (Chi routes)
├── middleware/      # Auth (JWT validation), CORS
├── model/          # Domain models
├── repository/     # Data access layer (hand-written SQL via pgx)
├── server/         # HTTP server setup and routing
└── service/        # Business logic layer
```

---

## Local setup

### Prerequisites

- Go 1.22+
- A Supabase project (for auth and cloud database)
- `make` (optional — all commands are simple `go` invocations if `make` is unavailable)

### 1. Clone and install dependencies

```bash
git clone https://github.com/mavalu22/goalden-back
cd goalden-back
go mod download
```

### 2. Configure environment

```bash
cp .env.example .env
```

Fill in `.env` with your Supabase credentials. See `.env.example` for descriptions of each variable.

**Required variables:**

| Variable | Where to find it |
|----------|-----------------|
| `DATABASE_URL` | Supabase dashboard → Project Settings → Database → Connection string (URI) |
| `SUPABASE_URL` | Supabase dashboard → Project Settings → API → Project URL |
| `SUPABASE_SERVICE_ROLE_KEY` | Supabase dashboard → Project Settings → API → service_role key |

> The `SUPABASE_SERVICE_ROLE_KEY` is a server-side secret. Never expose it in client code or commit it to version control.

### 3. Build and run

```bash
make build
./bin/goalden-api
```

Or directly with Go:

```bash
go build -o bin/goalden-api ./cmd/server
./bin/goalden-api
```

The server connects to the Supabase PostgreSQL database and **runs all embedded SQL migrations automatically at startup** before accepting requests. No external migration tool is needed for the common Supabase-hosted setup.

### Local Postgres + Redis (optional)

The steps above connect to remote Supabase. If you want a fully local setup instead:

```bash
docker compose up -d    # starts local Postgres + Redis containers
```

Then set `DATABASE_URL` in `.env` to the local Postgres connection string.

> `REDIS_URL` is present in config but the server uses in-memory JWT caching — a local Redis instance is not required.

---

## Available commands

| Command | Description |
|---|---|
| `make build` | Compile to `bin/goalden-api` |
| `make dev` | Run with live reload (requires [air](https://github.com/air-verse/air)) |
| `make test` | Run all tests with race detector |
| `make lint` | Run golangci-lint |
| `make migrate-up` | Apply migrations via golang-migrate (optional — server auto-migrates on startup) |
| `make migrate-down` | Rollback last migration via golang-migrate |
| `make docker-up` | Start local Postgres + Redis containers |
| `make docker-down` | Stop containers |
| `make docker-build` | Build production Docker image |

---

## Authentication

All authenticated endpoints require a `Bearer` token in the `Authorization` header. The token is a Supabase JWT — validated against Supabase's `/auth/v1/user` endpoint. Validated tokens are cached in-memory for 5 minutes to avoid redundant Supabase API calls. The user ID is extracted from the validated token and used to scope all data operations.

---

## Related

- [goalden-front](https://github.com/mavalu22/goalden-front) — Flutter frontend
- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — Full system architecture and sync protocol
- [docs/RELEASE.md](docs/RELEASE.md) — Build, deployment, and migration instructions
