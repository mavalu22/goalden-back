package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/goalden/goalden-api/internal/model"
)

// UserRepo is the Postgres implementation of repository.UserRepository.
type UserRepo struct {
	pool *pgxpool.Pool
}

// NewUserRepo creates a new UserRepo.
func NewUserRepo(pool *pgxpool.Pool) *UserRepo {
	return &UserRepo{pool: pool}
}

// UpsertUser creates or updates a user record. Safe to call multiple times for the same user.
func (r *UserRepo) UpsertUser(ctx context.Context, id, email string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO users (id, email)
		VALUES ($1, $2)
		ON CONFLICT (id) DO UPDATE SET email = EXCLUDED.email
	`, id, email)
	if err != nil {
		return fmt.Errorf("upsert user: %w", err)
	}
	return nil
}

// GetUser retrieves a user by their ID.
func (r *UserRepo) GetUser(ctx context.Context, id string) (*model.User, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, email, created_at
		FROM users
		WHERE id = $1
	`, id)

	u := &model.User{}
	if err := row.Scan(&u.ID, &u.Email, &u.CreatedAt); err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	return u, nil
}
