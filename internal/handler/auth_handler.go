package handler

import (
	"encoding/json"
	"net/http"

	"github.com/goalden/goalden-api/internal/middleware"
	"github.com/goalden/goalden-api/internal/repository"
)

// AuthHandler handles authentication-related endpoints.
type AuthHandler struct {
	users repository.UserRepository
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(users repository.UserRepository) *AuthHandler {
	return &AuthHandler{users: users}
}

// SyncUser handles POST /api/v1/auth/sync-user.
// Creates or updates the user record after a successful Supabase login.
// This endpoint is idempotent — safe to call multiple times for the same user.
func (h *AuthHandler) SyncUser(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.users.UpsertUser(r.Context(), userID, body.Email); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to sync user")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
