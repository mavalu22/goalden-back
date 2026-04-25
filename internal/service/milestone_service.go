package service

import (
	"context"
	"time"

	"github.com/goalden/goalden-api/internal/model"
	"github.com/goalden/goalden-api/internal/repository"
)

// MilestoneSyncRequest carries the client's local milestone changes.
type MilestoneSyncRequest struct {
	Milestones []model.Milestone `json:"milestones"`
	DeletedIDs []string          `json:"deleted_ids"`
	LastSyncAt time.Time         `json:"last_sync_at"`
}

// MilestoneSyncResponse carries server-side milestone changes back to the client.
type MilestoneSyncResponse struct {
	Milestones []model.Milestone `json:"milestones"`
	DeletedIDs []string          `json:"deleted_ids"`
}

// MilestoneService handles milestone business logic and sync.
type MilestoneService struct {
	repo repository.MilestoneRepository
}

// NewMilestoneService creates a new MilestoneService.
func NewMilestoneService(repo repository.MilestoneRepository) *MilestoneService {
	return &MilestoneService{repo: repo}
}

// GetAllMilestones returns all non-deleted milestones for a user.
func (s *MilestoneService) GetAllMilestones(ctx context.Context, userID string) ([]*model.Milestone, error) {
	return s.repo.GetMilestonesForUser(ctx, userID)
}

// DeleteMilestone soft-deletes a milestone.
func (s *MilestoneService) DeleteMilestone(ctx context.Context, userID, milestoneID string) error {
	return s.repo.DeleteMilestone(ctx, milestoneID, userID)
}

// Sync processes the client's changes and returns server-side changes.
func (s *MilestoneService) Sync(ctx context.Context, userID string, req MilestoneSyncRequest) (MilestoneSyncResponse, error) {
	// Push client milestones (last-write-wins via upsert).
	if len(req.Milestones) > 0 {
		ptrs := make([]*model.Milestone, len(req.Milestones))
		for i := range req.Milestones {
			m := req.Milestones[i]
			m.UserID = userID // enforce ownership
			ptrs[i] = &m
		}
		if err := s.repo.BatchUpsertMilestones(ctx, ptrs); err != nil {
			return MilestoneSyncResponse{}, err
		}
	}

	// Apply client deletions.
	if len(req.DeletedIDs) > 0 {
		if err := s.repo.BatchDeleteMilestones(ctx, req.DeletedIDs, userID); err != nil {
			return MilestoneSyncResponse{}, err
		}
	}

	// Pull server changes since last_sync_at.
	serverMilestones, err := s.repo.GetMilestonesUpdatedSince(ctx, userID, req.LastSyncAt)
	if err != nil {
		return MilestoneSyncResponse{}, err
	}
	deletedRefs, err := s.repo.GetDeletedMilestoneIDsSince(ctx, userID, req.LastSyncAt)
	if err != nil {
		return MilestoneSyncResponse{}, err
	}

	out := make([]model.Milestone, 0, len(serverMilestones))
	for _, m := range serverMilestones {
		out = append(out, *m)
	}
	deletedIDs := make([]string, 0, len(deletedRefs))
	for _, ref := range deletedRefs {
		deletedIDs = append(deletedIDs, ref.ID)
	}

	return MilestoneSyncResponse{
		Milestones: out,
		DeletedIDs: deletedIDs,
	}, nil
}
