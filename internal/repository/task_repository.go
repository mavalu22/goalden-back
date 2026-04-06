package repository

import (
	"context"
	"time"

	"github.com/goalden/goalden-api/internal/model"
)

// TaskRepository defines persistence operations for tasks.
type TaskRepository interface {
	GetTasksForUser(ctx context.Context, userID string) ([]*model.Task, error)
	GetTasksForUserAndDate(ctx context.Context, userID string, date time.Time) ([]*model.Task, error)
	GetTasksUpdatedSince(ctx context.Context, userID string, since time.Time) ([]*model.Task, error)
	GetDeletedIDsSince(ctx context.Context, userID string, since time.Time) ([]string, error)
	UpsertTask(ctx context.Context, task *model.Task) error
	DeleteTask(ctx context.Context, id, userID string) error
	BatchUpsertTasks(ctx context.Context, tasks []*model.Task) error
}
