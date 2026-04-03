/*
Package handler implements the Goalden task API endpoints.

# Sync Protocol — Flutter Client Contract

## Authentication
All task endpoints require:
  - Header: Authorization: Bearer <supabase_access_token>
  - Optional header: X-Device-ID: <device_uuid>  (for future per-device tracking)

## Endpoints

### POST /api/v1/auth/sync-user
Call this immediately after a successful Supabase login to register/update the user record.

  Request:  { "email": "user@example.com" }
  Response: { "status": "ok" }

### GET /api/v1/tasks
Returns ALL tasks for the authenticated user. Use on first launch / new device to pull the
full task list.

  Response:
  {
    "tasks": [ <Task>, ... ]
  }

### POST /api/v1/tasks/sync
Bidirectional sync endpoint. Send local changes, receive server changes.
Conflict resolution: last-write-wins based on updated_at timestamp.

  Request:
  {
    "tasks":        [ <Task>, ... ],     // tasks created/modified locally
    "deleted_ids":  ["uuid1", "uuid2"], // task IDs deleted locally
    "last_sync_at": "2024-01-01T00:00:00Z"  // ISO 8601; use "0001-01-01T00:00:00Z" for first sync
  }

  Response:
  {
    "tasks":       [ <Task>, ... ],     // tasks updated on server since last_sync_at (excludes those sent by client)
    "deleted_ids": ["uuid1", "uuid2"]  // IDs deleted on server since last_sync_at (future: soft-delete log)
  }

### DELETE /api/v1/tasks/{id}
Deletes a single task. Only the owning user can delete their tasks.

  Response: 204 No Content

## Task object shape
{
  "id":              "uuid",
  "user_id":         "uuid",
  "title":           "string",
  "date":            "2024-01-15",         // date only, no time component
  "priority":        "normal" | "high",
  "note":            "string" | null,
  "done":            true | false,
  "recurrence":      "none" | "daily" | "weekly" | "custom_days",
  "recurrence_days": "[1,3,5]" | null,    // JSON string; 1=Monday … 7=Sunday
  "sort_order":      0,
  "created_at":      "2024-01-15T10:00:00Z",
  "updated_at":      "2024-01-15T10:00:00Z",
  "completed_at":    "2024-01-15T10:05:00Z" | null
}

## Sync flow (recommended client implementation)
1. On app launch: call POST /auth/sync-user, then POST /tasks/sync with all local
   pending changes and last_sync_at from local storage.
2. Merge server response tasks into local DB (last-write-wins on updated_at).
3. Delete any IDs in response deleted_ids from local DB.
4. Store current server time as last_sync_at for the next sync.
5. Periodic background sync: repeat step 1 every ~30s when online.
6. On reconnect after offline period: immediately trigger a sync.
*/
package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/goalden/goalden-api/internal/middleware"
	"github.com/goalden/goalden-api/internal/model"
	"github.com/goalden/goalden-api/internal/repository"
)

// TaskHandler handles task-related HTTP endpoints.
type TaskHandler struct {
	tasks repository.TaskRepository
}

// NewTaskHandler creates a new TaskHandler.
func NewTaskHandler(tasks repository.TaskRepository) *TaskHandler {
	return &TaskHandler{tasks: tasks}
}

// taskDTO is the JSON shape used for task serialization/deserialization.
type taskDTO struct {
	ID             string     `json:"id"`
	UserID         string     `json:"user_id"`
	Title          string     `json:"title"`
	Date           string     `json:"date"`           // "YYYY-MM-DD"
	Priority       string     `json:"priority"`
	Note           *string    `json:"note"`
	Done           bool       `json:"done"`
	Recurrence     string     `json:"recurrence"`
	RecurrenceDays *string    `json:"recurrence_days"`
	SortOrder      int        `json:"sort_order"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	CompletedAt    *time.Time `json:"completed_at"`
}

// GetTasks handles GET /api/v1/tasks.
// Returns all tasks for the authenticated user. Used for new-device initial pull.
func (h *TaskHandler) GetTasks(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	tasks, err := h.tasks.GetTasksForUser(r.Context(), userID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve tasks")
		return
	}

	dtos := make([]taskDTO, 0, len(tasks))
	for _, t := range tasks {
		dtos = append(dtos, modelToDTO(t))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{"tasks": dtos})
}

// SyncTasks handles POST /api/v1/tasks/sync.
// Accepts local changes (upserts + deletes) and returns server-side changes since last_sync_at.
func (h *TaskHandler) SyncTasks(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	// X-Device-ID is accepted for future per-device tracking and analytics.
	// The server echoes it back so the client can confirm it was received.
	// Session validation is stateless (Supabase token per request), so each device
	// maintains an independent session — logging out on one device does not revoke
	// tokens on other devices until they expire or are explicitly revoked in Supabase.
	deviceID := r.Header.Get("X-Device-ID")
	if deviceID != "" {
		w.Header().Set("X-Device-ID", deviceID)
	}

	var body struct {
		Tasks      []taskDTO `json:"tasks"`
		DeletedIDs []string  `json:"deleted_ids"`
		LastSyncAt time.Time `json:"last_sync_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Track IDs sent by client so we exclude them from the server response
	clientIDs := make(map[string]struct{}, len(body.Tasks))

	// Upsert tasks sent by client (last-write-wins enforced in SQL)
	if len(body.Tasks) > 0 {
		models := make([]*model.Task, 0, len(body.Tasks))
		for _, dto := range body.Tasks {
			// Enforce user ownership — ignore tasks that don't belong to this user
			if dto.UserID != userID {
				continue
			}
			t, err := dtoToModel(dto)
			if err != nil {
				writeJSONError(w, http.StatusBadRequest, "invalid task data: "+err.Error())
				return
			}
			models = append(models, t)
			clientIDs[dto.ID] = struct{}{}
		}
		if err := h.tasks.BatchUpsertTasks(r.Context(), models); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to upsert tasks")
			return
		}
	}

	// Delete tasks requested by client (ownership enforced in DELETE query)
	for _, id := range body.DeletedIDs {
		if err := h.tasks.DeleteTask(r.Context(), id, userID); err != nil {
			// Log but don't fail — task may already be deleted
			_ = err
		}
	}

	// Fetch tasks updated on the server since last_sync_at that were NOT sent by this client
	serverUpdated, err := h.tasks.GetTasksUpdatedSince(r.Context(), userID, body.LastSyncAt)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve server updates")
		return
	}

	responseTasks := make([]taskDTO, 0)
	for _, t := range serverUpdated {
		if _, sentByClient := clientIDs[t.ID]; !sentByClient {
			responseTasks = append(responseTasks, modelToDTO(t))
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"tasks":       responseTasks,
		"deleted_ids": []string{}, // reserved for future soft-delete log
	})
}

// DeleteTask handles DELETE /api/v1/tasks/{id}.
func (h *TaskHandler) DeleteTask(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "missing task id")
		return
	}

	if err := h.tasks.DeleteTask(r.Context(), id, userID); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to delete task")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// modelToDTO converts a model.Task to a taskDTO for JSON serialization.
func modelToDTO(t *model.Task) taskDTO {
	return taskDTO{
		ID:             t.ID,
		UserID:         t.UserID,
		Title:          t.Title,
		Date:           t.Date.Format("2006-01-02"),
		Priority:       t.Priority,
		Note:           t.Note,
		Done:           t.Done,
		Recurrence:     t.Recurrence,
		RecurrenceDays: t.RecurrenceDays,
		SortOrder:      t.SortOrder,
		CreatedAt:      t.CreatedAt,
		UpdatedAt:      t.UpdatedAt,
		CompletedAt:    t.CompletedAt,
	}
}

// validPriorities and validRecurrences are the allowed enum values.
var (
	validPriorities  = map[string]bool{"normal": true, "high": true}
	validRecurrences = map[string]bool{"none": true, "daily": true, "weekly": true, "custom_days": true}
)

// dtoToModel converts a taskDTO from JSON to a model.Task with input validation.
func dtoToModel(dto taskDTO) (*model.Task, error) {
	if dto.ID == "" {
		return nil, fmt.Errorf("task id is required")
	}
	if dto.Title == "" {
		return nil, fmt.Errorf("task title is required")
	}

	date, err := time.Parse("2006-01-02", dto.Date)
	if err != nil {
		return nil, fmt.Errorf("invalid date format (expected YYYY-MM-DD): %w", err)
	}

	priority := dto.Priority
	if priority == "" {
		priority = "normal"
	}
	if !validPriorities[priority] {
		return nil, fmt.Errorf("invalid priority %q: must be 'normal' or 'high'", priority)
	}

	recurrence := dto.Recurrence
	if recurrence == "" {
		recurrence = "none"
	}
	if !validRecurrences[recurrence] {
		return nil, fmt.Errorf("invalid recurrence %q: must be 'none', 'daily', 'weekly', or 'custom_days'", recurrence)
	}

	updatedAt := dto.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	createdAt := dto.CreatedAt
	if createdAt.IsZero() {
		createdAt = updatedAt
	}

	return &model.Task{
		ID:             dto.ID,
		UserID:         dto.UserID,
		Title:          dto.Title,
		Date:           date,
		Priority:       priority,
		Note:           dto.Note,
		Done:           dto.Done,
		Recurrence:     recurrence,
		RecurrenceDays: dto.RecurrenceDays,
		SortOrder:      dto.SortOrder,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
		CompletedAt:    dto.CompletedAt,
	}, nil
}
