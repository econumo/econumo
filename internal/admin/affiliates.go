package admin

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
)

func (s *Service) AffiliateUsers(ctx context.Context, since time.Time, afterUserID string, limit int) ([]model.AdminAffiliateUser, error) {
	rows, err := s.affiliates.ListAffiliateUsers(ctx, since, afterUserID, limit)
	if err != nil {
		return nil, err
	}
	reqctx.AddLogAttr(ctx, "affiliate_users", len(rows))
	return rows, nil
}
