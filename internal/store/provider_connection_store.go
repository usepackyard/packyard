package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/uptrace/bun"

	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/pid"
)

type providerConnectionStoreDB struct {
	db *bun.DB
}

func NewProviderConnectionStoreDB(db *bun.DB) ProviderConnectionStore {
	return &providerConnectionStoreDB{db: db}
}

func (s *providerConnectionStoreDB) List(ctx context.Context, orgID int64) ([]model.ProviderConnection, error) {
	var conns []model.ProviderConnection
	err := s.db.NewSelect().Model(&conns).
		Where("org_id = ?", orgID).
		Order("provider", "name").
		Scan(ctx)
	return conns, err
}

func (s *providerConnectionStoreDB) GetByID(ctx context.Context, orgID, id int64) (*model.ProviderConnection, error) {
	conn := new(model.ProviderConnection)
	err := s.db.NewSelect().Model(conn).
		Where("org_id = ?", orgID).
		Where("id = ?", id).
		Scan(ctx)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func (s *providerConnectionStoreDB) GetByPublicID(ctx context.Context, orgID int64, publicID string) (*model.ProviderConnection, error) {
	conn := new(model.ProviderConnection)
	err := s.db.NewSelect().Model(conn).
		Where("org_id = ?", orgID).
		Where("public_id = ?", publicID).
		Scan(ctx)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func (s *providerConnectionStoreDB) Create(ctx context.Context, conn *model.ProviderConnection) error {
	now := time.Now()
	conn.CreatedAt = now
	conn.UpdatedAt = now
	if conn.PublicID == "" {
		conn.PublicID = pid.New(pid.ProviderConnection)
	}
	_, err := s.db.NewInsert().Model(conn).Returning("id").Exec(ctx)
	return err
}

func (s *providerConnectionStoreDB) Update(ctx context.Context, conn *model.ProviderConnection) error {
	conn.UpdatedAt = time.Now()
	_, err := s.db.NewUpdate().Model(conn).
		Column("name", "provider", "auth_type", "secret_encrypted", "token_prefix", "config", "updated_at").
		Where("org_id = ?", conn.OrgID).
		Where("id = ?", conn.ID).
		Exec(ctx)
	return err
}

func (s *providerConnectionStoreDB) Delete(ctx context.Context, orgID, id int64) error {
	_, err := s.db.NewDelete().Model((*model.ProviderConnection)(nil)).
		Where("org_id = ?", orgID).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (s *providerConnectionStoreDB) CountSources(ctx context.Context, id int64) (int64, error) {
	n, err := s.db.NewSelect().Model((*model.PackageSource)(nil)).
		Where("connection_id = ?", id).
		Count(ctx)
	return int64(n), err
}
