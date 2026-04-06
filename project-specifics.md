# Goalden Backend — Project Specifics

## Tech Stack

### Core
- **Language:** Go 1.22+ (latest stable)
- **HTTP Framework:** Chi (v5) — stdlib-compatible, minimal overhead, composable middleware
- **Protocol:** REST API (JSON over HTTP)
- **Architecture:** Clean Architecture with layered separation

### Database
- **Primary database:** PostgreSQL 16+
- **Driver:** pgx (v5) — high-performance, pure Go PostgreSQL driver
- **Query builder:** sqlc — generates type-safe Go code from SQL queries. No ORM
- **Migrations:** golang-migrate — versioned SQL migration files

### Cache & Queue
- **Cache:** Redis (via Upstash in production, local Redis in development)
- **Go client:** go-redis (v9)
- **Usage:** Session caching, rate limiting, frequently accessed data, future background job queues

### Authentication
- **Supabase Auth** handles all user authentication (Google, Apple, Email/Password)
- The Go backend does NOT handle login flows directly
- The backend **validates Supabase JWT tokens** on every authenticated request
- JWT validation uses Supabase's public JWKS endpoint
- User ID is extracted from the validated JWT and used to scope all data operations

### Infrastructure
- **Containerization:** Docker + Docker Compose for local development
- **Hosting (MVP):** Railway (free tier, Docker-based deploy)
- **Database hosting (MVP):** Supabase PostgreSQL (free tier — 500MB storage, unlimited API requests, connection pooling via Supavisor). Same Supabase project used for Auth, keeping everything unified
- **Cache hosting (MVP):** Upstash Redis (free tier, serverless)
- **Migration path:** All services are containerized and cloud-agnostic — can migrate to AWS ECS, GCP Cloud Run, or any Docker-compatible platform without code changes. The PostgreSQL database can be migrated to any managed Postgres provider (RDS, Cloud SQL, Neon) via pg_dump/pg_restore when needed

---

## Project Architecture

### Folder Structure

```
goalden-api/
├── cmd/
│   └── server/
│       └── main.go                  # Entry point — starts HTTP server
├── internal/
│   ├── config/
│   │   └── config.go                # Environment config loading
│   ├── server/
│   │   ├── server.go                # HTTP server setup, middleware stack
│   │   └── routes.go                # Route definitions
│   ├── middleware/
│   │   ├── auth.go                  # JWT validation middleware (Supabase)
│   │   ├── cors.go                  # CORS configuration
│   │   ├── ratelimit.go             # Rate limiting (Redis-backed)
│   │   ├── logging.go               # Request/response logging
│   │   └── recovery.go              # Panic recovery
│   ├── handler/                     # HTTP handlers (presentation layer)
│   │   ├── task_handler.go
│   │   ├── goal_handler.go          # Future
│   │   ├── history_handler.go       # Future
│   │   └── health_handler.go
│   ├── service/                     # Business logic (domain layer)
│   │   ├── task_service.go
│   │   ├── goal_service.go          # Future
│   │   └── sync_service.go
│   ├── repository/                  # Data access (data layer)
│   │   ├── task_repository.go
│   │   ├── goal_repository.go       # Future
│   │   └── user_repository.go
│   ├── model/                       # Domain models / entities
│   │   ├── task.go
│   │   ├── goal.go                  # Future
│   │   └── user.go
│   ├── dto/                         # Request/Response DTOs
│   │   ├── task_dto.go
│   │   └── error_dto.go
│   ├── database/
│   │   ├── postgres.go              # PostgreSQL connection pool
│   │   ├── redis.go                 # Redis connection
│   │   └── queries/                 # sqlc generated code
│   │       ├── db.go
│   │       ├── models.go
│   │       └── tasks.sql.go
│   └── pkg/                         # Internal shared utilities
│       ├── validator/               # Input validation helpers
│       ├── response/                # Standardized JSON response helpers
│       └── errs/                    # Custom error types
├── sql/
│   ├── migrations/                  # golang-migrate SQL files
│   │   ├── 000001_create_users.up.sql
│   │   ├── 000001_create_users.down.sql
│   │   ├── 000002_create_tasks.up.sql
│   │   └── 000002_create_tasks.down.sql
│   └── queries/                     # sqlc SQL query files
│       └── tasks.sql
├── sqlc.yaml                        # sqlc configuration
├── Dockerfile
├── docker-compose.yml               # Local dev: Go + PostgreSQL + Redis
├── .env.example
├── Makefile                         # Common commands
├── go.mod
└── go.sum
```

### Architecture Pattern

- **Clean Architecture** with 3 layers: handler → service → repository
- **Handler layer:** Receives HTTP requests, validates input, calls service, returns JSON responses
- **Service layer:** Contains business logic. Knows nothing about HTTP. Receives and returns domain models
- **Repository layer:** Data access only. SQL queries via sqlc. Knows nothing about business rules
- Dependencies flow inward: handler → service → repository → database
- Each layer communicates through interfaces for testability

---

## API Design

### Base URL
```
/api/v1
```

### Authentication
- All endpoints (except health check) require a valid Supabase JWT in the `Authorization: Bearer <token>` header
- The `auth` middleware validates the token and injects the user ID into the request context
- Unauthenticated requests receive `401 Unauthorized`

### Endpoints (MVP)

#### Health
| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check (no auth required) |

#### Tasks
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/tasks` | List tasks (filterable by date range) |
| GET | `/api/v1/tasks/:id` | Get a single task |
| POST | `/api/v1/tasks` | Create a task |
| PUT | `/api/v1/tasks/:id` | Update a task |
| DELETE | `/api/v1/tasks/:id` | Delete a task |
| PATCH | `/api/v1/tasks/:id/complete` | Toggle task completion |
| PATCH | `/api/v1/tasks/:id/reschedule` | Reschedule task to a new date |

#### Sync
| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/sync/push` | Push local changes to server |
| GET | `/api/v1/sync/pull` | Pull changes since last sync timestamp |

### Standard Response Format

**Success:**
```json
{
  "data": { ... },
  "meta": {
    "timestamp": "2026-04-02T12:00:00Z"
  }
}
```

**Error:**
```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Task title is required"
  }
}
```

### Query Parameters for Task Listing
- `date_from` — filter tasks from this date (inclusive, format: YYYY-MM-DD)
- `date_to` — filter tasks up to this date (inclusive, format: YYYY-MM-DD)
- `status` — filter by status: `all`, `pending`, `completed` (default: `all`)
- `include_overdue` — include overdue tasks (default: `false`)

---

## Database Schema (PostgreSQL)

### Users Table
```sql
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    supabase_id TEXT UNIQUE NOT NULL,
    email TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### Tasks Table
```sql
CREATE TABLE tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    note TEXT DEFAULT '',
    date DATE NOT NULL DEFAULT CURRENT_DATE,
    priority TEXT NOT NULL DEFAULT 'normal' CHECK (priority IN ('normal', 'high')),
    done BOOLEAN NOT NULL DEFAULT FALSE,
    recurrence TEXT NOT NULL DEFAULT 'none' CHECK (recurrence IN ('none', 'daily', 'weekly', 'custom_days')),
    recurrence_days INTEGER[] DEFAULT '{}',
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_tasks_user_date ON tasks(user_id, date) WHERE deleted_at IS NULL;
CREATE INDEX idx_tasks_user_overdue ON tasks(user_id, date, done) WHERE deleted_at IS NULL AND done = FALSE;
```

---

## Key Packages

| Package | Purpose |
|---------|---------|
| `github.com/go-chi/chi/v5` | HTTP router and middleware |
| `github.com/jackc/pgx/v5` | PostgreSQL driver |
| `github.com/redis/go-redis/v9` | Redis client |
| `github.com/golang-migrate/migrate/v4` | Database migrations |
| `github.com/sqlc-dev/sqlc` | Type-safe SQL code generation |
| `github.com/golang-jwt/jwt/v5` | JWT parsing and validation |
| `github.com/go-playground/validator/v10` | Input validation |
| `github.com/rs/zerolog` | Structured logging (zero allocation) |
| `github.com/joho/godotenv` | Load .env files |
| `github.com/google/uuid` | UUID generation |

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
- Interfaces: named by what they do, not prefixed with `I` (e.g., `TaskRepository`, not `ITaskRepository`)
- Constructors: `NewXxx` pattern (e.g., `NewTaskService`)

### Error Handling
- Always handle errors explicitly — never ignore with `_`
- Wrap errors with context using `fmt.Errorf("doing something: %w", err)`
- Define custom error types in `internal/pkg/errs/` for domain-specific errors
- Return appropriate HTTP status codes from handlers based on error type
- Log errors at the handler level, not in service or repository layers

### Testing
- Write table-driven tests following Go conventions
- Test files are in the same package with `_test.go` suffix
- Use interfaces for dependencies to enable mocking
- Integration tests use a test database via Docker Compose
- Aim for coverage on service layer business logic — don't over-test handlers or repositories

---

## Environment Variables

```env
# Server
PORT=8080
ENV=development

# PostgreSQL (local dev uses Docker, production uses Supabase)
DATABASE_URL=postgres://goalden:goalden@localhost:5432/goalden?sslmode=disable
# Production DATABASE_URL comes from Supabase project settings > Database > Connection string

# Redis
REDIS_URL=redis://localhost:6379

# Supabase (Auth + Database)
SUPABASE_URL=https://your-project.supabase.co
SUPABASE_JWT_SECRET=your-jwt-secret
SUPABASE_ANON_KEY=your-anon-key
SUPABASE_SERVICE_ROLE_KEY=your-service-role-key

# CORS
ALLOWED_ORIGINS=http://localhost:3000,http://localhost:8080
```

---

## Local Development

### Docker Compose
```yaml
# docker-compose.yml provides:
# - PostgreSQL 16 on port 5432 (local dev mirror of Supabase Postgres)
# - Redis 7 on port 6379
# - Go backend on port 8080 (with hot-reload via air)
# Note: In production, PostgreSQL is hosted by Supabase. Docker Compose
# provides a local instance for development and testing only.
```

### Makefile Commands
```makefile
make dev          # Start docker-compose + run server with hot-reload
make build        # Build the Go binary
make test         # Run all tests
make lint         # Run golangci-lint
make migrate-up   # Run database migrations
make migrate-down # Rollback last migration
make sqlc         # Regenerate sqlc code
make docker-build # Build Docker image
```

---

## Security Checklist

- All endpoints behind JWT authentication (except health check)
- Input validation on every request using validator package
- SQL injection prevention via sqlc (parameterized queries only)
- Rate limiting on all endpoints (Redis-backed)
- CORS configured to allow only known origins
- No secrets in code — all via environment variables
- Soft delete for tasks (deleted_at) — data is never physically removed
- User data scoping — every query filtered by authenticated user_id