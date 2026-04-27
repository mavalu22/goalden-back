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

// GoalHandler handles goal-related HTTP endpoints.
type GoalHandler struct {
	svc *service.GoalService
}

// NewGoalHandler creates a new GoalHandler backed by the given service.
func NewGoalHandler(svc *service.GoalService) *GoalHandler {
	return &GoalHandler{svc: svc}
}

// goalDTO is the JSON shape for goal serialization/deserialization.
type goalDTO struct {
	ID          string     `json:"id"`
	UserID      string     `json:"user_id"`
	Title       string     `json:"title"`
	Description *string    `json:"description"`
	Color       string     `json:"color"`
	Status      string     `json:"status"`
	Deadline    *string    `json:"deadline"`   // "YYYY-MM-DD" or null
	Starred     bool       `json:"starred"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ArchivedAt  *time.Time `json:"archived_at"`
	DeletedAt   *time.Time `json:"deleted_at"`
}

// GetGoals handles GET /api/v1/goals.
func (h *GoalHandler) GetGoals(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	goals, err := h.svc.GetAllGoals(r.Context(), userID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve goals")
		return
	}

	dtos := make([]goalDTO, 0, len(goals))
	for _, g := range goals {
		dtos = append(dtos, goalModelToDTO(g))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{"goals": dtos})
}

// SyncGoals handles POST /api/v1/goals/sync.
func (h *GoalHandler) SyncGoals(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	var body struct {
		Goals      []goalDTO `json:"goals"`
		DeletedIDs []string  `json:"deleted_ids"`
		LastSyncAt time.Time `json:"last_sync_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	models := make([]*model.Goal, 0, len(body.Goals))
	for _, dto := range body.Goals {
		g, err := goalDTOToModel(dto)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid goal data: "+err.Error())
			return
		}
		models = append(models, g)
	}

	result, err := h.svc.Sync(r.Context(), userID, service.GoalSyncRequest{
		Goals:      models,
		DeletedIDs: body.DeletedIDs,
		LastSyncAt: body.LastSyncAt,
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "sync failed")
		return
	}

	responseGoals := make([]goalDTO, 0, len(result.Goals))
	for _, g := range result.Goals {
		responseGoals = append(responseGoals, goalModelToDTO(g))
	}

	deletedGoalsOut := make([]map[string]any, 0, len(result.DeletedGoals))
	for _, d := range result.DeletedGoals {
		deletedGoalsOut = append(deletedGoalsOut, map[string]any{
			"id":         d.ID,
			"deleted_at": d.DeletedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"goals":         responseGoals,
		"deleted_goals": deletedGoalsOut,
	})
}

// DeleteGoal handles DELETE /api/v1/goals/{id}.
func (h *GoalHandler) DeleteGoal(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "missing goal id")
		return
	}

	if err := h.svc.DeleteGoal(r.Context(), userID, id); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to delete goal")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func goalModelToDTO(g *model.Goal) goalDTO {
	var deadline *string
	if g.Deadline != nil {
		s := g.Deadline.Format("2006-01-02")
		deadline = &s
	}
	return goalDTO{
		ID:          g.ID,
		UserID:      g.UserID,
		Title:       g.Title,
		Description: g.Description,
		Color:       g.Color,
		Status:      g.Status,
		Deadline:    deadline,
		Starred:     g.Starred,
		CreatedAt:   g.CreatedAt,
		UpdatedAt:   g.UpdatedAt,
		ArchivedAt:  g.ArchivedAt,
		DeletedAt:   g.DeletedAt,
	}
}

func goalDTOToModel(dto goalDTO) (*model.Goal, error) {
	var deadline *time.Time
	if dto.Deadline != nil && *dto.Deadline != "" {
		t, err := time.Parse("2006-01-02", *dto.Deadline)
		if err != nil {
			return nil, err
		}
		deadline = &t
	}

	status := dto.Status
	if status == "" {
		status = "active"
	}

	updatedAt := dto.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	createdAt := dto.CreatedAt
	if createdAt.IsZero() {
		createdAt = updatedAt
	}

	return &model.Goal{
		ID:          dto.ID,
		UserID:      dto.UserID,
		Title:       dto.Title,
		Description: dto.Description,
		Color:       dto.Color,
		Status:      status,
		Deadline:    deadline,
		Starred:     dto.Starred,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
		ArchivedAt:  dto.ArchivedAt,
		DeletedAt:   dto.DeletedAt,
	}, nil
}
