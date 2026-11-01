package admin

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *Service) UserStatus(ctx context.Context, userID vo.Id) (*model.AdminUserStatusResult, error) {
	reqctx.AddLogAttr(ctx, "user_id", userID.String())
	rec, err := s.users.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	reqctx.AddLogAttr(ctx, "active", rec.IsActive)
	return &model.AdminUserStatusResult{UserID: rec.ID, Active: rec.IsActive}, nil
}
