-- Migration 003: Add goal_id to tasks for task ↔ goal linking.
-- Idempotent: uses IF NOT EXISTS so re-running is safe.

ALTER TABLE tasks ADD COLUMN IF NOT EXISTS goal_id TEXT;

-- Index for efficiently querying all tasks belonging to a specific goal.
CREATE INDEX IF NOT EXISTS idx_tasks_goal_id ON tasks(goal_id) WHERE goal_id IS NOT NULL;
