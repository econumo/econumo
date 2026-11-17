package repo

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
)

// ListAffiliateUsers returns users with a recorded affiliate code whose option
// was created at or after since, oldest first.
func (r *Repo) ListAffiliateUsers(ctx context.Context, since time.Time, limit int) ([]model.AdminAffiliateUser, error) {
	rows, err := r.q.ListAffiliateUsers(ctx, r.db(ctx), since.UTC(), limit)
	if err != nil {
		return nil, err
	}
	out := make([]model.AdminAffiliateUser, 0, len(rows))
	for _, row := range rows {
		out = append(out, model.AdminAffiliateUser{
			UserId:    row.UserID,
			Affiliate: *row.Value,
			CreatedAt: row.CreatedAt.UTC().Format(datetime.Layout),
		})
	}
	return out, nil
}
