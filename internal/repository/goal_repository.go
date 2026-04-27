package repository

import (
	"context"
	"time"

	"github.com/goalden/goalden-api/internal/model"
)

// DeletedGoalRef pairs a goal ID with its soft-deletion timestamp.
type DeletedGoalRef struct {
	ID        string
	DeletedAt time.Time
}

// GoalRepository defines persistence operations for goals.
type GoalRepository interface {
	GetGoalsForUser(ctx context.Context, userID string) ([]*model.Goal, error)
	GetGoalsUpdatedSince(ctx context.Context, userID string, since time.Time) ([]*model.Goal, error)
	GetDeletedGoalIDsSince(ctx context.Context, userID string, since time.Time) ([]DeletedGoalRef, error)
	BatchUpsertGoals(ctx context.Context, goals []*model.Goal) error
	BatchDeleteGoals(ctx context.Context, ids []string, userID string) error
	DeleteGoal(ctx context.Context, id, userID string) error
}
