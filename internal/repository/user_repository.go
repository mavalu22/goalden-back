package repository

import (
	"context"

	"github.com/goalden/goalden-api/internal/model"
)

// UserRepository defines persistence operations for users.
type UserRepository interface {
	UpsertUser(ctx context.Context, id, email string) error
	GetUser(ctx context.Context, id string) (*model.User, error)
}
