package service

import (
	"context"
	"time"

	"github.com/goalden/goalden-api/internal/model"
	"github.com/goalden/goalden-api/internal/repository"
)

// GoalSyncRequest carries the client's local goal changes to be pushed to the server.
type GoalSyncRequest struct {
	Goals      []*model.Goal
	DeletedIDs []string
	LastSyncAt time.Time
}

// DeletedGoal pairs a goal ID with the timestamp of its soft-deletion.
type DeletedGoal struct {
	ID        string
	DeletedAt time.Time
}

// GoalSyncResponse carries server-side goal changes back to the client.
type GoalSyncResponse struct {
	Goals        []*model.Goal
	DeletedGoals []DeletedGoal
}

// GoalService encapsulates sync business logic on top of the goal repository.
type GoalService struct {
	repo repository.GoalRepository
}

// NewGoalService creates a GoalService backed by the given repository.
func NewGoalService(repo repository.GoalRepository) *GoalService {
	return &GoalService{repo: repo}
}

// DeleteGoal soft-deletes a single goal owned by the user.
func (s *GoalService) DeleteGoal(ctx context.Context, userID, goalID string) error {
	return s.repo.DeleteGoal(ctx, goalID, userID)
}

// GetAllGoals returns every non-deleted goal owned by the user.
func (s *GoalService) GetAllGoals(ctx context.Context, userID string) ([]*model.Goal, error) {
	return s.repo.GetGoalsForUser(ctx, userID)
}

// Sync pushes client goal changes to the cloud and pulls server changes back.
// Conflict resolution is last-write-wins based on updated_at.
func (s *GoalService) Sync(ctx context.Context, userID string, req GoalSyncRequest) (GoalSyncResponse, error) {
	clientIDs := make(map[string]struct{}, len(req.Goals))
	clientDeletedSet := make(map[string]struct{}, len(req.DeletedIDs))

	if len(req.Goals) > 0 {
		safe := make([]*model.Goal, 0, len(req.Goals))
		for _, g := range req.Goals {
			if g.UserID == "" {
				g.UserID = userID
			} else if g.UserID != userID {
				continue
			}
			safe = append(safe, g)
			clientIDs[g.ID] = struct{}{}
		}
		if err := s.repo.BatchUpsertGoals(ctx, safe); err != nil {
			return GoalSyncResponse{}, err
		}
	}

	for _, id := range req.DeletedIDs {
		clientDeletedSet[id] = struct{}{}
	}
	if len(req.DeletedIDs) > 0 {
		if err := s.repo.BatchDeleteGoals(ctx, req.DeletedIDs, userID); err != nil {
			return GoalSyncResponse{}, err
		}
	}

	serverGoals, err := s.repo.GetGoalsUpdatedSince(ctx, userID, req.LastSyncAt)
	if err != nil {
		return GoalSyncResponse{}, err
	}

	responseGoals := make([]*model.Goal, 0, len(serverGoals))
	for _, g := range serverGoals {
		if _, sentByClient := clientIDs[g.ID]; sentByClient {
			continue
		}
		if g.DeletedAt == nil {
			responseGoals = append(responseGoals, g)
		}
	}

	deletedPairs, err := s.repo.GetDeletedGoalIDsSince(ctx, userID, req.LastSyncAt)
	if err != nil {
		return GoalSyncResponse{}, err
	}

	deletedGoals := make([]DeletedGoal, 0, len(deletedPairs))
	for _, pair := range deletedPairs {
		if _, sentByClient := clientDeletedSet[pair.ID]; !sentByClient {
			deletedGoals = append(deletedGoals, DeletedGoal{
				ID:        pair.ID,
				DeletedAt: pair.DeletedAt,
			})
		}
	}

	return GoalSyncResponse{
		Goals:        responseGoals,
		DeletedGoals: deletedGoals,
	}, nil
}
