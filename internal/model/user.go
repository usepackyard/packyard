package model

import (
	"time"

	"github.com/uptrace/bun"
)

type User struct {
	bun.BaseModel `bun:"table:users,alias:u" json:"-"`

	ID            int64     `bun:"id,pk,autoincrement" json:"-"`
	PublicID      string    `bun:"public_id,notnull,unique" json:"id"`
	Email         string    `bun:"email,notnull,unique" json:"email"`
	Password      string    `bun:"password,notnull" json:"-"`
	Name          string    `bun:"name,notnull" json:"name"`
	Language      string    `bun:"language,notnull,default:'en'" json:"language"`
	IsActive      bool      `bun:"is_active,notnull,default:true" json:"is_active"`
	IsSuperAdmin  bool      `bun:"is_super_admin,notnull,default:false" json:"is_super_admin"`
	CreatedAt     time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt     time.Time `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`
}
