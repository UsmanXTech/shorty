package links

import "time"

type Link struct {
	ID        int64      `json:"id"`
	Slug      string     `json:"slug"`
	URL       string     `json:"url"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}
