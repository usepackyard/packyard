package model

import (
	"time"

	"github.com/uptrace/bun"
)

type Session struct {
	bun.BaseModel `bun:"table:sessions,alias:s" json:"-"`

	ID        string    `bun:"id,pk" json:"id"`
	UserID    int64     `bun:"user_id,notnull" json:"user_id"`
	ExpiresAt time.Time `bun:"expires_at,notnull" json:"expires_at"`
	CreatedAt time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
}
