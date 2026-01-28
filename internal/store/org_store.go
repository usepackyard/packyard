package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/uptrace/bun"

	"github.com/usepackyard/packyard/internal/model"
)

type orgStoreDB struct {
	db *bun.DB
}

func NewOrgStoreDB(db *bun.DB) OrgStore {
	return &orgStoreDB{db: db}
}

func (s *orgStoreDB) GetByID(ctx context.Context, id int64) (*model.Organization, error) {
	o := new(model.Organization)
	err := s.db.NewSelect().Model(o).Where("id = ?", id).Scan(ctx)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return o, nil
}

func (s *orgStoreDB) GetBySlug(ctx context.Context, slug string) (*model.Organization, error) {
	o := new(model.Organization)
	err := s.db.NewSelect().Model(o).Where("slug = ?", slug).Scan(ctx)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return o, nil
}

func (s *orgStoreDB) List(ctx context.Context) ([]model.Organization, error) {
	var orgs []model.Organization
	err := s.db.NewSelect().Model(&orgs).Order("name").Scan(ctx)
	return orgs, err
}

func (s *orgStoreDB) Create(ctx context.Context, org *model.Organization) error {
	now := time.Now()
	org.CreatedAt = now
	org.UpdatedAt = now
	if org.Status == "" {
		org.Status = model.OrgStatusActive
	}
	_, err := s.db.NewInsert().Model(org).Returning("id").Exec(ctx)
	return err
}

func (s *orgStoreDB) Update(ctx context.Context, org *model.Organization) error {
	org.UpdatedAt = time.Now()
	_, err := s.db.NewUpdate().Model(org).
		Column("slug", "name", "updated_at").
		Where("id = ?", org.ID).
		Exec(ctx)
	return err
}

func (s *orgStoreDB) UpdateStatus(ctx context.Context, id int64, status string) error {
	_, err := s.db.NewUpdate().Model((*model.Organization)(nil)).
		Set("status = ?", status).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (s *orgStoreDB) Delete(ctx context.Context, id int64) error {
	_, err := s.db.NewDelete().Model((*model.Organization)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

func (s *orgStoreDB) ListMembers(ctx context.Context, orgID int64) ([]model.OrgMember, error) {
	var members []model.OrgMember
	err := s.db.NewSelect().Model(&members).
		Relation("User").
		Where("m.org_id = ?", orgID).
		Order("m.created_at").
		Scan(ctx)
	return members, err
}

func (s *orgStoreDB) GetMember(ctx context.Context, orgID, userID int64) (*model.OrgMember, error) {
	m := new(model.OrgMember)
	err := s.db.NewSelect().Model(m).
		Where("org_id = ?", orgID).
		Where("user_id = ?", userID).
		Scan(ctx)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (s *orgStoreDB) AddMember(ctx context.Context, m *model.OrgMember) error {
	m.CreatedAt = time.Now()
	if m.Permissions == nil {
		m.Permissions = model.JSONStringSlice{}
	}
	_, err := s.db.NewInsert().Model(m).Returning("id").Exec(ctx)
	return err
}

func (s *orgStoreDB) UpdateMember(ctx context.Context, m *model.OrgMember) error {
	if m.Permissions == nil {
		m.Permissions = model.JSONStringSlice{}
	}
	_, err := s.db.NewUpdate().Model(m).
		Column("role", "permissions").
		Where("org_id = ?", m.OrgID).
		Where("user_id = ?", m.UserID).
		Exec(ctx)
	return err
}

func (s *orgStoreDB) RemoveMember(ctx context.Context, orgID, userID int64) error {
	_, err := s.db.NewDelete().Model((*model.OrgMember)(nil)).
		Where("org_id = ?", orgID).
		Where("user_id = ?", userID).
		Exec(ctx)
	return err
}

func (s *orgStoreDB) ListUserOrgs(ctx context.Context, userID int64) ([]model.Organization, error) {
	var orgs []model.Organization
	err := s.db.NewSelect().Model(&orgs).
		Join("JOIN org_members AS m ON m.org_id = o.id").
		Where("m.user_id = ?", userID).
		Order("o.name").
		Scan(ctx)
	return orgs, err
}
