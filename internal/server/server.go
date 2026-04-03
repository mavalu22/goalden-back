package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/goalden/goalden-api/internal/config"
	"github.com/goalden/goalden-api/internal/handler"
)

func New(cfg *config.Config) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware(cfg.AllowedOrigins))

	r.Get("/health", handler.Health)

	r.Route("/api/v1", func(r chi.Router) {
		// Protected routes will be added as tasks are implemented
	})

	return r
}
