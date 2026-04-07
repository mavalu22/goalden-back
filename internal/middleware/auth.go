package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type contextKey string

const userIDKey contextKey = "userID"

// tokenCacheEntry holds a verified user ID and its expiry time.
type tokenCacheEntry struct {
	userID    string
	expiresAt time.Time
}

// tokenCache is a simple in-memory cache for verified JWT→userID mappings.
// Avoids a remote Supabase API call on every request for the same token.
type tokenCache struct {
	mu    sync.RWMutex
	store map[string]tokenCacheEntry
}

func newTokenCache() *tokenCache {
	return &tokenCache{store: make(map[string]tokenCacheEntry)}
}

func (c *tokenCache) get(token string) (string, bool) {
	c.mu.RLock()
	entry, ok := c.store[token]
	c.mu.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		return "", false
	}
	return entry.userID, true
}

func (c *tokenCache) set(token, userID string, ttl time.Duration) {
	c.mu.Lock()
	c.store[token] = tokenCacheEntry{userID: userID, expiresAt: time.Now().Add(ttl)}
	c.mu.Unlock()
}

// evictExpired removes entries that have passed their TTL.
// Call periodically to reclaim memory (run from a background goroutine).
func (c *tokenCache) evictExpired() {
	now := time.Now()
	c.mu.Lock()
	for k, v := range c.store {
		if now.After(v.expiresAt) {
			delete(c.store, k)
		}
	}
	c.mu.Unlock()
}

const (
	// tokenCacheTTL is how long a verified token is trusted before re-checking
	// with Supabase. Supabase access tokens expire after 1 hour; 5 minutes is
	// a safe window that avoids hammering the Supabase API.
	tokenCacheTTL = 5 * time.Minute

	// supabaseAuthTimeout is the per-request deadline for calls to the
	// Supabase /auth/v1/user endpoint. Prevents slow Supabase responses from
	// stalling the entire sync handler indefinitely.
	supabaseAuthTimeout = 10 * time.Second
)

// AuthMiddleware validates Supabase JWTs by calling the Supabase user endpoint.
type AuthMiddleware struct {
	supabaseURL            string
	supabaseServiceRoleKey string
	cache                  *tokenCache
	httpClient             *http.Client
}

// NewAuthMiddleware creates a new AuthMiddleware with a token cache and a
// timeout-aware HTTP client.
func NewAuthMiddleware(supabaseURL, supabaseServiceRoleKey string) *AuthMiddleware {
	m := &AuthMiddleware{
		supabaseURL:            supabaseURL,
		supabaseServiceRoleKey: supabaseServiceRoleKey,
		cache:                  newTokenCache(),
		httpClient: &http.Client{
			Timeout: supabaseAuthTimeout,
		},
	}
	// Background goroutine to evict stale cache entries every minute.
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			m.cache.evictExpired()
		}
	}()
	return m
}

// Authenticate is a Chi-compatible middleware that validates the Bearer token
// against Supabase's /auth/v1/user endpoint and stores the user ID in context.
// Results are cached per token for tokenCacheTTL to avoid redundant API calls.
func (m *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		if token == "" {
			writeJSONError(w, http.StatusUnauthorized, "missing or invalid authorization header")
			return
		}

		// Fast path: serve from cache.
		if userID, ok := m.cache.get(token); ok {
			ctx := context.WithValue(r.Context(), userIDKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Slow path: verify with Supabase and populate cache.
		userID, err := m.verifyToken(r.Context(), token)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		m.cache.set(token, userID, tokenCacheTTL)

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

	resp, err := m.httpClient.Do(req)
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
