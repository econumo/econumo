package admin

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/port"
	"github.com/econumo/econumo/internal/shared/vo"
)

type Service struct {
	users      UserLookup
	conns      ConnectionLookup
	affiliates AffiliateLister
	deleted    DeletedUserLister
	deletions  DeletionStateReader
	clock      port.Clock
}

func NewService(users UserLookup, conns ConnectionLookup, affiliates AffiliateLister, deleted DeletedUserLister, deletions DeletionStateReader, clk port.Clock) *Service {
	return &Service{users: users, conns: conns, affiliates: affiliates, deleted: deleted, deletions: deletions, clock: clk}
}

// view reads the deletion mark separately from the record; a NotFound means
// the row was wiped after the record was read.
func (s *Service) view(ctx context.Context, r UserRecord, id vo.Id) (model.AdminUserView, error) {
	st, err := s.deletions.GetState(ctx, id)
	if err != nil {
		return model.AdminUserView{}, err
	}
	v := model.AdminUserView{
		Id:                   r.ID,
		Name:                 r.Name,
		Email:                r.Email,
		Avatar:               r.Avatar,
		AccessLevel:          string(r.AccessLevel),
		AccessUntil:          formatUntil(r.AccessUntil),
		Affiliate:            r.Affiliate,
		AffiliateCode:        r.AffiliateCode,
		EffectiveAccessLevel: string(model.EffectiveAccessLevel(r.AccessLevel, r.AccessUntil, s.clock.Now())),
		DeletionMarkedAt:     formatUntil(st.MarkedAt),
	}
	if st.MarkedAt != nil {
		v.DeletionReason = string(st.Reason)
	}
	return v, nil
}

// formatUntil renders a nullable expiry as the wire/log value: the frozen
// datetime layout, or "" for no expiry.
func formatUntil(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(datetime.Layout)
}
