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

// MilestoneHandler handles milestone-related HTTP endpoints.
type MilestoneHandler struct {
	svc *service.MilestoneService
}

// NewMilestoneHandler creates a new MilestoneHandler.
func NewMilestoneHandler(svc *service.MilestoneService) *MilestoneHandler {
	return &MilestoneHandler{svc: svc}
}

// milestoneDTO is the JSON shape used for milestone serialization/deserialization.
type milestoneDTO struct {
	ID          string     `json:"id"`
	GoalID      string     `json:"goal_id"`
	UserID      string     `json:"user_id"`
	Title       string     `json:"title"`
	Date        string     `json:"date"` // "YYYY-MM-DD"
	Done        bool       `json:"done"`
	CompletedAt *time.Time `json:"completed_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DeletedAt   *time.Time `json:"deleted_at"`
}

func milestoneModelToDTO(m *model.Milestone) milestoneDTO {
	return milestoneDTO{
		ID:          m.ID,
		GoalID:      m.GoalID,
		UserID:      m.UserID,
		Title:       m.Title,
		Date:        m.Date.Format("2006-01-02"),
		Done:        m.Done,
		CompletedAt: m.CompletedAt,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
		DeletedAt:   m.DeletedAt,
	}
}

func milestoneDTOToModel(dto milestoneDTO) (*model.Milestone, error) {
	date, err := time.Parse("2006-01-02", dto.Date)
	if err != nil {
		return nil, err
	}
	return &model.Milestone{
		ID:          dto.ID,
		GoalID:      dto.GoalID,
		UserID:      dto.UserID,
		Title:       dto.Title,
		Date:        date,
		Done:        dto.Done,
		CompletedAt: dto.CompletedAt,
		CreatedAt:   dto.CreatedAt,
		UpdatedAt:   dto.UpdatedAt,
		DeletedAt:   dto.DeletedAt,
	}, nil
}

// GetMilestones handles GET /api/v1/milestones.
func (h *MilestoneHandler) GetMilestones(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	milestones, err := h.svc.GetAllMilestones(r.Context(), userID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve milestones")
		return
	}

	dtos := make([]milestoneDTO, 0, len(milestones))
	for _, m := range milestones {
		dtos = append(dtos, milestoneModelToDTO(m))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"milestones": dtos})
}

// SyncMilestones handles POST /api/v1/milestones/sync.
func (h *MilestoneHandler) SyncMilestones(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	var body struct {
		Milestones []milestoneDTO `json:"milestones"`
		DeletedIDs []string       `json:"deleted_ids"`
		LastSyncAt time.Time      `json:"last_sync_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	milestones := make([]model.Milestone, 0, len(body.Milestones))
	for _, dto := range body.Milestones {
		m, err := milestoneDTOToModel(dto)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid date format")
			return
		}
		m.UserID = userID
		milestones = append(milestones, *m)
	}

	resp, err := h.svc.Sync(r.Context(), userID, service.MilestoneSyncRequest{
		Milestones: milestones,
		DeletedIDs: body.DeletedIDs,
		LastSyncAt: body.LastSyncAt,
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "sync failed")
		return
	}

	outDTOs := make([]milestoneDTO, 0, len(resp.Milestones))
	for i := range resp.Milestones {
		outDTOs = append(outDTOs, milestoneModelToDTO(&resp.Milestones[i]))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"milestones":  outDTOs,
		"deleted_ids": resp.DeletedIDs,
	})
}

// DeleteMilestone handles DELETE /api/v1/milestones/{id}.
func (h *MilestoneHandler) DeleteMilestone(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "missing milestone id")
		return
	}

	if err := h.svc.DeleteMilestone(r.Context(), userID, id); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "delete failed")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
