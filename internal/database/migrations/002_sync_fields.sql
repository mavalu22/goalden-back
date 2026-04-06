-- Migration 002: Add sync metadata and missing task fields
-- All ALTER TABLE statements use IF NOT EXISTS / DO NOTHING patterns to remain idempotent.

-- source_task_id: links a recurring-task instance back to its originating task.
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS source_task_id TEXT REFERENCES tasks(id) ON DELETE SET NULL;

-- start_time_minutes / end_time_minutes: optional time range (0–1439 = minutes from midnight).
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS start_time_minutes INTEGER;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS end_time_minutes   INTEGER;

-- deleted_at: soft-delete marker for sync propagation.
-- A non-null value means the task has been deleted on this device and the deletion
-- should be propagated to other devices during the next sync round.
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

-- Index to efficiently query soft-deleted tasks during sync pull.
CREATE INDEX IF NOT EXISTS idx_tasks_user_deleted ON tasks(user_id, deleted_at) WHERE deleted_at IS NOT NULL;
