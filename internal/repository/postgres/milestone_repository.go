package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/goalden/goalden-api/internal/model"
	"github.com/goalden/goalden-api/internal/repository"
)

// MilestoneRepo is the Postgres implementation of repository.MilestoneRepository.
type MilestoneRepo struct {
	pool *pgxpool.Pool
}

// NewMilestoneRepo creates a new MilestoneRepo.
func NewMilestoneRepo(pool *pgxpool.Pool) *MilestoneRepo {
	return &MilestoneRepo{pool: pool}
}

const milestoneCols = `
	id, goal_id, user_id, title, date, done,
	completed_at, created_at, updated_at, deleted_at`

func scanMilestones(rows pgx.Rows) ([]*model.Milestone, error) {
	defer rows.Close()
	var out []*model.Milestone
	for rows.Next() {
		m := &model.Milestone{}
		if err := rows.Scan(
			&m.ID, &m.GoalID, &m.UserID, &m.Title, &m.Date, &m.Done,
			&m.CompletedAt, &m.CreatedAt, &m.UpdatedAt, &m.DeletedAt,
		); err != nil {
			return nil, fmt.Errorf("scan milestone: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetMilestonesForUser returns all non-deleted milestones for a user.
func (r *MilestoneRepo) GetMilestonesForUser(ctx context.Context, userID string) ([]*model.Milestone, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT`+milestoneCols+`
		FROM milestones
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY date ASC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("get milestones for user: %w", err)
	}
	return scanMilestones(rows)
}

// GetMilestonesUpdatedSince returns milestones (including soft-deleted) updated after since.
func (r *MilestoneRepo) GetMilestonesUpdatedSince(ctx context.Context, userID string, since time.Time) ([]*model.Milestone, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT`+milestoneCols+`
		FROM milestones
		WHERE user_id = $1 AND updated_at > $2
		ORDER BY updated_at ASC
	`, userID, since)
	if err != nil {
		return nil, fmt.Errorf("get milestones updated since: %w", err)
	}
	return scanMilestones(rows)
}

// GetDeletedMilestoneIDsSince returns (id, deleted_at) pairs for milestones soft-deleted after since.
func (r *MilestoneRepo) GetDeletedMilestoneIDsSince(ctx context.Context, userID string, since time.Time) ([]repository.DeletedMilestoneRef, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, deleted_at FROM milestones
		WHERE user_id = $1 AND deleted_at > $2
		ORDER BY deleted_at ASC
	`, userID, since)
	if err != nil {
		return nil, fmt.Errorf("get deleted milestone ids since: %w", err)
	}
	defer rows.Close()

	var refs []repository.DeletedMilestoneRef
	for rows.Next() {
		var ref repository.DeletedMilestoneRef
		if err := rows.Scan(&ref.ID, &ref.DeletedAt); err != nil {
			return nil, fmt.Errorf("scan deleted milestone ref: %w", err)
		}
		refs = append(refs, ref)
	}
	return refs, rows.Err()
}

// UpsertMilestone inserts or updates a milestone using last-write-wins.
func (r *MilestoneRepo) UpsertMilestone(ctx context.Context, m *model.Milestone) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO milestones (
			id, goal_id, user_id, title, date, done,
			completed_at, created_at, updated_at, deleted_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (id) DO UPDATE
			SET goal_id      = EXCLUDED.goal_id,
			    title        = EXCLUDED.title,
			    date         = EXCLUDED.date,
			    done         = EXCLUDED.done,
			    completed_at = EXCLUDED.completed_at,
			    updated_at   = EXCLUDED.updated_at,
			    deleted_at   = EXCLUDED.deleted_at
			WHERE EXCLUDED.updated_at > milestones.updated_at
	`,
		m.ID, m.GoalID, m.UserID, m.Title, m.Date, m.Done,
		m.CompletedAt, m.CreatedAt, m.UpdatedAt, m.DeletedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert milestone: %w", err)
	}
	return nil
}

// DeleteMilestone soft-deletes a milestone (sets deleted_at).
func (r *MilestoneRepo) DeleteMilestone(ctx context.Context, id, userID string) error {
	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx, `
		UPDATE milestones
		SET deleted_at = $1, updated_at = $1
		WHERE id = $2 AND user_id = $3 AND deleted_at IS NULL
	`, now, id, userID)
	if err != nil {
		return fmt.Errorf("delete milestone: %w", err)
	}
	return nil
}

// BatchUpsertMilestones upserts multiple milestones in a single transaction.
func (r *MilestoneRepo) BatchUpsertMilestones(ctx context.Context, milestones []*model.Milestone) error {
	if len(milestones) == 0 {
		return nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin batch upsert milestones: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	for _, m := range milestones {
		_, err := tx.Exec(ctx, `
			INSERT INTO milestones (
				id, goal_id, user_id, title, date, done,
				completed_at, created_at, updated_at, deleted_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
			ON CONFLICT (id) DO UPDATE
				SET goal_id      = EXCLUDED.goal_id,
				    title        = EXCLUDED.title,
				    date         = EXCLUDED.date,
				    done         = EXCLUDED.done,
				    completed_at = EXCLUDED.completed_at,
				    updated_at   = EXCLUDED.updated_at,
				    deleted_at   = EXCLUDED.deleted_at
				WHERE EXCLUDED.updated_at > milestones.updated_at
		`,
			m.ID, m.GoalID, m.UserID, m.Title, m.Date, m.Done,
			m.CompletedAt, m.CreatedAt, m.UpdatedAt, m.DeletedAt,
		)
		if err != nil {
			return fmt.Errorf("batch upsert milestone %s: %w", m.ID, err)
		}
	}
	return tx.Commit(ctx)
}

// BatchDeleteMilestones soft-deletes multiple milestones in a single transaction.
func (r *MilestoneRepo) BatchDeleteMilestones(ctx context.Context, ids []string, userID string) error {
	if len(ids) == 0 {
		return nil
	}
	now := time.Now().UTC()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin batch delete milestones: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	for _, id := range ids {
		_, err := tx.Exec(ctx, `
			UPDATE milestones
			SET deleted_at = $1, updated_at = $1
			WHERE id = $2 AND user_id = $3 AND deleted_at IS NULL
		`, now, id, userID)
		if err != nil {
			return fmt.Errorf("batch delete milestone %s: %w", id, err)
		}
	}
	return tx.Commit(ctx)
}
