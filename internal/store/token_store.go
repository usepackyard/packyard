package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/uptrace/bun"

	"github.com/usepackyard/packyard/internal/model"
)

type tokenStoreDB struct {
	db *bun.DB
}

func NewTokenStoreDB(db *bun.DB) TokenStore {
	return &tokenStoreDB{db: db}
}

func (s *tokenStoreDB) List(ctx context.Context, orgID int64) ([]model.APIToken, error) {
	var tokens []model.APIToken
	err := s.db.NewSelect().Model(&tokens).
		Where("org_id = ?", orgID).
		Order("created_at DESC").
		Scan(ctx)
	return tokens, err
}

func (s *tokenStoreDB) Create(ctx context.Context, t *model.APIToken) error {
	t.CreatedAt = time.Now()
	t.IsActive = true
	_, err := s.db.NewInsert().Model(t).Returning("id").Exec(ctx)
	return err
}

func (s *tokenStoreDB) GetByHash(ctx context.Context, hash string) (*model.APIToken, error) {
	t := new(model.APIToken)
	err := s.db.NewSelect().Model(t).
		Where("token_hash = ?", hash).
		Where("is_active = ?", true).
		Scan(ctx)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (s *tokenStoreDB) Delete(ctx context.Context, orgID, id int64) error {
	_, err := s.db.NewDelete().Model((*model.APIToken)(nil)).
		Where("org_id = ?", orgID).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (s *tokenStoreDB) UpdateLastUsed(ctx context.Context, id int64) error {
	now := time.Now()
	_, err := s.db.NewUpdate().Model((*model.APIToken)(nil)).
		Set("last_used_at = ?", now).
		Where("id = ?", id).
		Exec(ctx)
	return err
}
