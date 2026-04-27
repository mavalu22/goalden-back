package model

import "time"

// Goal represents a user goal stored in the cloud database.
type Goal struct {
	ID          string
	UserID      string
	Title       string
	Description *string
	Color       string     // hex color id from the palette
	Status      string     // "active" | "archived"
	Deadline    *time.Time // date only (UTC midnight)
	Starred     bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ArchivedAt  *time.Time
	DeletedAt   *time.Time // non-nil = soft-deleted; propagated during sync
}
