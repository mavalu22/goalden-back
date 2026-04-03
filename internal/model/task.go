package model

import "time"

// Task represents a user task stored in the cloud database.
type Task struct {
	ID             string
	UserID         string
	Title          string
	Date           time.Time
	Priority       string     // "normal" | "high"
	Note           *string
	Done           bool
	Recurrence     string     // "none" | "daily" | "weekly" | "custom_days"
	RecurrenceDays *string    // JSON array of ints, e.g. "[1,3,5]"
	SortOrder      int
	CreatedAt      time.Time
	UpdatedAt      time.Time
	CompletedAt    *time.Time
}
