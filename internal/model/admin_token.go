package model

import (
	"time"

	"github.com/uptrace/bun"
)

// AdminToken is a long-lived Bearer token granting super-admin-equivalent
// access to /api/admin/*. Intended for machine-to-machine integrations
// (CI, external automation, provisioning scripts). Minted from the
// super-admin UI, shown once in plaintext, stored as a SHA-256 hash.
//
// Kept in a separate table from api_tokens (Composer-scope tokens) because:
//  - admin tokens aren't scoped to an org
//  - they have distinct auth paths (Bearer vs Basic)
//  - different audit/lifecycle expectations
type AdminToken struct {
	bun.BaseModel `bun:"table:admin_tokens,alias:at" json:"-"`

	ID          int64      `bun:"id,pk,autoincrement" json:"-"`
	PublicID    string     `bun:"public_id,notnull,unique" json:"id"`
	Name        string     `bun:"name,notnull" json:"name"`
	TokenHash   string     `bun:"token_hash,notnull,unique" json:"-"`
	TokenPrefix string     `bun:"token_prefix,notnull" json:"token_prefix"`
	CreatedBy   int64      `bun:"created_by,notnull" json:"-"`
	LastUsedAt  *time.Time `bun:"last_used_at" json:"last_used_at,omitempty"`
	ExpiresAt   *time.Time `bun:"expires_at" json:"expires_at,omitempty"`
	IsActive    bool       `bun:"is_active,notnull" json:"is_active"`
	CreatedAt   time.Time  `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
}
