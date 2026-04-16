package model

import (
	"time"

	"github.com/uptrace/bun"
)

type Package struct {
	bun.BaseModel `bun:"table:packages,alias:p" json:"-"`

	ID          int64     `bun:"id,pk,autoincrement" json:"-"`
	PublicID    string    `bun:"public_id,notnull,unique" json:"id"`
	OrgID       int64     `bun:"org_id,notnull,unique:pkg_org_name" json:"org_id"`
	Name        string    `bun:"name,notnull,unique:pkg_org_name" json:"name"`
	Type        string    `bun:"type,notnull" json:"type"`
	Description string    `bun:"description" json:"description"`
	Homepage    string    `bun:"homepage" json:"homepage,omitempty"`
	Versions    []Version `bun:"rel:has-many,join:id=package_id" json:"versions,omitempty"`
	CreatedAt   time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt   time.Time `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`
}

type Version struct {
	bun.BaseModel `bun:"table:versions,alias:v" json:"-"`

	ID                int64      `bun:"id,pk,autoincrement" json:"-"`
	PublicID          string     `bun:"public_id,notnull,unique" json:"id"`
	PackageID         int64      `bun:"package_id,notnull" json:"-"`
	Version           string     `bun:"version,notnull" json:"version"`
	VersionNormalized string     `bun:"version_normalized,notnull" json:"version_normalized"`
	DistType          string     `bun:"dist_type,notnull" json:"dist_type"`
	DistSHA1          string     `bun:"dist_sha1,notnull" json:"dist_sha1"`
	StoragePath       string     `bun:"storage_path,notnull" json:"-"`
	FileSize          int64      `bun:"file_size,notnull,default:0" json:"file_size"`
	ComposerJSON      string     `bun:"composer_json,notnull,type:text" json:"-"`
	RequireJSON       string     `bun:"require_json,type:text" json:"-"`
	DownloadCount     int64      `bun:"download_count,notnull,default:0" json:"download_count"`
	LastDownloadedAt  *time.Time `bun:"last_downloaded_at" json:"last_downloaded_at,omitempty"`
	CreatedAt         time.Time  `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
}
