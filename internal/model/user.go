package model

import "time"

// User represents a Goalden user linked to a Supabase auth identity.
type User struct {
	ID        string    // Supabase auth user ID (UUID)
	Email     string
	CreatedAt time.Time
}
