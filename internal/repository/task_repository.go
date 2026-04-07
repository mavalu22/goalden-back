package repository

import (
	"context"
	"time"

	"github.com/goalden/goalden-api/internal/model"
)

// DeletedTaskRef pairs a task ID with its soft-deletion timestamp.
// Returned by GetDeletedIDsSince so callers can apply last-write-wins when
// a task was locally modified after it was deleted on the server.
type DeletedTaskRef struct {
	ID        string
	DeletedAt time.Time
}

// TaskRepository defines persistence operations for tasks.
type TaskRepository interface {
	GetTasksForUser(ctx context.Context, userID string) ([]*model.Task, error)
	GetTasksForUserAndDate(ctx context.Context, userID string, date time.Time) ([]*model.Task, error)
	GetTasksUpdatedSince(ctx context.Context, userID string, since time.Time) ([]*model.Task, error)
	GetDeletedIDsSince(ctx context.Context, userID string, since time.Time) ([]DeletedTaskRef, error)
	UpsertTask(ctx context.Context, task *model.Task) error
	DeleteTask(ctx context.Context, id, userID string) error
	BatchUpsertTasks(ctx context.Context, tasks []*model.Task) error
	BatchDeleteTasks(ctx context.Context, ids []string, userID string) error
}
