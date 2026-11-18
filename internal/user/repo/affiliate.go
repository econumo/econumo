package repo

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
)

// nilUserID sorts below every user id on both engines (the pgsql column is UUID,
// so an empty string would not even parse).
const nilUserID = "00000000-0000-0000-0000-000000000000"

// ListAffiliateUsers returns users attributed to program (every program when
// it is empty) whose affiliate option
// was created at or after since, oldest first. A non-empty afterUserID turns
// since into an exclusive cursor: only rows after (since, afterUserID) return.
func (r *Repo) ListAffiliateUsers(ctx context.Context, program string, since time.Time, afterUserID string, limit int) ([]model.AdminAffiliateUser, error) {
	if afterUserID == "" {
		afterUserID = nilUserID
	}
	rows, err := r.q.ListAffiliateUsers(ctx, r.db(ctx), program, since.UTC(), afterUserID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]model.AdminAffiliateUser, 0, len(rows))
	for _, row := range rows {
		code := ""
		if row.Code != nil {
			code = *row.Code
		}
		out = append(out, model.AdminAffiliateUser{
			UserId:        row.UserID,
			Affiliate:     *row.Value,
			AffiliateCode: code,
			CreatedAt:     row.CreatedAt.UTC().Format(datetime.Layout),
		})
	}
	return out, nil
}
