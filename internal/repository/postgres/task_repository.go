package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/goalden/goalden-api/internal/model"
)

// TaskRepo is the Postgres implementation of repository.TaskRepository.
type TaskRepo struct {
	pool *pgxpool.Pool
}

// NewTaskRepo creates a new TaskRepo.
func NewTaskRepo(pool *pgxpool.Pool) *TaskRepo {
	return &TaskRepo{pool: pool}
}

// GetTasksForUser returns all tasks belonging to a user.
func (r *TaskRepo) GetTasksForUser(ctx context.Context, userID string) ([]*model.Task, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, title, date, priority, note, done,
		       recurrence, recurrence_days, sort_order,
		       created_at, updated_at, completed_at
		FROM tasks
		WHERE user_id = $1
		ORDER BY date ASC, sort_order ASC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("get tasks for user: %w", err)
	}
	defer rows.Close()

	return scanTasks(rows)
}

// GetTasksForUserAndDate returns tasks for a specific user and date.
func (r *TaskRepo) GetTasksForUserAndDate(ctx context.Context, userID string, date time.Time) ([]*model.Task, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, title, date, priority, note, done,
		       recurrence, recurrence_days, sort_order,
		       created_at, updated_at, completed_at
		FROM tasks
		WHERE user_id = $1 AND date = $2
		ORDER BY sort_order ASC
	`, userID, date)
	if err != nil {
		return nil, fmt.Errorf("get tasks for user and date: %w", err)
	}
	defer rows.Close()

	return scanTasks(rows)
}

// GetTasksUpdatedSince returns all tasks for a user updated after the given time.
func (r *TaskRepo) GetTasksUpdatedSince(ctx context.Context, userID string, since time.Time) ([]*model.Task, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, title, date, priority, note, done,
		       recurrence, recurrence_days, sort_order,
		       created_at, updated_at, completed_at
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
			created_at, updated_at, completed_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10,
			$11, $12, $13
		)
		ON CONFLICT (id) DO UPDATE SET
			title           = EXCLUDED.title,
			date            = EXCLUDED.date,
			priority        = EXCLUDED.priority,
			note            = EXCLUDED.note,
			done            = EXCLUDED.done,
			recurrence      = EXCLUDED.recurrence,
			recurrence_days = EXCLUDED.recurrence_days,
			sort_order      = EXCLUDED.sort_order,
			updated_at      = EXCLUDED.updated_at,
			completed_at    = EXCLUDED.completed_at
		WHERE EXCLUDED.updated_at >= tasks.updated_at
	`,
		task.ID, task.UserID, task.Title, task.Date, task.Priority, task.Note, task.Done,
		task.Recurrence, task.RecurrenceDays, task.SortOrder,
		task.CreatedAt, task.UpdatedAt, task.CompletedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert task %s: %w", task.ID, err)
	}
	return nil
}

// DeleteTask removes a task by ID, enforcing user ownership.
func (r *TaskRepo) DeleteTask(ctx context.Context, id, userID string) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM tasks WHERE id = $1 AND user_id = $2
	`, id, userID)
	if err != nil {
		return fmt.Errorf("delete task %s: %w", id, err)
	}
	return nil
}

// BatchUpsertTasks upserts multiple tasks efficiently using a single transaction.
func (r *TaskRepo) BatchUpsertTasks(ctx context.Context, tasks []*model.Task) error {
	if len(tasks) == 0 {
		return nil
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin batch upsert tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	for _, task := range tasks {
		_, err := tx.Exec(ctx, `
			INSERT INTO tasks (
				id, user_id, title, date, priority, note, done,
				recurrence, recurrence_days, sort_order,
				created_at, updated_at, completed_at
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7,
				$8, $9, $10,
				$11, $12, $13
			)
			ON CONFLICT (id) DO UPDATE SET
				title           = EXCLUDED.title,
				date            = EXCLUDED.date,
				priority        = EXCLUDED.priority,
				note            = EXCLUDED.note,
				done            = EXCLUDED.done,
				recurrence      = EXCLUDED.recurrence,
				recurrence_days = EXCLUDED.recurrence_days,
				sort_order      = EXCLUDED.sort_order,
				updated_at      = EXCLUDED.updated_at,
				completed_at    = EXCLUDED.completed_at
			WHERE EXCLUDED.updated_at >= tasks.updated_at
		`,
			task.ID, task.UserID, task.Title, task.Date, task.Priority, task.Note, task.Done,
			task.Recurrence, task.RecurrenceDays, task.SortOrder,
			task.CreatedAt, task.UpdatedAt, task.CompletedAt,
		)
		if err != nil {
			return fmt.Errorf("batch upsert task %s: %w", task.ID, err)
		}
	}

	return tx.Commit(ctx)
}

// scanTasks scans a pgx.Rows result set into a slice of Task pointers.
func scanTasks(rows pgx.Rows) ([]*model.Task, error) {
	var tasks []*model.Task
	for rows.Next() {
		t := &model.Task{}
		if err := rows.Scan(
			&t.ID, &t.UserID, &t.Title, &t.Date, &t.Priority, &t.Note, &t.Done,
			&t.Recurrence, &t.RecurrenceDays, &t.SortOrder,
			&t.CreatedAt, &t.UpdatedAt, &t.CompletedAt,
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
