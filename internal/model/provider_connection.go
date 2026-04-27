package model

import (
	"time"

	"github.com/uptrace/bun"
)

const (
	ProviderAuthNone  = "none"
	ProviderAuthToken = "token"
)

type ProviderConnection struct {
	bun.BaseModel `bun:"table:provider_connections,alias:pc" json:"-"`

	ID              int64     `bun:"id,pk,autoincrement" json:"-"`
	PublicID        string    `bun:"public_id,notnull,unique" json:"id"`
	OrgID           int64     `bun:"org_id,notnull" json:"-"`
	Name            string    `bun:"name,notnull" json:"name"`
	Provider        string    `bun:"provider,notnull" json:"provider"`
	AuthType        string    `bun:"auth_type,notnull" json:"auth_type"`
	SecretEncrypted string    `bun:"secret_encrypted,type:text" json:"-"`
	TokenPrefix     string    `bun:"token_prefix" json:"token_prefix,omitempty"`
	Config          string    `bun:"config,type:text" json:"-"`
	CreatedBy       *int64    `bun:"created_by" json:"-"`
	CreatedAt       time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt       time.Time `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`
}
