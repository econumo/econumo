package admin

import (
	"context"
	"encoding/json"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/reqctx"
)

type deletedDetails struct {
	Reason         string `json:"reason"`
	LifetimeAccess bool   `json:"lifetimeAccess"`
}

// DeletedUsers fails the page on unreadable details rather than skipping the
// row: a skipped row would let the poller's cursor move past a deletion.
func (s *Service) DeletedUsers(ctx context.Context, since time.Time, afterID string, limit int) ([]model.AdminDeletedUser, error) {
	rows, err := s.deleted.ListByEvent(ctx, model.AuditDeleted, since, afterID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]model.AdminDeletedUser, 0, len(rows))
	for _, r := range rows {
		var d deletedDetails
		if err := json.Unmarshal([]byte(r.Details), &d); err != nil {
			return nil, err
		}
		out = append(out, model.AdminDeletedUser{
			Id:             r.ID.String(),
			UserId:         r.UserID.String(),
			DeletedAt:      r.CreatedAt.UTC().Format(datetime.Layout),
			Reason:         d.Reason,
			LifetimeAccess: d.LifetimeAccess,
		})
	}
	reqctx.AddLogAttr(ctx, "deleted_users", len(out))
	return out, nil
}
