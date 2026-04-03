CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,          -- Supabase auth user ID (UUID as text)
    email TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS tasks (
    id TEXT PRIMARY KEY,          -- UUID generated client-side
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    date DATE NOT NULL,
    priority TEXT NOT NULL DEFAULT 'normal',   -- 'normal' | 'high'
    note TEXT,
    done BOOLEAN NOT NULL DEFAULT FALSE,
    recurrence TEXT NOT NULL DEFAULT 'none',   -- 'none' | 'daily' | 'weekly' | 'custom_days'
    recurrence_days TEXT,          -- JSON array of ints, e.g. [1,3,5]
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_tasks_user_date ON tasks(user_id, date);
CREATE INDEX IF NOT EXISTS idx_tasks_user_updated ON tasks(user_id, updated_at);
