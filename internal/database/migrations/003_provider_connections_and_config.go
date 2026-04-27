package migrations

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/usepackyard/packyard/internal/credentials"
	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/pid"
)

type legacySourceRow struct {
	ID           int64
	PackageID    int64
	Provider     string
	RepoOwner    string
	RepoName     string
	Strategy     string
	AssetPattern string
	AuthToken    string
	OrgID        int64
	PackageName  string
}

type migrationSourceConfig struct {
	Owner        string `json:"owner"`
	Repo         string `json:"repo"`
	Strategy     string `json:"strategy"`
	AssetPattern string `json:"asset_pattern"`
}

type migrationConnectionConfig struct {
	Host string `json:"host,omitempty"`
}

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		if _, err := db.NewCreateTable().Model((*model.ProviderConnection)(nil)).IfNotExists().Exec(ctx); err != nil {
			return fmt.Errorf("create provider_connections: %w", err)
		}
		if _, err := db.NewCreateIndex().Model((*model.ProviderConnection)(nil)).Index("idx_provider_connections_org_id").Column("org_id").IfNotExists().Exec(ctx); err != nil {
			return fmt.Errorf("index provider_connections.org_id: %w", err)
		}
		if _, err := db.ExecContext(ctx, "CREATE UNIQUE INDEX idx_provider_connections_public_id ON provider_connections (public_id)"); err != nil {
			if !isAlreadyExists(err) {
				return fmt.Errorf("index provider_connections.public_id: %w", err)
			}
		}

		if err := addColumnIfMissing(ctx, db, "package_sources", "connection_id", "BIGINT"); err != nil {
			return err
		}
		if err := addColumnIfMissing(ctx, db, "package_sources", "provider_config", "TEXT"); err != nil {
			return err
		}
		if err := addColumnIfMissing(ctx, db, "package_sources", "repo_key", "VARCHAR(512)"); err != nil {
			return err
		}

		hasLegacyRepoOwner, err := columnExists(ctx, db, "package_sources", "repo_owner")
		if err != nil {
			return err
		}
		if hasLegacyRepoOwner {
			if err := migrateLegacySources(ctx, db); err != nil {
				return err
			}
			for _, col := range []string{"repo_owner", "repo_name", "strategy", "asset_pattern", "auth_token"} {
				if err := dropColumnIfExists(ctx, db, "package_sources", col); err != nil {
					return err
				}
			}
		}

		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		for _, col := range []struct {
			name string
			typ  string
		}{
			{"repo_owner", "VARCHAR NOT NULL DEFAULT ''"},
			{"repo_name", "VARCHAR NOT NULL DEFAULT ''"},
			{"strategy", "VARCHAR NOT NULL DEFAULT ''"},
			{"asset_pattern", "VARCHAR"},
			{"auth_token", "VARCHAR"},
		} {
			if err := addColumnIfMissing(ctx, db, "package_sources", col.name, col.typ); err != nil {
				return err
			}
		}

		rows, err := db.QueryContext(ctx, "SELECT id, provider, provider_config FROM package_sources")
		if err != nil {
			return fmt.Errorf("scan package_sources for down migration: %w", err)
		}
		defer rows.Close()
		type row struct {
			id       int64
			provider string
			raw      string
		}
		var sources []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.id, &r.provider, &r.raw); err != nil {
				return fmt.Errorf("scan package_sources row: %w", err)
			}
			sources = append(sources, r)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for _, r := range sources {
			cfg := migrationSourceConfig{}
			if strings.TrimSpace(r.raw) != "" {
				_ = json.Unmarshal([]byte(r.raw), &cfg)
			}
			if _, err := db.ExecContext(ctx,
				"UPDATE package_sources SET repo_owner = ?, repo_name = ?, strategy = ?, asset_pattern = ? WHERE id = ?",
				cfg.Owner, cfg.Repo, cfg.Strategy, cfg.AssetPattern, r.id); err != nil {
				return fmt.Errorf("restore legacy source columns: %w", err)
			}
		}

		for _, col := range []string{"connection_id", "provider_config", "repo_key"} {
			if err := dropColumnIfExists(ctx, db, "package_sources", col); err != nil {
				return err
			}
		}
		if _, err := db.NewDropTable().Model((*model.ProviderConnection)(nil)).IfExists().Cascade().Exec(ctx); err != nil {
			return fmt.Errorf("drop provider_connections: %w", err)
		}
		return nil
	})
}

func migrateLegacySources(ctx context.Context, db *bun.DB) error {
	rows, err := db.QueryContext(ctx, `
		SELECT ps.id, ps.package_id, ps.provider,
		       COALESCE(ps.repo_owner, ''), COALESCE(ps.repo_name, ''),
		       COALESCE(ps.strategy, ''), COALESCE(ps.asset_pattern, ''),
		       COALESCE(ps.auth_token, ''), p.org_id, p.name
		  FROM package_sources AS ps
		  JOIN packages AS p ON p.id = ps.package_id
	`)
	if err != nil {
		return fmt.Errorf("scan legacy package_sources: %w", err)
	}
	defer rows.Close()

	var sources []legacySourceRow
	for rows.Next() {
		var r legacySourceRow
		if err := rows.Scan(&r.ID, &r.PackageID, &r.Provider, &r.RepoOwner, &r.RepoName, &r.Strategy, &r.AssetPattern, &r.AuthToken, &r.OrgID, &r.PackageName); err != nil {
			return fmt.Errorf("scan legacy package_sources row: %w", err)
		}
		sources = append(sources, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	key := os.Getenv("PACKYARD_CREDENTIALS_KEY")
	for _, src := range sources {
		providerConfig := ""
		repoKey := ""
		if src.Provider != "upload" {
			strategy := src.Strategy
			if strategy == "" {
				strategy = "release_asset"
			}
			assetPattern := src.AssetPattern
			if assetPattern == "" {
				assetPattern = "*.zip"
			}
			cfg := migrationSourceConfig{
				Owner:        src.RepoOwner,
				Repo:         src.RepoName,
				Strategy:     strategy,
				AssetPattern: assetPattern,
			}
			raw, err := json.Marshal(cfg)
			if err != nil {
				return err
			}
			providerConfig = string(raw)
			repoKey = migrationRepoKey(src.Provider, "", src.RepoOwner, src.RepoName)
		}

		var connectionID *int64
		if strings.TrimSpace(src.AuthToken) != "" && src.Provider != "upload" {
			if _, err := credentials.DecodeKey(key); err != nil {
				return fmt.Errorf("migrate source %d token: PACKYARD_CREDENTIALS_KEY is required and must be 64 hex characters: %w", src.ID, err)
			}
			encrypted, err := credentials.EncryptString(src.AuthToken, key)
			if err != nil {
				return fmt.Errorf("encrypt source %d token: %w", src.ID, err)
			}
			connConfig := "{}"
			if src.Provider == "gitlab" {
				raw, _ := json.Marshal(migrationConnectionConfig{Host: "gitlab.com"})
				connConfig = string(raw)
			}
			now := time.Now()
			conn := &model.ProviderConnection{
				PublicID:        pid.New(pid.ProviderConnection),
				OrgID:           src.OrgID,
				Name:            fmt.Sprintf("Migrated %s credential for %s", displayProvider(src.Provider), src.PackageName),
				Provider:        src.Provider,
				AuthType:        model.ProviderAuthToken,
				SecretEncrypted: encrypted,
				TokenPrefix:     credentials.TokenPrefix(src.AuthToken),
				Config:          connConfig,
				CreatedAt:       now,
				UpdatedAt:       now,
			}
			if _, err := db.NewInsert().Model(conn).Returning("id").Exec(ctx); err != nil {
				return fmt.Errorf("create migrated provider connection: %w", err)
			}
			connectionID = &conn.ID
		}

		q := db.NewUpdate().Model((*model.PackageSource)(nil)).
			Set("provider_config = ?", providerConfig).
			Set("repo_key = ?", repoKey).
			Where("id = ?", src.ID)
		if connectionID != nil {
			q = q.Set("connection_id = ?", *connectionID)
		}
		if _, err := q.Exec(ctx); err != nil {
			return fmt.Errorf("backfill package_sources id=%d: %w", src.ID, err)
		}
	}
	return nil
}

func migrationRepoKey(providerName, host, owner, repo string) string {
	owner = strings.ToLower(strings.TrimSpace(owner))
	repo = strings.ToLower(strings.TrimSpace(repo))
	if providerName == "gitlab" {
		if host == "" {
			host = "gitlab.com"
		}
		return strings.ToLower(strings.TrimSpace(host)) + "/" + owner + "/" + repo
	}
	if owner == "" && repo == "" {
		return ""
	}
	return owner + "/" + repo
}

func displayProvider(providerName string) string {
	switch providerName {
	case "github":
		return "GitHub"
	case "gitlab":
		return "GitLab"
	default:
		if providerName == "" {
			return "provider"
		}
		return providerName
	}
}

func addColumnIfMissing(ctx context.Context, db *bun.DB, table, column, typ string) error {
	exists, err := columnExists(ctx, db, table, column)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, typ)); err != nil {
		if isAlreadyExists(err) {
			return nil
		}
		return fmt.Errorf("add %s.%s: %w", table, column, err)
	}
	return nil
}

func dropColumnIfExists(ctx context.Context, db *bun.DB, table, column string) error {
	exists, err := columnExists(ctx, db, table, column)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", table, column)); err != nil {
		if isMissingColumn(err) {
			return nil
		}
		return fmt.Errorf("drop %s.%s: %w", table, column, err)
	}
	return nil
}

func columnExists(ctx context.Context, db *bun.DB, table, column string) (bool, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("SELECT %s FROM %s WHERE 1 = 0", column, table))
	if err == nil {
		_ = rows.Close()
		return true, nil
	}
	if isMissingColumn(err) {
		return false, nil
	}
	return false, fmt.Errorf("check %s.%s: %w", table, column, err)
}

func isAlreadyExists(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate") ||
		strings.Contains(msg, "already") ||
		strings.Contains(msg, "exists")
}

func isMissingColumn(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such column") ||
		strings.Contains(msg, "unknown column") ||
		strings.Contains(msg, "does not exist")
}
