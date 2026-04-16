package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/uptrace/bun"

	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/pid"
)

type sourceStoreDB struct {
	db *bun.DB
}

func NewSourceStoreDB(db *bun.DB) SourceStore {
	return &sourceStoreDB{db: db}
}

func (s *sourceStoreDB) GetByPackageID(ctx context.Context, packageID int64) (*model.PackageSource, error) {
	src := new(model.PackageSource)
	err := s.db.NewSelect().Model(src).Where("package_id = ?", packageID).Scan(ctx)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return src, nil
}

func (s *sourceStoreDB) GetByRepo(ctx context.Context, provider, owner, name string) (*model.PackageSource, error) {
	src := new(model.PackageSource)
	err := s.db.NewSelect().Model(src).
		Where("provider = ?", provider).
		Where("repo_owner = ?", owner).
		Where("repo_name = ?", name).
		Scan(ctx)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return src, nil
}

func (s *sourceStoreDB) Create(ctx context.Context, src *model.PackageSource) error {
	now := time.Now()
	src.CreatedAt = now
	src.UpdatedAt = now
	if src.PublicID == "" {
		src.PublicID = pid.New(pid.PackageSource)
	}
	_, err := s.db.NewInsert().Model(src).Returning("id").Exec(ctx)
	return err
}

func (s *sourceStoreDB) Update(ctx context.Context, src *model.PackageSource) error {
	src.UpdatedAt = time.Now()
	_, err := s.db.NewUpdate().Model(src).
		Column("provider", "repo_owner", "repo_name", "strategy", "asset_pattern", "auth_token", "webhook_secret", "updated_at").
		Where("package_id = ?", src.PackageID).
		Exec(ctx)
	return err
}

func (s *sourceStoreDB) Delete(ctx context.Context, packageID int64) error {
	_, err := s.db.NewDelete().Model((*model.PackageSource)(nil)).Where("package_id = ?", packageID).Exec(ctx)
	return err
}

func (s *sourceStoreDB) UpdateLastSynced(ctx context.Context, packageID int64) error {
	now := time.Now()
	_, err := s.db.NewUpdate().Model((*model.PackageSource)(nil)).
		Set("last_synced_at = ?", now).
		Where("package_id = ?", packageID).
		Exec(ctx)
	return err
}
