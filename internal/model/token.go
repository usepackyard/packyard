package model

import (
	"time"

	"github.com/uptrace/bun"
)

type APIToken struct {
	bun.BaseModel `bun:"table:api_tokens,alias:t" json:"-"`

	ID          int64      `bun:"id,pk,autoincrement" json:"id"`
	OrgID       int64      `bun:"org_id,notnull" json:"org_id"`
	Name        string     `bun:"name,notnull" json:"name"`
	TokenHash    string     `bun:"token_hash,notnull,unique" json:"-"`
	PasswordHash string     `bun:"password_hash,notnull" json:"-"`
	TokenPrefix  string     `bun:"token_prefix,notnull" json:"token_prefix"`
	LastUsedAt  *time.Time `bun:"last_used_at" json:"last_used_at,omitempty"`
	ExpiresAt   *time.Time `bun:"expires_at" json:"expires_at,omitempty"`
	IsActive    bool       `bun:"is_active,notnull,default:true" json:"is_active"`
	CreatedBy   *int64     `bun:"created_by" json:"created_by,omitempty"`
	CreatedAt   time.Time  `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
}
