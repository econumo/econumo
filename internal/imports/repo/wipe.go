package repo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/shared/vo"
)

// ClearAppliedRefsToUser nulls the applied category, payee, tag and rule of
// other users' import links that point at the user's rows. Those columns have
// no foreign key, so deleting the user would otherwise leave them dangling;
// the user's own links go with the import_sources cascade.
func (r *Repo) ClearAppliedRefsToUser(ctx context.Context, userID vo.Id) error {
	for _, run := range []func(context.Context, backend.DBTX, string) error{
		r.q.ClearImportLinkAppliedCategoryOfUser,
		r.q.ClearImportLinkAppliedPayeeOfUser,
		r.q.ClearImportLinkAppliedTagOfUser,
		r.q.ClearImportLinkAppliedRuleOfUser,
	} {
		if err := run(ctx, r.db(ctx), userID.String()); err != nil {
			return err
		}
	}
	return nil
}
