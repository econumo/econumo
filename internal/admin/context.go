package admin

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

// UserContext keeps the connection graph — and the authorization rules behind
// it — in the product, so the portal never duplicates "who may see whom".
func (s *Service) UserContext(ctx context.Context, userID vo.Id) (*model.AdminUserContextResult, error) {
	reqctx.AddLogAttr(ctx, "user_id", userID.String())
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
			// A dangling connection (its user row gone — possible once account
			// deletion ships) must not abort the target user's context: the
			// portal reads an error here as "no such user, stop retrying" and
			// would refuse a valid purchase over an unrelated row.
			if _, ok := errs.AsNotFound(cerr); ok {
				continue
			}
			return nil, cerr
		}
		conns = append(conns, s.view(rec))
	}
	// How many connected users' data left the product on this call.
	reqctx.AddLogAttr(ctx, "connections", len(conns))
	return &model.AdminUserContextResult{User: s.view(self), Connections: conns}, nil
}
