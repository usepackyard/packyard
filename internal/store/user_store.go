package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/uptrace/bun"

	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/pid"
)

type userStoreDB struct {
	db *bun.DB
}

func NewUserStoreDB(db *bun.DB) UserStore {
	return &userStoreDB{db: db}
}

func (s *userStoreDB) GetByID(ctx context.Context, id int64) (*model.User, error) {
	u := new(model.User)
	err := s.db.NewSelect().Model(u).Where("id = ?", id).Scan(ctx)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (s *userStoreDB) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	u := new(model.User)
	err := s.db.NewSelect().Model(u).Where("email = ?", email).Scan(ctx)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (s *userStoreDB) List(ctx context.Context) ([]model.User, error) {
	var users []model.User
	err := s.db.NewSelect().Model(&users).Order("created_at").Scan(ctx)
	return users, err
}

func (s *userStoreDB) ListSuperAdmins(ctx context.Context) ([]model.User, error) {
	var users []model.User
	err := s.db.NewSelect().Model(&users).
		Where("is_super_admin = ?", true).
		Order("created_at").
		Scan(ctx)
	return users, err
}

func (s *userStoreDB) Create(ctx context.Context, u *model.User) error {
	now := time.Now()
	u.CreatedAt = now
	u.UpdatedAt = now
	u.IsActive = true
	if u.PublicID == "" {
		u.PublicID = pid.New(pid.User)
	}
	_, err := s.db.NewInsert().Model(u).Returning("id").Exec(ctx)
	return err
}

func (s *userStoreDB) GetByPublicID(ctx context.Context, publicID string) (*model.User, error) {
	u := new(model.User)
	err := s.db.NewSelect().Model(u).Where("public_id = ?", publicID).Scan(ctx)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (s *userStoreDB) Update(ctx context.Context, u *model.User) error {
	u.UpdatedAt = time.Now()
	_, err := s.db.NewUpdate().Model(u).
		Column("email", "password", "name", "language", "is_active", "is_super_admin", "updated_at").
		Where("id = ?", u.ID).
		Exec(ctx)
	return err
}

func (s *userStoreDB) Delete(ctx context.Context, id int64) error {
	_, err := s.db.NewDelete().Model((*model.User)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

func (s *userStoreDB) Count(ctx context.Context) (int64, error) {
	count, err := s.db.NewSelect().Model((*model.User)(nil)).Count(ctx)
	return int64(count), err
}
