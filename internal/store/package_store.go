package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/uptrace/bun"

	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/pid"
)

type packageStoreDB struct {
	db *bun.DB
}

func NewPackageStoreDB(db *bun.DB) PackageStore {
	return &packageStoreDB{db: db}
}

func (s *packageStoreDB) List(ctx context.Context, orgID int64) ([]model.Package, error) {
	var packages []model.Package
	err := s.db.NewSelect().Model(&packages).
		Where("org_id = ?", orgID).
		Order("name").
		Scan(ctx)
	return packages, err
}

func (s *packageStoreDB) GetByID(ctx context.Context, orgID, id int64) (*model.Package, error) {
	p := new(model.Package)
	err := s.db.NewSelect().Model(p).
		Where("org_id = ?", orgID).
		Where("id = ?", id).
		Scan(ctx)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (s *packageStoreDB) GetByIDGlobal(ctx context.Context, id int64) (*model.Package, error) {
	p := new(model.Package)
	err := s.db.NewSelect().Model(p).Where("id = ?", id).Scan(ctx)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (s *packageStoreDB) GetByName(ctx context.Context, orgID int64, name string) (*model.Package, error) {
	p := new(model.Package)
	err := s.db.NewSelect().Model(p).
		Where("org_id = ?", orgID).
		Where("name = ?", name).
		Scan(ctx)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (s *packageStoreDB) Create(ctx context.Context, pkg *model.Package) error {
	now := time.Now()
	pkg.CreatedAt = now
	pkg.UpdatedAt = now
	if pkg.PublicID == "" {
		pkg.PublicID = pid.New(pid.Package)
	}
	_, err := s.db.NewInsert().Model(pkg).Returning("id").Exec(ctx)
	return err
}

func (s *packageStoreDB) GetByPublicID(ctx context.Context, orgID int64, publicID string) (*model.Package, error) {
	p := new(model.Package)
	err := s.db.NewSelect().Model(p).
		Where("org_id = ?", orgID).
		Where("public_id = ?", publicID).
		Scan(ctx)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (s *packageStoreDB) GetByPublicIDGlobal(ctx context.Context, publicID string) (*model.Package, error) {
	p := new(model.Package)
	err := s.db.NewSelect().Model(p).Where("public_id = ?", publicID).Scan(ctx)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (s *packageStoreDB) Update(ctx context.Context, pkg *model.Package) error {
	pkg.UpdatedAt = time.Now()
	_, err := s.db.NewUpdate().Model(pkg).
		Column("name", "type", "description", "homepage", "updated_at").
		Where("id = ?", pkg.ID).
		Exec(ctx)
	return err
}

func (s *packageStoreDB) Delete(ctx context.Context, orgID, id int64) error {
	_, err := s.db.NewDelete().Model((*model.Package)(nil)).
		Where("org_id = ?", orgID).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (s *packageStoreDB) ListVersions(ctx context.Context, orgID, packageID int64) ([]model.Version, error) {
	var versions []model.Version
	err := s.db.NewSelect().Model(&versions).
		Join("JOIN packages AS p ON p.id = v.package_id").
		Where("p.org_id = ?", orgID).
		Where("v.package_id = ?", packageID).
		// Newest release first. Tiebreaker on version_normalized for
		// rows that share created_at (common when many versions were
		// imported in one pre-backfill batch) — ensures deterministic,
		// semver-ish order instead of DB insertion order.
		Order("v.created_at DESC", "v.version_normalized DESC").
		Scan(ctx)
	return versions, err
}

func (s *packageStoreDB) GetVersionByID(ctx context.Context, id int64) (*model.Version, error) {
	v := new(model.Version)
	err := s.db.NewSelect().Model(v).Where("id = ?", id).Scan(ctx)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (s *packageStoreDB) CreateVersion(ctx context.Context, v *model.Version) error {
	// Preserve a caller-supplied CreatedAt (used by sync to stamp the
	// upstream release date) and default to "now" for direct uploads.
	if v.CreatedAt.IsZero() {
		v.CreatedAt = time.Now()
	}
	if v.PublicID == "" {
		v.PublicID = pid.New(pid.Version)
	}
	_, err := s.db.NewInsert().Model(v).Returning("id").Exec(ctx)
	return err
}

func (s *packageStoreDB) GetVersionByPublicID(ctx context.Context, publicID string) (*model.Version, error) {
	v := new(model.Version)
	err := s.db.NewSelect().Model(v).Where("public_id = ?", publicID).Scan(ctx)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (s *packageStoreDB) DeleteVersion(ctx context.Context, id int64) error {
	_, err := s.db.NewDelete().Model((*model.Version)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

func (s *packageStoreDB) UpdateVersionCreatedAt(ctx context.Context, id int64, createdAt time.Time) error {
	_, err := s.db.NewUpdate().Model((*model.Version)(nil)).
		Set("created_at = ?", createdAt).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (s *packageStoreDB) IncrementDownload(ctx context.Context, versionID int64, at time.Time) error {
	_, err := s.db.NewUpdate().Model((*model.Version)(nil)).
		Set("download_count = download_count + 1").
		Set("last_downloaded_at = ?", at).
		Where("id = ?", versionID).
		Exec(ctx)
	return err
}

func (s *packageStoreDB) ListAllWithVersions(ctx context.Context, orgID int64) ([]model.Package, error) {
	var packages []model.Package
	err := s.db.NewSelect().Model(&packages).
		Relation("Versions", func(q *bun.SelectQuery) *bun.SelectQuery {
			// See ListVersions — same sort rationale (newest date first,
			// version_normalized as deterministic tiebreaker).
			return q.Order("created_at DESC", "version_normalized DESC")
		}).
		Where("p.org_id = ?", orgID).
		Order("p.name").
		Scan(ctx)
	return packages, err
}
