package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/uptrace/bun"

	"github.com/usepackyard/packyard/internal/model"
)

type adminTokenStoreDB struct {
	db *bun.DB
}

func NewAdminTokenStoreDB(db *bun.DB) AdminTokenStore {
	return &adminTokenStoreDB{db: db}
}

func (s *adminTokenStoreDB) List(ctx context.Context) ([]model.AdminToken, error) {
	var tokens []model.AdminToken
	err := s.db.NewSelect().Model(&tokens).Order("created_at DESC").Scan(ctx)
	return tokens, err
}

func (s *adminTokenStoreDB) Create(ctx context.Context, t *model.AdminToken) error {
	t.CreatedAt = time.Now()
	_, err := s.db.NewInsert().Model(t).Returning("id").Exec(ctx)
	return err
}

func (s *adminTokenStoreDB) GetByHash(ctx context.Context, hash string) (*model.AdminToken, error) {
	t := new(model.AdminToken)
	err := s.db.NewSelect().Model(t).Where("token_hash = ?", hash).Scan(ctx)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (s *adminTokenStoreDB) Delete(ctx context.Context, id int64) error {
	_, err := s.db.NewDelete().Model((*model.AdminToken)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

func (s *adminTokenStoreDB) UpdateLastUsed(ctx context.Context, id int64) error {
	now := time.Now()
	_, err := s.db.NewUpdate().Model((*model.AdminToken)(nil)).
		Set("last_used_at = ?", now).
		Where("id = ?", id).
		Exec(ctx)
	return err
}
