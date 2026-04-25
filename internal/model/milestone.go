package model

import "time"

// Milestone represents a checkpoint on the way to a Goal.
type Milestone struct {
	ID          string
	GoalID      string
	UserID      string
	Title       string
	Date        time.Time
	Done        bool
	CompletedAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time
}
