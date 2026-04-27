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

// GoalRepo is the Postgres implementation of repository.GoalRepository.
type GoalRepo struct {
	pool *pgxpool.Pool
}

// NewGoalRepo creates a new GoalRepo.
func NewGoalRepo(pool *pgxpool.Pool) *GoalRepo {
	return &GoalRepo{pool: pool}
}

const goalSelectCols = `
	id, user_id, title, description, color, status,
	deadline, starred, created_at, updated_at, archived_at, deleted_at`

// GetGoalsForUser returns all non-deleted goals for a user. Used for initial pull.
func (r *GoalRepo) GetGoalsForUser(ctx context.Context, userID string) ([]*model.Goal, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT`+goalSelectCols+`
		FROM goals
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY created_at ASC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("get goals for user: %w", err)
	}
	defer rows.Close()
	return scanGoals(rows)
}

// GetGoalsUpdatedSince returns all goals (including soft-deleted) updated after since.
func (r *GoalRepo) GetGoalsUpdatedSince(ctx context.Context, userID string, since time.Time) ([]*model.Goal, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT`+goalSelectCols+`
		FROM goals
		WHERE user_id = $1 AND updated_at > $2
		ORDER BY updated_at ASC
	`, userID, since)
	if err != nil {
		return nil, fmt.Errorf("get goals updated since: %w", err)
	}
	defer rows.Close()
	return scanGoals(rows)
}

// GetDeletedGoalIDsSince returns (id, deleted_at) for goals soft-deleted after since.
func (r *GoalRepo) GetDeletedGoalIDsSince(ctx context.Context, userID string, since time.Time) ([]repository.DeletedGoalRef, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, deleted_at FROM goals
		WHERE user_id = $1 AND deleted_at > $2
		ORDER BY deleted_at ASC
	`, userID, since)
	if err != nil {
		return nil, fmt.Errorf("get deleted goal ids since: %w", err)
	}
	defer rows.Close()

	var refs []repository.DeletedGoalRef
	for rows.Next() {
		var ref repository.DeletedGoalRef
		if err := rows.Scan(&ref.ID, &ref.DeletedAt); err != nil {
			return nil, fmt.Errorf("scan deleted goal ref: %w", err)
		}
		refs = append(refs, ref)
	}
	return refs, rows.Err()
}

// BatchUpsertGoals upserts goals using last-write-wins on updated_at.
func (r *GoalRepo) BatchUpsertGoals(ctx context.Context, goals []*model.Goal) error {
	if len(goals) == 0 {
		return nil
	}

	ids := make([]string, len(goals))
	userIDs := make([]string, len(goals))
	titles := make([]string, len(goals))
	descriptions := make([]*string, len(goals))
	colors := make([]string, len(goals))
	statuses := make([]string, len(goals))
	deadlines := make([]*time.Time, len(goals))
	starreds := make([]bool, len(goals))
	createdAts := make([]time.Time, len(goals))
	updatedAts := make([]time.Time, len(goals))
	archivedAts := make([]*time.Time, len(goals))
	deletedAts := make([]*time.Time, len(goals))

	for i, g := range goals {
		ids[i] = g.ID
		userIDs[i] = g.UserID
		titles[i] = g.Title
		descriptions[i] = g.Description
		colors[i] = g.Color
		statuses[i] = g.Status
		deadlines[i] = g.Deadline
		starreds[i] = g.Starred
		createdAts[i] = g.CreatedAt
		updatedAts[i] = g.UpdatedAt
		archivedAts[i] = g.ArchivedAt
		deletedAts[i] = g.DeletedAt
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO goals (
			id, user_id, title, description, color, status,
			deadline, starred, created_at, updated_at, archived_at, deleted_at
		)
		SELECT * FROM unnest(
			$1::text[], $2::text[], $3::text[], $4::text[], $5::text[], $6::text[],
			$7::date[], $8::bool[], $9::timestamptz[], $10::timestamptz[], $11::timestamptz[], $12::timestamptz[]
		) AS g(
			id, user_id, title, description, color, status,
			deadline, starred, created_at, updated_at, archived_at, deleted_at
		)
		ON CONFLICT (id) DO UPDATE SET
			title       = EXCLUDED.title,
			description = EXCLUDED.description,
			color       = EXCLUDED.color,
			status      = EXCLUDED.status,
			deadline    = EXCLUDED.deadline,
			starred     = EXCLUDED.starred,
			updated_at  = EXCLUDED.updated_at,
			archived_at = EXCLUDED.archived_at,
			deleted_at  = EXCLUDED.deleted_at
		WHERE EXCLUDED.updated_at >= goals.updated_at
	`,
		ids, userIDs, titles, descriptions, colors, statuses,
		deadlines, starreds, createdAts, updatedAts, archivedAts, deletedAts,
	)
	if err != nil {
		return fmt.Errorf("batch upsert goals: %w", err)
	}
	return nil
}

// BatchDeleteGoals soft-deletes goals owned by userID.
func (r *GoalRepo) BatchDeleteGoals(ctx context.Context, ids []string, userID string) error {
	if len(ids) == 0 {
		return nil
	}
	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx, `
		UPDATE goals SET deleted_at = $1, updated_at = $1
		WHERE id = ANY($2) AND user_id = $3 AND deleted_at IS NULL
	`, now, ids, userID)
	if err != nil {
		return fmt.Errorf("batch delete goals: %w", err)
	}
	return nil
}

// DeleteGoal soft-deletes a single goal owned by userID.
func (r *GoalRepo) DeleteGoal(ctx context.Context, id, userID string) error {
	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx, `
		UPDATE goals SET deleted_at = $1, updated_at = $1
		WHERE id = $2 AND user_id = $3 AND deleted_at IS NULL
	`, now, id, userID)
	return err
}

func scanGoals(rows pgx.Rows) ([]*model.Goal, error) {
	var result []*model.Goal
	for rows.Next() {
		g := &model.Goal{}
		if err := rows.Scan(
			&g.ID, &g.UserID, &g.Title, &g.Description, &g.Color, &g.Status,
			&g.Deadline, &g.Starred, &g.CreatedAt, &g.UpdatedAt, &g.ArchivedAt, &g.DeletedAt,
		); err != nil {
			return nil, fmt.Errorf("scan goal: %w", err)
		}
		result = append(result, g)
	}
	return result, rows.Err()
}
