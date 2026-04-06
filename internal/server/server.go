package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/goalden/goalden-api/internal/config"
	"github.com/goalden/goalden-api/internal/handler"
	appmiddleware "github.com/goalden/goalden-api/internal/middleware"
	"github.com/goalden/goalden-api/internal/repository/postgres"
	"github.com/goalden/goalden-api/internal/service"
)

// New constructs the HTTP router with all routes wired up.
func New(cfg *config.Config, db *pgxpool.Pool) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware(cfg.AllowedOrigins))

	healthHandler := handler.NewHealthHandler(db)
	r.Get("/health", healthHandler.Health)

	// Repositories
	userRepo := postgres.NewUserRepo(db)
	taskRepo := postgres.NewTaskRepo(db)

	// Services
	taskSvc := service.NewTaskService(taskRepo)

	// Handlers
	authHandler := handler.NewAuthHandler(userRepo)
	taskHandler := handler.NewTaskHandler(taskSvc)

	// Auth middleware
	authMiddleware := appmiddleware.NewAuthMiddleware(cfg.SupabaseURL, cfg.SupabaseServiceRoleKey)

	r.Route("/api/v1", func(r chi.Router) {
		// Sync user after login (requires valid Supabase token)
		r.With(authMiddleware.Authenticate).Post("/auth/sync-user", authHandler.SyncUser)

		// Task routes — all require authentication
		r.Group(func(r chi.Router) {
			r.Use(authMiddleware.Authenticate)
			r.Get("/tasks", taskHandler.GetTasks)
			r.Post("/tasks/sync", taskHandler.SyncTasks)
			r.Delete("/tasks/{id}", taskHandler.DeleteTask)
		})
	})

	return r
}
