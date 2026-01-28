package model

import (
	"time"

	"github.com/uptrace/bun"
)

// Organization lifecycle status. Anything other than StatusActive blocks
// Composer and dashboard access for the org (402 for suspended, 404 for
// archived) while preserving data for reactivation / audit.
const (
	OrgStatusActive    = "active"
	OrgStatusSuspended = "suspended"
	OrgStatusArchived  = "archived"
)

type Organization struct {
	bun.BaseModel `bun:"table:organizations,alias:o" json:"-"`

	ID        int64     `bun:"id,pk,autoincrement" json:"id"`
	Slug      string    `bun:"slug,notnull,unique" json:"slug"`
	Name      string    `bun:"name,notnull" json:"name"`
	Status    string    `bun:"status,notnull,default:'active'" json:"status"`
	CreatedAt time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`
}

type OrgMember struct {
	bun.BaseModel `bun:"table:org_members,alias:m" json:"-"`

	ID          int64           `bun:"id,pk,autoincrement" json:"id"`
	OrgID       int64           `bun:"org_id,notnull,unique:org_user" json:"org_id"`
	UserID      int64           `bun:"user_id,notnull,unique:org_user" json:"user_id"`
	Role        string          `bun:"role,notnull" json:"role"`
	Permissions JSONStringSlice `bun:"permissions,notnull,type:text" json:"permissions"`
	CreatedAt   time.Time       `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	User        *User           `bun:"rel:belongs-to,join:user_id=id" json:"user,omitempty"`
}
