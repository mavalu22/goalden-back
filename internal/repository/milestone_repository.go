package repository

import (
	"context"
	"time"

	"github.com/goalden/goalden-api/internal/model"
)

// DeletedMilestoneRef pairs a milestone ID with its soft-deletion timestamp.
type DeletedMilestoneRef struct {
	ID        string
	DeletedAt time.Time
}

// MilestoneRepository defines persistence operations for milestones.
type MilestoneRepository interface {
	GetMilestonesForUser(ctx context.Context, userID string) ([]*model.Milestone, error)
	GetMilestonesUpdatedSince(ctx context.Context, userID string, since time.Time) ([]*model.Milestone, error)
	GetDeletedMilestoneIDsSince(ctx context.Context, userID string, since time.Time) ([]DeletedMilestoneRef, error)
	UpsertMilestone(ctx context.Context, m *model.Milestone) error
	DeleteMilestone(ctx context.Context, id, userID string) error
	BatchUpsertMilestones(ctx context.Context, milestones []*model.Milestone) error
	BatchDeleteMilestones(ctx context.Context, ids []string, userID string) error
}
