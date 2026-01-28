package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/uptrace/bun"

	"github.com/usepackyard/packyard/internal/model"
)

type sessionStoreDB struct {
	db *bun.DB
}

func NewSessionStoreDB(db *bun.DB) SessionStore {
	return &sessionStoreDB{db: db}
}

func (s *sessionStoreDB) Create(ctx context.Context, sess *model.Session) error {
	sess.CreatedAt = time.Now()
	_, err := s.db.NewInsert().Model(sess).Exec(ctx)
	return err
}

func (s *sessionStoreDB) GetByID(ctx context.Context, id string) (*model.Session, error) {
	sess := new(model.Session)
	err := s.db.NewSelect().Model(sess).Where("id = ?", id).Scan(ctx)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return sess, nil
}

func (s *sessionStoreDB) Delete(ctx context.Context, id string) error {
	_, err := s.db.NewDelete().Model((*model.Session)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

func (s *sessionStoreDB) DeleteExpired(ctx context.Context) error {
	_, err := s.db.NewDelete().Model((*model.Session)(nil)).Where("expires_at < ?", time.Now()).Exec(ctx)
	return err
}

func (s *sessionStoreDB) DeleteByUserID(ctx context.Context, userID int64) error {
	_, err := s.db.NewDelete().Model((*model.Session)(nil)).Where("user_id = ?", userID).Exec(ctx)
	return err
}

func (s *sessionStoreDB) DeleteOthersByUserID(ctx context.Context, userID int64, keepSessionID string) error {
	_, err := s.db.NewDelete().Model((*model.Session)(nil)).
		Where("user_id = ?", userID).
		Where("id != ?", keepSessionID).
		Exec(ctx)
	return err
}
