-- Migration 005: Add goals table for multi-device goal sync.

CREATE TABLE IF NOT EXISTS goals (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title       TEXT NOT NULL,
    description TEXT,
    color       TEXT NOT NULL DEFAULT '',         -- hex color id from the palette
    status      TEXT NOT NULL DEFAULT 'active',   -- 'active' | 'archived'
    deadline    DATE,
    starred     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    archived_at TIMESTAMPTZ,
    deleted_at  TIMESTAMPTZ                       -- soft-delete for sync propagation
);

CREATE INDEX IF NOT EXISTS idx_goals_user_updated ON goals(user_id, updated_at);
CREATE INDEX IF NOT EXISTS idx_goals_user_deleted ON goals(user_id, deleted_at) WHERE deleted_at IS NOT NULL;
