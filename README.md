# Goalden API

The Go REST API backend for [Goalden](https://github.com/mavalu22/goalden-front) — a minimal daily and weekly task management app.

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

Fill in `.env`:

```
DATABASE_URL=postgres://user:pass@localhost:5432/goalden
REDIS_URL=redis://localhost:6379
SUPABASE_JWT_SECRET=your-supabase-jwt-secret
PORT=8080
```

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
- [RELEASE.md](RELEASE.md) — Deploy and migration instructions
