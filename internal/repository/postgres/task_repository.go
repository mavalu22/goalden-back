package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/goalden/goalden-api/internal/model"
	"github.com/goalden/goalden-api/internal/repository"
)

// TaskRepo is the Postgres implementation of repository.TaskRepository.
type TaskRepo struct {
	pool *pgxpool.Pool
}

// NewTaskRepo creates a new TaskRepo.
func NewTaskRepo(pool *pgxpool.Pool) *TaskRepo {
	return &TaskRepo{pool: pool}
}

// selectCols is the canonical column list used in every SELECT.
const selectCols = `
	id, user_id, title, date, priority, note, done,
	recurrence, recurrence_days, sort_order,
	source_task_id, start_time_minutes, end_time_minutes,
	goal_id,
	created_at, updated_at, completed_at, deleted_at`

// GetTasksForUser returns all non-deleted tasks belonging to a user.
func (r *TaskRepo) GetTasksForUser(ctx context.Context, userID string) ([]*model.Task, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT`+selectCols+`
		FROM tasks
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY date ASC, sort_order ASC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("get tasks for user: %w", err)
	}
	defer rows.Close()

	return scanTasks(rows)
}

// GetTasksForUserAndDate returns non-deleted tasks for a specific user and date.
func (r *TaskRepo) GetTasksForUserAndDate(ctx context.Context, userID string, date time.Time) ([]*model.Task, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT`+selectCols+`
		FROM tasks
		WHERE user_id = $1 AND date = $2 AND deleted_at IS NULL
		ORDER BY sort_order ASC
	`, userID, date)
	if err != nil {
		return nil, fmt.Errorf("get tasks for user and date: %w", err)
	}
	defer rows.Close()

	return scanTasks(rows)
}

// GetDeletedIDsSince returns (id, deleted_at) pairs for tasks soft-deleted after
// the given time. The deleted_at timestamp is included for LWW conflict resolution.
func (r *TaskRepo) GetDeletedIDsSince(ctx context.Context, userID string, since time.Time) ([]repository.DeletedTaskRef, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, deleted_at FROM tasks
		WHERE user_id = $1 AND deleted_at > $2
		ORDER BY deleted_at ASC
	`, userID, since)
	if err != nil {
		return nil, fmt.Errorf("get deleted ids since: %w", err)
	}
	defer rows.Close()

	var refs []repository.DeletedTaskRef
	for rows.Next() {
		var ref repository.DeletedTaskRef
		if err := rows.Scan(&ref.ID, &ref.DeletedAt); err != nil {
			return nil, fmt.Errorf("scan deleted ref: %w", err)
		}
		refs = append(refs, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return refs, nil
}

// GetTasksUpdatedSince returns all tasks (including soft-deleted) for a user
// that were modified after the given time. Soft-deleted tasks are included so
// the client can propagate deletions.
func (r *TaskRepo) GetTasksUpdatedSince(ctx context.Context, userID string, since time.Time) ([]*model.Task, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT`+selectCols+`
		FROM tasks
		WHERE user_id = $1 AND updated_at > $2
		ORDER BY updated_at ASC
	`, userID, since)
	if err != nil {
		return nil, fmt.Errorf("get tasks updated since: %w", err)
	}
	defer rows.Close()

	return scanTasks(rows)
}

// UpsertTask inserts or updates a single task (last-write-wins on updated_at).
func (r *TaskRepo) UpsertTask(ctx context.Context, task *model.Task) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO tasks (
			id, user_id, title, date, priority, note, done,
			recurrence, recurrence_days, sort_order,
			source_task_id, start_time_minutes, end_time_minutes,
			goal_id,
			created_at, updated_at, completed_at, deleted_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10,
			$11, $12, $13,
			$14,
			$15, $16, $17, $18
		)
		ON CONFLICT (id) DO UPDATE SET
			title              = EXCLUDED.title,
			date               = EXCLUDED.date,
			priority           = EXCLUDED.priority,
			note               = EXCLUDED.note,
			done               = EXCLUDED.done,
			recurrence         = EXCLUDED.recurrence,
			recurrence_days    = EXCLUDED.recurrence_days,
			sort_order         = EXCLUDED.sort_order,
			source_task_id     = EXCLUDED.source_task_id,
			start_time_minutes = EXCLUDED.start_time_minutes,
			end_time_minutes   = EXCLUDED.end_time_minutes,
			goal_id            = EXCLUDED.goal_id,
			updated_at         = EXCLUDED.updated_at,
			completed_at       = EXCLUDED.completed_at,
			deleted_at         = EXCLUDED.deleted_at
		WHERE EXCLUDED.updated_at >= tasks.updated_at
	`,
		task.ID, task.UserID, task.Title, task.Date, task.Priority, task.Note, task.Done,
		task.Recurrence, task.RecurrenceDays, task.SortOrder,
		task.SourceTaskID, task.StartTimeMin, task.EndTimeMin,
		task.GoalID,
		task.CreatedAt, task.UpdatedAt, task.CompletedAt, task.DeletedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert task %s: %w", task.ID, err)
	}
	return nil
}

// DeleteTask soft-deletes a task by setting deleted_at, enforcing user ownership.
// Soft delete is used so that the deletion can be propagated to other devices during sync.
func (r *TaskRepo) DeleteTask(ctx context.Context, id, userID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE tasks
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND user_id = $2
	`, id, userID)
	if err != nil {
		return fmt.Errorf("delete task %s: %w", id, err)
	}
	return nil
}

// BatchDeleteTasks soft-deletes multiple tasks in a single query, enforcing
// user ownership. Tasks not belonging to userID are silently ignored.
func (r *TaskRepo) BatchDeleteTasks(ctx context.Context, ids []string, userID string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE tasks
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = ANY($1) AND user_id = $2
	`, ids, userID)
	if err != nil {
		return fmt.Errorf("batch delete tasks: %w", err)
	}
	return nil
}

// BatchUpsertTasks upserts multiple tasks in a single query using unnest()
// array expansion. This sends all rows to PostgreSQL in one round-trip instead
// of N sequential Exec calls, which is critical for remote databases (Supabase).
func (r *TaskRepo) BatchUpsertTasks(ctx context.Context, tasks []*model.Task) error {
	if len(tasks) == 0 {
		return nil
	}

	// Build parallel slices — one element per task per column.
	ids := make([]string, len(tasks))
	userIDs := make([]string, len(tasks))
	titles := make([]string, len(tasks))
	dates := make([]time.Time, len(tasks))
	priorities := make([]string, len(tasks))
	notes := make([]*string, len(tasks))
	dones := make([]bool, len(tasks))
	recurrences := make([]string, len(tasks))
	recurrenceDays := make([]*string, len(tasks))
	sortOrders := make([]int, len(tasks))
	sourceTaskIDs := make([]*string, len(tasks))
	startTimeMins := make([]*int, len(tasks))
	endTimeMins := make([]*int, len(tasks))
	goalIDs := make([]*string, len(tasks))
	createdAts := make([]time.Time, len(tasks))
	updatedAts := make([]time.Time, len(tasks))
	completedAts := make([]*time.Time, len(tasks))
	deletedAts := make([]*time.Time, len(tasks))

	for i, t := range tasks {
		ids[i] = t.ID
		userIDs[i] = t.UserID
		titles[i] = t.Title
		dates[i] = t.Date
		priorities[i] = t.Priority
		notes[i] = t.Note
		dones[i] = t.Done
		recurrences[i] = t.Recurrence
		recurrenceDays[i] = t.RecurrenceDays
		sortOrders[i] = t.SortOrder
		sourceTaskIDs[i] = t.SourceTaskID
		startTimeMins[i] = t.StartTimeMin
		endTimeMins[i] = t.EndTimeMin
		goalIDs[i] = t.GoalID
		createdAts[i] = t.CreatedAt
		updatedAts[i] = t.UpdatedAt
		completedAts[i] = t.CompletedAt
		deletedAts[i] = t.DeletedAt
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO tasks (
			id, user_id, title, date, priority, note, done,
			recurrence, recurrence_days, sort_order,
			source_task_id, start_time_minutes, end_time_minutes,
			goal_id,
			created_at, updated_at, completed_at, deleted_at
		)
		SELECT * FROM unnest(
			$1::text[], $2::text[], $3::text[], $4::date[], $5::text[], $6::text[], $7::bool[],
			$8::text[], $9::text[], $10::int[],
			$11::text[], $12::int[], $13::int[],
			$14::text[],
			$15::timestamptz[], $16::timestamptz[], $17::timestamptz[], $18::timestamptz[]
		) AS t(
			id, user_id, title, date, priority, note, done,
			recurrence, recurrence_days, sort_order,
			source_task_id, start_time_minutes, end_time_minutes,
			goal_id,
			created_at, updated_at, completed_at, deleted_at
		)
		ON CONFLICT (id) DO UPDATE SET
			title              = EXCLUDED.title,
			date               = EXCLUDED.date,
			priority           = EXCLUDED.priority,
			note               = EXCLUDED.note,
			done               = EXCLUDED.done,
			recurrence         = EXCLUDED.recurrence,
			recurrence_days    = EXCLUDED.recurrence_days,
			sort_order         = EXCLUDED.sort_order,
			source_task_id     = EXCLUDED.source_task_id,
			start_time_minutes = EXCLUDED.start_time_minutes,
			end_time_minutes   = EXCLUDED.end_time_minutes,
			goal_id            = EXCLUDED.goal_id,
			updated_at         = EXCLUDED.updated_at,
			completed_at       = EXCLUDED.completed_at,
			deleted_at         = EXCLUDED.deleted_at
		WHERE EXCLUDED.updated_at >= tasks.updated_at
	`,
		ids, userIDs, titles, dates, priorities, notes, dones,
		recurrences, recurrenceDays, sortOrders,
		sourceTaskIDs, startTimeMins, endTimeMins,
		goalIDs,
		createdAts, updatedAts, completedAts, deletedAts,
	)
	if err != nil {
		return fmt.Errorf("batch upsert tasks: %w", err)
	}
	return nil
}

// scanTasks scans a pgx.Rows result set into a slice of Task pointers.
func scanTasks(rows pgx.Rows) ([]*model.Task, error) {
	var tasks []*model.Task
	for rows.Next() {
		t := &model.Task{}
		if err := rows.Scan(
			&t.ID, &t.UserID, &t.Title, &t.Date, &t.Priority, &t.Note, &t.Done,
			&t.Recurrence, &t.RecurrenceDays, &t.SortOrder,
			&t.SourceTaskID, &t.StartTimeMin, &t.EndTimeMin,
			&t.GoalID,
			&t.CreatedAt, &t.UpdatedAt, &t.CompletedAt, &t.DeletedAt,
		); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return tasks, nil
}
