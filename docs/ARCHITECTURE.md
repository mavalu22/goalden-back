# Goalden — System Architecture

## Overview

Goalden is a minimal daily and weekly task management app built on an **offline-first** architecture. Tasks are stored locally on the device and synced bidirectionally to a cloud backend when an internet connection is available.

The system has two main repositories:

| Repo | Role |
|------|------|
| `goalden-back` | Go REST API — handles auth, sync, and persistent cloud storage |
| `goalden-front` | Flutter app — handles local task management, UI, and sync coordination |

---

## System Components

```
┌─────────────────────────────────────────────────────────┐
│                    Flutter Client                        │
│                                                         │
│  ┌──────────┐  ┌──────────────┐  ┌──────────────────┐  │
│  │   UI     │  │   Riverpod   │  │   Drift SQLite   │  │
│  │ (GoRouter│  │  Providers   │  │   (local store)  │  │
│  │  + pages)│  │  + Notifiers │  │                  │  │
│  └──────────┘  └──────┬───────┘  └────────┬─────────┘  │
│                       │                   │             │
│                 ┌─────▼─────────────────-─┐             │
│                 │       SyncService        │             │
│                 │ (bidirectional sync logic)│             │
│                 └─────────────┬────────────┘             │
└───────────────────────────────┼─────────────────────────┘
                                │ HTTPS (JWT)
                                ▼
┌─────────────────────────────────────────────────────────┐
│                    Go API (goalden-back)                 │
│                                                         │
│  ┌──────────┐  ┌──────────────┐  ┌──────────────────┐  │
│  │  Chi     │  │   Handlers   │  │  TaskService     │  │
│  │  Router  │──│  (HTTP layer)│──│  (business logic)│  │
│  └──────────┘  └──────────────┘  └────────┬─────────┘  │
│                                           │             │
│  ┌──────────────────┐  ┌─────────────────▼──────────┐  │
│  │  Auth Middleware  │  │  TaskRepository (pgxpool)  │  │
│  │  (Supabase JWT)   │  │  (PostgreSQL via Supabase) │  │
│  └──────────────────┘  └───────────────────────────-┘  │
└─────────────────────────────────────────────────────────┘
                                │
                                ▼
                    ┌───────────────────────┐
                    │   Supabase            │
                    │  ┌─────────────────┐  │
                    │  │  Auth (JWT)     │  │
                    │  ├─────────────────┤  │
                    │  │  PostgreSQL DB  │  │
                    │  └─────────────────┘  │
                    └───────────────────────┘
```

---

## Frontend (goalden-front)

**Stack:** Flutter · Dart · Riverpod · Drift (SQLite) · GoRouter

### Local Storage

All tasks are stored in a per-user **SQLite database** managed by [Drift](https://drift.simonbinder.eu/). The database file is created in the app's private support directory using a stable, file-safe user ID derived from the Supabase session.

Key schema columns for sync:

| Column | Purpose |
|--------|---------|
| `sync_state` | `pending_create`, `pending_update`, `pending_delete`, `synced` |
| `source_task_id` | Non-null on recurring task instances; links to the source task |
| `deleted_at` | Soft-delete timestamp; kept as a tombstone until sync confirms deletion |
| `updated_at` | Used for last-write-wins conflict resolution |

### State Management

Riverpod providers wire UI to the local database via reactive streams (Drift's `watchX` queries). The key providers are:

- `todayTasksProvider` — streams tasks for today; also triggers recurring task generation
- `pendingTasksProvider` — streams unfinished tasks from previous days
- `taskActionsProvider` — handles all task mutations (create, update, delete, reorder)
- `syncActionsProvider` — coordinates background push sync after each mutation

### Sync Coordination

`SyncActionsNotifier.pushSync()` is called after every task mutation. It uses a synchronous boolean guard (`_syncing`) set before the first `await` to prevent concurrent sync calls from overlapping.

### Recurring Tasks

Recurring tasks use a **lazy, per-date generation** model:

- A **source task** has `recurrence = daily | weekly | custom_days` and `source_task_id = null`
- **Instances** are generated on demand when a date is first viewed: `recurrence = none`, `source_task_id = <source.id>`
- `RecurrenceService.generateForDate()` runs on `todayTasksProvider` initialization
- Instances are only created if they don't already exist (idempotent)
- Generation is not retroactive — only the requested date is processed

When a recurring source task is deleted, all future instances are soft-deleted first so the tombstones can be synced to other devices and prevent re-generation there.

---

## Backend (goalden-back)

**Stack:** Go · Chi · pgxpool · PostgreSQL (via Supabase)

### API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/api/v1/auth/sync-user` | Register/update user after login |
| `GET` | `/api/v1/tasks` | Full task pull for new device |
| `POST` | `/api/v1/tasks/sync` | Bidirectional sync |
| `DELETE` | `/api/v1/tasks/{id}` | Soft-delete a single task |

All endpoints require `Authorization: Bearer <supabase_jwt>`.

### Authentication

The auth middleware validates Supabase JWTs on every request. To avoid per-request Supabase API calls (which add ~40–50ms latency), validated tokens are cached in-memory with a **5-minute TTL** and a background eviction goroutine.

### Database

PostgreSQL is accessed via `pgxpool`. Batch operations use PostgreSQL's `unnest()` array expansion to avoid N-round-trip patterns:

- `BatchUpsertTasks` — single `INSERT ... SELECT * FROM unnest(...)` for all incoming sync tasks
- `BatchDeleteTasks` — single `UPDATE WHERE id = ANY($1)` for soft-deleting

Tasks are **soft-deleted** (the `deleted_at` column is set) rather than hard-deleted, so deletions can be propagated to other devices during sync.

---

## Offline-First Behavior

The app works fully without a network connection:

1. All reads and writes go through the local SQLite database
2. Each mutation marks the task with a `sync_state` (`pending_create`, `pending_update`, `pending_delete`)
3. After each mutation, a background sync is triggered — if offline, it silently fails and retries on reconnect
4. The UI reflects local state immediately; sync is invisible to the user in the happy path

This means the app is **always responsive** regardless of connectivity.

---

## Bidirectional Sync Flow

### Client → Server (push)

1. Client reads all tasks with `sync_state != synced` from local SQLite
2. Client sends them in the `tasks` array of `POST /tasks/sync`, along with `deleted_ids` and `last_sync_at`
3. Server upserts received tasks (last-write-wins on `updated_at`)
4. Server soft-deletes the IDs in `deleted_ids`

### Server → Client (pull, same request)

5. Server queries all tasks modified since `last_sync_at` for this user
6. Server returns them in the response `tasks` array, plus `deleted_tasks` (soft-deleted since `last_sync_at`)
7. Client merges server tasks into local SQLite (last-write-wins on `updated_at`)
8. Client removes (or tombstones) any IDs in `deleted_tasks`
9. Client updates `last_sync_at` to the current server time

### Conflict Resolution

**Last-write-wins** based on `updated_at`. Whichever version (local or server) has the more recent `updated_at` is kept. This is intentionally simple — Goalden is a personal, single-user app with no real-time collaboration.

### Soft-Delete Tombstones

Deleted tasks are not removed immediately from the client's local database. The `deleted_at` timestamp is set and `sync_state` is set to `pending_delete`. After a successful sync confirms the server has the deletion, the tombstone is purged from local storage — except for recurring instance tombstones, which are retained to prevent `RecurrenceService` from recreating them.

---

## Key Design Decisions

### Why offline-first?

Tasks are time-critical and must be accessible without a network connection. Local SQLite provides instant reads and writes, and sync is treated as a background concern rather than a blocking operation.

### Why last-write-wins?

Goalden is a single-user personal productivity app. Complex merge strategies (CRDTs, operational transforms) add implementation complexity that isn't justified here. LWW on `updated_at` is simple, predictable, and sufficient.

### Why soft-delete?

Hard-deleting a task on one device means the server has no record of the deletion. When another device syncs, it would not know the task was deleted and would keep it. Soft-deletes leave a tombstone that propagates the deletion to all devices during sync.

### Why lazy recurring instance generation?

Pre-generating all future instances (weeks or months ahead) would bloat the database and create unnecessary sync traffic. Instead, instances are generated on the client only when a date is first viewed. This keeps the data set small and sync fast.

### Why a JWT token cache?

Supabase JWT validation via its REST API adds ~40–50ms per request. A 5-minute in-memory cache with per-token TTL eliminates this overhead for the common case (frequent API calls within a short window) while maintaining correctness (tokens are validated on first use and re-validated after TTL expiry).

---

## Repository Structure

### Backend (`goalden-back`)

```
cmd/server/        Entry point
internal/
  config/          Environment config loading
  database/        DB connection pool and migrations
  handler/         HTTP handlers (request/response, DTO conversion)
  middleware/       Auth (JWT validation + token cache)
  model/           Domain models
  repository/      Data access layer (PostgreSQL queries)
  service/         Business logic (sync, task operations)
sql/migrations/    SQL migration files
Makefile           Build, test, lint, migrate targets
Dockerfile         Production container build
```

### Frontend (`goalden-front`)

```
lib/
  data/
    local/         Drift database, DAOs, migrations
    services/      SyncService (HTTP + local DB coordination)
  domain/
    models/        Task model (Freezed)
    repositories/  Repository interfaces
    services/      RecurrenceService
  presentation/
    today/         Today screen, providers, widgets
    week/          Week screen
    history/       History screen
    goals/         Goals screen (placeholder)
  providers/       Global providers (auth, database, sync)
test/              Unit and integration tests
```

---

## Further Reading

- `goalden-back/internal/handler/task_handler.go` — full sync protocol contract and endpoint documentation in package-level comment
- `goalden-back/docs/RELEASE.md` — build and deployment instructions
- `goalden-front/docs/PLATFORM_STATUS.md` — platform support status
- `goalden-front/sync_test_checklist.md` — manual sync test checklist
