package model

import (
	"time"

	"github.com/uptrace/bun"
)

// SSOTicket stores a one-time login ticket.
type SSOTicket struct {
	bun.BaseModel `bun:"table:sso_tickets,alias:st" json:"-"`

	ID         int64      `bun:"id,pk,autoincrement" json:"id"`
	TokenHash  string     `bun:"token_hash,notnull,unique" json:"-"`
	UserID     int64      `bun:"user_id,notnull" json:"user_id"`
	Audience   string     `bun:"audience,notnull" json:"audience"`
	RedirectTo string     `bun:"redirect_to,notnull,default:'/'" json:"redirect_to"`
	ExpiresAt  time.Time  `bun:"expires_at,notnull" json:"expires_at"`
	ConsumedAt *time.Time `bun:"consumed_at" json:"consumed_at,omitempty"`
	CreatedAt  time.Time  `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
}
