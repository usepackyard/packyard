package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/uptrace/bun"

	"github.com/usepackyard/packyard/internal/model"
)

type ssoTicketStoreDB struct {
	db *bun.DB
}

func NewSSOTicketStoreDB(db *bun.DB) SSOTicketStore {
	return &ssoTicketStoreDB{db: db}
}

func (s *ssoTicketStoreDB) Create(ctx context.Context, t *model.SSOTicket) error {
	now := time.Now()
	t.CreatedAt = now
	_, err := s.db.NewInsert().Model(t).Returning("id").Exec(ctx)
	return err
}

func (s *ssoTicketStoreDB) Consume(ctx context.Context, tokenHash, audience string, now time.Time) (*model.SSOTicket, error) {
	var out *model.SSOTicket
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		ticket := new(model.SSOTicket)
		if err := tx.NewSelect().Model(ticket).Where("token_hash = ?", tokenHash).Scan(ctx); err != nil {
			if err == sql.ErrNoRows {
				return ErrSSOTicketNotFound
			}
			return err
		}
		if ticket.Audience != audience {
			return ErrSSOTicketAudienceInvalid
		}
		if ticket.ConsumedAt != nil {
			return ErrSSOTicketConsumed
		}
		if !ticket.ExpiresAt.After(now) {
			return ErrSSOTicketExpired
		}

		res, err := tx.NewUpdate().Model((*model.SSOTicket)(nil)).
			Set("consumed_at = ?", now).
			Where("id = ?", ticket.ID).
			Where("consumed_at IS NULL").
			Exec(ctx)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrSSOTicketConsumed
		}

		ticket.ConsumedAt = &now
		out = ticket
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
