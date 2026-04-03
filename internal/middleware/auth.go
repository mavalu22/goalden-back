package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type contextKey string

const userIDKey contextKey = "userID"

// AuthMiddleware validates Supabase JWTs by calling the Supabase user endpoint.
type AuthMiddleware struct {
	supabaseURL            string
	supabaseServiceRoleKey string
}

// NewAuthMiddleware creates a new AuthMiddleware.
func NewAuthMiddleware(supabaseURL, supabaseServiceRoleKey string) *AuthMiddleware {
	return &AuthMiddleware{
		supabaseURL:            supabaseURL,
		supabaseServiceRoleKey: supabaseServiceRoleKey,
	}
}

// Authenticate is a Chi-compatible middleware that validates the Bearer token
// against Supabase's /auth/v1/user endpoint and stores the user ID in context.
func (m *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		if token == "" {
			writeJSONError(w, http.StatusUnauthorized, "missing or invalid authorization header")
			return
		}

		userID, err := m.verifyToken(r.Context(), token)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}

		ctx := context.WithValue(r.Context(), userIDKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// UserIDFromContext extracts the authenticated user ID from the request context.
func UserIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(userIDKey).(string)
	return id, ok && id != ""
}

// extractBearerToken reads the Bearer token from the Authorization header.
func extractBearerToken(r *http.Request) string {
	header := r.Header.Get("Authorization")
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// verifyToken calls Supabase's /auth/v1/user endpoint to validate the token
// and returns the authenticated user's ID.
func (m *AuthMiddleware) verifyToken(ctx context.Context, token string) (string, error) {
	url := fmt.Sprintf("%s/auth/v1/user", m.supabaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("create supabase request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("apikey", m.supabaseServiceRoleKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("supabase request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("supabase returned status %d", resp.StatusCode)
	}

	var payload struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode supabase response: %w", err)
	}
	if payload.ID == "" {
		return "", fmt.Errorf("supabase returned empty user ID")
	}
	return payload.ID, nil
}

// writeJSONError writes a structured JSON error response.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
