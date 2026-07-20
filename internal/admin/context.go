package admin

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// UserContext keeps the connection graph — and the authorization rules behind
// it — in the product, so the portal never duplicates "who may see whom".
func (s *Service) UserContext(ctx context.Context, userID vo.Id) (*model.AdminUserContextResult, error) {
	self, err := s.users.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	ids, err := s.conns.ConnectedUserIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	// One lookup per connection: connections are partners (typically 0-3), so a
	// dedicated cross-engine query would buy little over indexed primary-key reads.
	conns := make([]model.AdminUserView, 0, len(ids))
	for _, id := range ids {
		rec, cerr := s.users.GetUser(ctx, id)
		if cerr != nil {
			return nil, cerr
		}
		conns = append(conns, s.view(rec))
	}
	return &model.AdminUserContextResult{User: s.view(self), Connections: conns}, nil
}
