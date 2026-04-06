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

Returns ALL non-deleted tasks for the authenticated user. Use on first launch / new device
to pull the full task list.

	Response:
	{
	  "tasks": [ <Task>, ... ]
	}

### POST /api/v1/tasks/sync

Bidirectional sync endpoint. Send local changes, receive server changes.
Conflict resolution: last-write-wins based on updated_at timestamp.

	Request:
	{
	  "tasks":        [ <Task>, ... ],      // tasks created/modified locally
	  "deleted_ids":  ["uuid1", "uuid2"],   // task IDs deleted locally
	  "last_sync_at": "2024-01-01T00:00:00Z"  // ISO 8601; use zero value for first sync
	}

	Response:
	{
	  "tasks":       [ <Task>, ... ],      // tasks updated on server since last_sync_at
	  "deleted_ids": ["uuid1", "uuid2"]   // task IDs deleted on server since last_sync_at
	}

### DELETE /api/v1/tasks/{id}

Soft-deletes a single task. Only the owning user can delete their tasks.

	Response: 204 No Content

## Task object shape

	{
	  "id":                 "uuid",
	  "user_id":            "uuid",
	  "title":              "string",
	  "date":               "2024-01-15",           // date only, no time component
	  "priority":           "normal" | "high",
	  "note":               "string" | null,
	  "done":               true | false,
	  "recurrence":         "none" | "daily" | "weekly" | "custom_days",
	  "recurrence_days":    "[1,3,5]" | null,       // JSON string; 1=Monday … 7=Sunday
	  "sort_order":         0,
	  "source_task_id":     "uuid" | null,          // non-null for recurring instances
	  "start_time_minutes": 540 | null,             // minutes from midnight (0–1439)
	  "end_time_minutes":   600 | null,
	  "created_at":         "2024-01-15T10:00:00Z",
	  "updated_at":         "2024-01-15T10:00:00Z",
	  "completed_at":       "2024-01-15T10:05:00Z" | null,
	  "deleted_at":         "2024-01-15T10:06:00Z" | null  // non-null = soft-deleted
	}

## Recommended client sync flow

 1. On app launch: call POST /auth/sync-user, then POST /tasks/sync with all local
    pending changes and last_sync_at from local storage.
 2. Merge server response tasks into local DB (last-write-wins on updated_at).
 3. Remove any IDs in response deleted_ids from local DB.
 4. Store current server time as last_sync_at for the next sync.
 5. Periodic background sync: repeat every ~30 s when online.
 6. On reconnect after offline period: trigger a sync immediately.
*/
package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/goalden/goalden-api/internal/middleware"
	"github.com/goalden/goalden-api/internal/model"
	"github.com/goalden/goalden-api/internal/service"
)

// TaskHandler handles task-related HTTP endpoints.
type TaskHandler struct {
	svc *service.TaskService
}

// NewTaskHandler creates a new TaskHandler backed by the given service.
func NewTaskHandler(svc *service.TaskService) *TaskHandler {
	return &TaskHandler{svc: svc}
}

// taskDTO is the JSON shape used for task serialization/deserialization.
type taskDTO struct {
	ID             string     `json:"id"`
	UserID         string     `json:"user_id"`
	Title          string     `json:"title"`
	Date           string     `json:"date"`              // "YYYY-MM-DD"
	Priority       string     `json:"priority"`
	Note           *string    `json:"note"`
	Done           bool       `json:"done"`
	Recurrence     string     `json:"recurrence"`
	RecurrenceDays *string    `json:"recurrence_days"`
	SortOrder      int        `json:"sort_order"`
	SourceTaskID   *string    `json:"source_task_id"`    // non-nil for recurring-task instances
	StartTimeMin   *int       `json:"start_time_minutes"` // optional; minutes from midnight
	EndTimeMin     *int       `json:"end_time_minutes"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	CompletedAt    *time.Time `json:"completed_at"`
	DeletedAt      *time.Time `json:"deleted_at"`        // non-nil = soft-deleted
}

// GetTasks handles GET /api/v1/tasks.
// Returns all non-deleted tasks for the authenticated user. Used for new-device initial pull.
func (h *TaskHandler) GetTasks(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	tasks, err := h.svc.GetAllTasks(r.Context(), userID)
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

	// Optional device ID header — accepted but not yet persisted.
	_ = r.Header.Get("X-Device-ID")

	var body struct {
		Tasks      []taskDTO `json:"tasks"`
		DeletedIDs []string  `json:"deleted_ids"`
		LastSyncAt time.Time `json:"last_sync_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Convert DTOs to domain models.
	models := make([]*model.Task, 0, len(body.Tasks))
	for _, dto := range body.Tasks {
		t, err := dtoToModel(dto)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid task data: "+err.Error())
			return
		}
		models = append(models, t)
	}

	result, err := h.svc.Sync(r.Context(), userID, service.SyncRequest{
		Tasks:      models,
		DeletedIDs: body.DeletedIDs,
		LastSyncAt: body.LastSyncAt,
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "sync failed")
		return
	}

	responseTasks := make([]taskDTO, 0, len(result.Tasks))
	for _, t := range result.Tasks {
		responseTasks = append(responseTasks, modelToDTO(t))
	}
	if result.DeletedIDs == nil {
		result.DeletedIDs = []string{}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"tasks":       responseTasks,
		"deleted_ids": result.DeletedIDs,
	})
}

// DeleteTask handles DELETE /api/v1/tasks/{id}.
// Soft-deletes the task so the deletion can be propagated to other devices.
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

	if err := h.svc.DeleteTask(r.Context(), userID, id); err != nil {
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
		SourceTaskID:   t.SourceTaskID,
		StartTimeMin:   t.StartTimeMin,
		EndTimeMin:     t.EndTimeMin,
		CreatedAt:      t.CreatedAt,
		UpdatedAt:      t.UpdatedAt,
		CompletedAt:    t.CompletedAt,
		DeletedAt:      t.DeletedAt,
	}
}

// dtoToModel converts a taskDTO from JSON to a model.Task.
func dtoToModel(dto taskDTO) (*model.Task, error) {
	date, err := time.Parse("2006-01-02", dto.Date)
	if err != nil {
		return nil, err
	}

	priority := dto.Priority
	if priority == "" {
		priority = "normal"
	}
	recurrence := dto.Recurrence
	if recurrence == "" {
		recurrence = "none"
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
		SourceTaskID:   dto.SourceTaskID,
		StartTimeMin:   dto.StartTimeMin,
		EndTimeMin:     dto.EndTimeMin,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
		CompletedAt:    dto.CompletedAt,
		DeletedAt:      dto.DeletedAt,
	}, nil
}
