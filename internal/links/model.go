package links

import "time"

type Link struct {
	ID           int64      `json:"id"`
	Slug         string     `json:"slug"`
	URL          string     `json:"url"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	MaxClicks    *int64     `json:"max_clicks,omitempty"`
	Clicks       int64      `json:"clicks"`
	PasswordHash string     `json:"-"`
	TeamID       int64      `json:"team_id"`
	DomainID     *int64     `json:"domain_id,omitempty"`
}

// Protected reports whether the link requires a password to open.
func (l Link) Protected() bool {
	return l.PasswordHash != ""
}
