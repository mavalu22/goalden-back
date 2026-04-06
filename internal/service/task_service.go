// Package service contains business logic for Goalden task synchronization.
//
// # Sync contract (client-facing)
//
// The sync operation is a single round-trip:
//
//  1. Client sends all locally-changed tasks (creates + edits) and the IDs of any
//     locally-deleted tasks, along with the timestamp of the last successful sync.
//
//  2. Server upserts the client's tasks (last-write-wins on updated_at), soft-deletes
//     any tasks in deleted_ids, then returns every task and deletion that occurred on
//     the server since last_sync_at (excluding what the client just sent).
//
//  3. Client merges the server response into its local store:
//     — upsert returned tasks (last-write-wins)
//     — remove any IDs in returned deleted_ids
//     — persist the current server time as the new last_sync_at
//
// # Ownership
//
// All operations are scoped to the authenticated user. Tasks that do not belong to
// the requesting user are silently ignored on write and never returned on read.
package service

import (
	"context"
	"time"

	"github.com/goalden/goalden-api/internal/model"
	"github.com/goalden/goalden-api/internal/repository"
)

// SyncRequest carries the client's local changes to be pushed to the server.
type SyncRequest struct {
	// Tasks contains tasks created or modified locally since the last sync.
	Tasks []*model.Task
	// DeletedIDs contains IDs of tasks deleted locally since the last sync.
	DeletedIDs []string
	// LastSyncAt is the timestamp of the client's last successful sync.
	// Pass the zero value for a first-time sync (pulls everything).
	LastSyncAt time.Time
}

// SyncResponse carries server-side changes back to the client.
type SyncResponse struct {
	// Tasks contains tasks modified on the server since LastSyncAt that were
	// not included in the client's push (avoids echoing the client's own changes).
	Tasks []*model.Task
	// DeletedIDs contains IDs of tasks soft-deleted on the server since LastSyncAt.
	DeletedIDs []string
}

// TaskService encapsulates sync business logic on top of the task repository.
type TaskService struct {
	repo repository.TaskRepository
}

// NewTaskService creates a TaskService backed by the given repository.
func NewTaskService(repo repository.TaskRepository) *TaskService {
	return &TaskService{repo: repo}
}

// DeleteTask soft-deletes a single task owned by the user.
func (s *TaskService) DeleteTask(ctx context.Context, userID, taskID string) error {
	return s.repo.DeleteTask(ctx, taskID, userID)
}

// GetAllTasks returns every non-deleted task owned by the user.
// Intended for new-device initial pull.
func (s *TaskService) GetAllTasks(ctx context.Context, userID string) ([]*model.Task, error) {
	return s.repo.GetTasksForUser(ctx, userID)
}

// Sync pushes client changes to the cloud and pulls server changes back.
// Conflict resolution is last-write-wins based on updated_at.
func (s *TaskService) Sync(ctx context.Context, userID string, req SyncRequest) (SyncResponse, error) {
	// Track IDs sent by the client so we can exclude them from the pull response.
	clientIDs := make(map[string]struct{}, len(req.Tasks))

	// Push: upsert tasks sent by the client.
	if len(req.Tasks) > 0 {
		safe := make([]*model.Task, 0, len(req.Tasks))
		for _, t := range req.Tasks {
			if t.UserID != userID {
				continue // reject tasks belonging to another user
			}
			safe = append(safe, t)
			clientIDs[t.ID] = struct{}{}
		}
		if err := s.repo.BatchUpsertTasks(ctx, safe); err != nil {
			return SyncResponse{}, err
		}
	}

	// Push: soft-delete tasks requested by the client.
	for _, id := range req.DeletedIDs {
		if err := s.repo.DeleteTask(ctx, id, userID); err != nil {
			// Non-fatal: task may already be deleted or never existed.
			continue
		}
	}

	// Pull: tasks modified on the server since last sync that the client didn't just send.
	serverTasks, err := s.repo.GetTasksUpdatedSince(ctx, userID, req.LastSyncAt)
	if err != nil {
		return SyncResponse{}, err
	}

	responseTasks := make([]*model.Task, 0, len(serverTasks))
	for _, t := range serverTasks {
		if _, sentByClient := clientIDs[t.ID]; !sentByClient {
			// Include non-deleted tasks and soft-deleted ones so the client can
			// reconcile deletions via the DeletedAt field.
			if t.DeletedAt == nil {
				responseTasks = append(responseTasks, t)
			}
		}
	}

	// Pull: IDs deleted on the server since last sync.
	deletedIDs, err := s.repo.GetDeletedIDsSince(ctx, userID, req.LastSyncAt)
	if err != nil {
		return SyncResponse{}, err
	}

	// De-duplicate: exclude IDs the client sent as deletions (they already know).
	clientDeletedSet := make(map[string]struct{}, len(req.DeletedIDs))
	for _, id := range req.DeletedIDs {
		clientDeletedSet[id] = struct{}{}
	}
	filteredDeleted := make([]string, 0, len(deletedIDs))
	for _, id := range deletedIDs {
		if _, sentByClient := clientDeletedSet[id]; !sentByClient {
			filteredDeleted = append(filteredDeleted, id)
		}
	}

	return SyncResponse{
		Tasks:      responseTasks,
		DeletedIDs: filteredDeleted,
	}, nil
}
