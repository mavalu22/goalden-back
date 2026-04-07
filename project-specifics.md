# Goalden Backend — Project Specifics

## Tech Stack

### Core
- **Language:** Go 1.22+ (latest stable)
- **HTTP Framework:** Chi (v5) — stdlib-compatible, minimal overhead, composable middleware
- **Protocol:** REST API (JSON over HTTP)
- **Architecture:** Clean Architecture with layered separation

### Database
- **Primary database:** PostgreSQL 16+ (hosted on Supabase)
- **Driver:** pgx (v5) — high-performance, pure Go PostgreSQL driver
- **Migrations:** Embedded SQL files in `internal/database/migrations/` — run automatically at startup
- **Query generation:** sqlc configured (`sqlc.yaml`) for type-safe query generation when modifying SQL queries

### Authentication
- **Supabase Auth** handles all user authentication (Google, Apple, Email/Password)
- The Go backend does NOT handle login flows directly
- The backend **validates Supabase JWTs** on every authenticated request via Supabase's `/auth/v1/user` endpoint
- Validated tokens are cached **in-memory** with a 5-minute TTL — no external cache required
- User ID is extracted from the validated JWT and used to scope all data operations

### Infrastructure
- **Hosting:** Railway (Docker-based deploy)
- **Database hosting:** Supabase PostgreSQL (same project used for Auth)
- **Local development:** Docker Compose provides a local Postgres instance (optional — remote Supabase works directly)

---

## Project Architecture

### Folder Structure

```
goalden-api/
├── cmd/
│   └── server/
│       └── main.go                    # Entry point — connects DB, runs migrations, starts server
├── internal/
│   ├── config/
│   │   └── config.go                  # Environment config loading (.env + OS env)
│   ├── database/
│   │   ├── database.go                # PostgreSQL connection pool (pgxpool)
│   │   ├── migrate.go                 # Embedded migration runner (runs at startup)
│   │   └── migrations/
│   │       ├── 001_initial.sql        # Initial schema: users + tasks tables
│   │       └── 002_sync_fields.sql    # Sync metadata: source_task_id, time fields, deleted_at
│   ├── server/
│   │   ├── server.go                  # HTTP router setup, middleware stack, route wiring
│   │   └── cors.go                    # CORS middleware
│   ├── middleware/
│   │   └── auth.go                    # JWT validation middleware (Supabase) + in-memory token cache
│   ├── handler/
│   │   ├── auth_handler.go            # POST /api/v1/auth/sync-user
│   │   ├── task_handler.go            # GET /tasks, POST /tasks/sync, DELETE /tasks/{id}
│   │   ├── health_handler.go          # GET /health
│   │   └── helpers.go                 # Shared handler utilities
│   ├── service/
│   │   └── task_service.go            # Business logic layer
│   ├── repository/
│   │   ├── task_repository.go         # Repository interface
│   │   ├── user_repository.go         # Repository interface
│   │   └── postgres/
│   │       ├── task_repository.go     # PostgreSQL task implementation
│   │       └── user_repository.go     # PostgreSQL user implementation
│   ├── model/
│   │   ├── task.go                    # Task domain model
│   │   └── user.go                    # User domain model
│   └── pkg/                           # Internal shared utilities (scaffolded)
│       ├── errs/
│       ├── response/
│       └── validator/
├── sql/
│   ├── migrations/                    # sqlc schema source (mirrors internal/database/migrations)
│   └── queries/                       # sqlc query source files
├── sqlc.yaml                          # sqlc configuration
├── Dockerfile
├── docker-compose.yml                 # Local dev: Postgres + Redis containers
├── .env.example
├── Makefile
├── go.mod
└── go.sum
```

### Architecture Pattern

- **Clean Architecture** with 3 layers: handler → service → repository
- **Handler layer:** Receives HTTP requests, decodes JSON, calls service, writes JSON responses
- **Service layer:** Contains business logic. No HTTP dependencies. Operates on domain models
- **Repository layer:** Data access only. Hand-written SQL via pgx. No ORM
- Dependencies flow inward: handler → service → repository → database
- Chi built-in middleware handles logging, request IDs, panic recovery

---

## API

### Base URL
```
/api/v1
```

### Endpoints

#### Health
| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/health` | None | Liveness check — pings the database |

#### Auth
| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `POST` | `/api/v1/auth/sync-user` | Bearer | Register or update user record after Supabase login |

#### Tasks
| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/api/v1/tasks` | Bearer | Pull all non-deleted tasks for the user (full pull, new device) |
| `POST` | `/api/v1/tasks/sync` | Bearer | Bidirectional sync — push local changes, receive server changes |
| `DELETE` | `/api/v1/tasks/{id}` | Bearer | Soft-delete a single task |

All authenticated endpoints require `Authorization: Bearer <supabase_jwt>`. Unauthenticated requests receive `401 Unauthorized`.

### Sync endpoint contract

```
POST /api/v1/tasks/sync

Request:
{
  "tasks":        [ <Task>, ... ],         // tasks created/modified locally
  "deleted_ids":  ["id1", "id2"],          // task IDs deleted locally
  "last_sync_at": "2024-01-01T00:00:00Z"  // zero value for first sync
}

Response:
{
  "tasks":       [ <Task>, ... ],          // tasks updated on server since last_sync_at
  "deleted_ids": ["id1", "id2"]           // task IDs deleted on server since last_sync_at
}
```

### Task object shape

```json
{
  "id":                 "string (client-generated UUID)",
  "user_id":            "string (Supabase auth user ID)",
  "title":              "string",
  "date":               "YYYY-MM-DD",
  "priority":           "normal | high",
  "note":               "string | null",
  "done":               true | false,
  "recurrence":         "none | daily | weekly | custom_days",
  "recurrence_days":    "[1,3,5] (JSON string) | null",
  "source_task_id":     "string | null",
  "sort_order":         0,
  "start_time_minutes": 480,
  "end_time_minutes":   540,
  "created_at":         "ISO 8601",
  "updated_at":         "ISO 8601",
  "completed_at":       "ISO 8601 | null",
  "deleted_at":         "ISO 8601 | null"
}
```

---

## Database Schema (PostgreSQL)

### Users Table
```sql
CREATE TABLE IF NOT EXISTS users (
    id         TEXT PRIMARY KEY,    -- Supabase auth user ID (UUID as text)
    email      TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### Tasks Table
```sql
CREATE TABLE IF NOT EXISTS tasks (
    id               TEXT PRIMARY KEY,         -- UUID generated client-side
    user_id          TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title            TEXT NOT NULL,
    date             DATE NOT NULL,
    priority         TEXT NOT NULL DEFAULT 'normal',
    note             TEXT,
    done             BOOLEAN NOT NULL DEFAULT FALSE,
    recurrence       TEXT NOT NULL DEFAULT 'none',
    recurrence_days  TEXT,                     -- JSON array of ints, e.g. "[1,3,5]"
    sort_order       INTEGER NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at     TIMESTAMPTZ,
    -- Added in migration 002:
    source_task_id   TEXT REFERENCES tasks(id) ON DELETE SET NULL,
    start_time_minutes INTEGER,
    end_time_minutes   INTEGER,
    deleted_at       TIMESTAMPTZ
);
```

---

## Key Packages

| Package | Purpose |
|---------|---------|
| `github.com/go-chi/chi/v5` | HTTP router and middleware (logging, recovery, request ID built-in) |
| `github.com/jackc/pgx/v5` | PostgreSQL driver and connection pool (pgxpool) |
| `github.com/joho/godotenv` | Load `.env` file in development |

---

## Coding Conventions

### Go Style
- Follow the official [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments)
- Run `gofmt` and `go vet` before every commit
- Use `golangci-lint` for comprehensive linting
- Keep functions short and focused — if it's longer than 40 lines, consider splitting

### Naming Conventions
- Files: `snake_case.go`
- Packages: short, lowercase, single word when possible
- Exported types/functions: `PascalCase`
- Unexported types/functions: `camelCase`
- Interfaces: named by what they do (e.g., `TaskRepository`, not `ITaskRepository`)
- Constructors: `NewXxx` pattern (e.g., `NewTaskService`)

### Error Handling
- Always handle errors explicitly — never ignore with `_`
- Wrap errors with context using `fmt.Errorf("doing something: %w", err)`
- Return appropriate HTTP status codes from handlers based on error type
- Log errors at the handler level, not in service or repository layers

### Testing
- Write table-driven tests following Go conventions
- Test files are in the same package with `_test.go` suffix
- Use interfaces for dependencies to enable mocking
- Aim for coverage on service layer business logic

---

## Environment Variables

```env
# Server
PORT=8080
ENV=development

# PostgreSQL (Supabase-hosted by default)
DATABASE_URL=postgresql://postgres:<password>@db.<project-ref>.supabase.co:5432/postgres

# Supabase Auth
SUPABASE_URL=https://your-project.supabase.co
SUPABASE_SERVICE_ROLE_KEY=your-service-role-key

# Optional
SUPABASE_ANON_KEY=your-anon-key
REDIS_URL=redis://localhost:6379      # Present in config; server uses in-memory JWT cache
ALLOWED_ORIGINS=http://localhost:3000,http://localhost:8080
```

---

## Local Development

### Common path (Supabase-hosted database)

```bash
go mod download
cp .env.example .env    # fill in DATABASE_URL, SUPABASE_URL, SUPABASE_SERVICE_ROLE_KEY
make build
./bin/goalden-api       # auto-migrates on startup, then accepts requests
```

### Fully local Postgres

```bash
docker compose up -d    # starts local Postgres + Redis containers
# Set DATABASE_URL in .env to the local connection string
make build
./bin/goalden-api
```

### Makefile commands

```makefile
make build        # Compile to bin/goalden-api
make dev          # Live reload (requires air)
make test         # Run all tests with race detector
make lint         # Run golangci-lint
make migrate-up   # Apply migrations manually (optional — server auto-migrates)
make sqlc         # Regenerate sqlc query code
make docker-up    # Start local Postgres + Redis
make docker-build # Build production Docker image
```

---

## Security

- All task endpoints behind JWT authentication
- SQL injection prevention via parameterized queries (pgx)
- CORS configured to allow only known origins
- No secrets in code — all via environment variables
- Soft delete for tasks (`deleted_at`) — data is not physically removed
- User data scoping — every query filtered by authenticated user ID
- Token cache evicts expired entries via background goroutine
