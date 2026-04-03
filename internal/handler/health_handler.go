package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// HealthHandler holds dependencies for the health endpoint.
type HealthHandler struct {
	db *pgxpool.Pool
}

// NewHealthHandler creates a HealthHandler with an optional db pool.
func NewHealthHandler(db *pgxpool.Pool) *HealthHandler {
	return &HealthHandler{db: db}
}

// Health handles GET /health — returns service and database status.
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	dbStatus := "connected"

	if h.db != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := h.db.Ping(ctx); err != nil {
			dbStatus = "unavailable"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"status":    "ok",
		"db":        dbStatus,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}
