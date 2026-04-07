# Goalden API

The Go REST API backend for [Goalden](https://github.com/mavalu22/goalden-front) — a minimal daily and weekly task management app.

## Engineering highlights

- **Bidirectional sync** — single `POST /tasks/sync` endpoint handles full two-way task synchronization with last-write-wins conflict resolution based on `updated_at`
- **Offline-first** — the Flutter client writes to SQLite first and syncs in the background; the server only sees net changes
- **Soft-delete tombstoning** — deletions are propagated across devices via `deleted_at` timestamps rather than hard deletes
- **Batch operations** — `BatchUpsertTasks` uses a single PostgreSQL `INSERT … SELECT * FROM unnest(…)` to avoid N-round-trips; `BatchDeleteTasks` uses `UPDATE … WHERE id = ANY($1)`
- **JWT token cache** — in-memory cache with 5-minute TTL eliminates per-request Supabase auth calls (~40–50ms saved per request)
- **Regression test coverage** — including soft-delete sync tests for recurring task deletion to prevent silent re-creation on other devices

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the full system design.

---

## Tech stack

| Layer | Technology |
|---|---|
| Language | Go 1.22+ |
| HTTP framework | Chi v5 |
| Database | PostgreSQL 16 (Supabase) |
| Query generation | sqlc |
| Migrations | golang-migrate |
| Cache | Redis (Upstash in production) |
| Auth | Supabase JWT validation |
| Containerization | Docker + Docker Compose |
| Hosting | Railway |

---

## Architecture

Clean Architecture with layered separation:

```
cmd/server/         # Entry point
internal/
├── config/         # Environment config loading
├── database/       # Database connection setup
├── dto/            # Request/response data transfer objects
├── handler/        # HTTP handlers (Chi routes)
├── middleware/      # Auth (JWT validation), logging, CORS
├── model/          # Domain models
├── pkg/            # Shared utilities
├── repository/     # Data access layer (sqlc-generated queries)
├── server/         # HTTP server setup and routing
└── service/        # Business logic layer
sql/
├── migrations/     # Versioned SQL migration files
└── queries/        # sqlc query definitions
```

---

## Local setup

### Prerequisites

- Go 1.22+
- Docker + Docker Compose
- [`golang-migrate`](https://github.com/golang-migrate/migrate)
- [`sqlc`](https://sqlc.dev) (only needed if modifying queries)

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

### 3. Start local services

```bash
make docker-up      # starts Postgres + Redis via Docker Compose
```

### 4. Run migrations

```bash
make migrate-up
```

### 5. Run the server

```bash
make build
./bin/goalden-api

# Or with live reload (requires air)
make dev
```

---

## Available commands

| Command | Description |
|---|---|
| `make build` | Compile to `bin/goalden-api` |
| `make dev` | Run with live reload (air) |
| `make test` | Run all tests with race detector |
| `make lint` | Run golangci-lint |
| `make migrate-up` | Apply all pending migrations |
| `make migrate-down` | Rollback last migration |
| `make sqlc` | Regenerate type-safe query code |
| `make docker-up` | Start Postgres + Redis containers |
| `make docker-down` | Stop containers |
| `make docker-build` | Build production Docker image |

---

## Authentication

All authenticated endpoints require a `Bearer` token in the `Authorization` header. The token is a Supabase JWT — validated against Supabase's JWKS endpoint. The user ID is extracted from the validated token and used to scope all data operations.

---

## Related

- [goalden-front](https://github.com/mavalu22/goalden-front) — Flutter frontend
- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — Full system architecture and sync protocol
- [RELEASE.md](RELEASE.md) — Build, deployment, and migration instructions
