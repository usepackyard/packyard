package model

import (
	"time"

	"github.com/uptrace/bun"
)

type PackageSource struct {
	bun.BaseModel `bun:"table:package_sources,alias:ps" json:"-"`

	ID           int64  `bun:"id,pk,autoincrement" json:"-"`
	PublicID     string `bun:"public_id,notnull,unique" json:"id"`
	PackageID    int64  `bun:"package_id,notnull,unique" json:"-"`
	Provider     string `bun:"provider,notnull" json:"provider"`
	RepoOwner    string `bun:"repo_owner,notnull" json:"repo_owner"`
	RepoName     string `bun:"repo_name,notnull" json:"repo_name"`
	Strategy     string `bun:"strategy,notnull" json:"strategy"`
	AssetPattern string `bun:"asset_pattern" json:"asset_pattern"`
	// MetadataSource controls where composer.json content comes from for
	// each synced version:
	//   from_zip — read composer.json from inside the dist zip (default).
	//   manual   — synthesize composer.json from the Package row + the
	//              user-supplied ManualRequire. Used for release zips
	//              that legitimately don't ship composer.json
	//              (WordPress plugin distributions).
	MetadataSource string `bun:"metadata_source,notnull,default:'from_zip'" json:"metadata_source"`
	// VersionSource controls which version string becomes authoritative:
	//   auto          — composer.json's version if set, else git tag.
	//   git_tag       — always the git tag; composer.json's version field
	//                   is rewritten to match.
	//   composer_json — require composer.json to declare a version; skip
	//                   releases where it's empty.
	VersionSource string `bun:"version_source,notnull,default:'auto'" json:"version_source"`
	// ManualRequire is the JSON-encoded `require` block used when
	// MetadataSource=manual. Empty = no require.
	ManualRequire string     `bun:"manual_require,type:text" json:"manual_require,omitempty"`
	AuthToken     string     `bun:"auth_token" json:"-"`
	WebhookSecret string     `bun:"webhook_secret" json:"-"`
	LastSyncedAt  *time.Time `bun:"last_synced_at" json:"last_synced_at,omitempty"`
	CreatedAt     time.Time  `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt     time.Time  `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`
}
