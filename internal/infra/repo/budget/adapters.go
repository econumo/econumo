// Adapters wiring the budget service's MetadataLookup cross-module port to the
// existing category / tag / payee repositories. Lives here (infra) so
// app/budget depends only on its own small interfaces. The UserLookup,
// AccountLookup, and category/tag-metadata counterparts live in
// internal/server: they need the account/category/tag features' types, which
// an infra package must not import — CategoriesByOwners/TagsByOwners delegate
// to categoryMetadataLookup/tagMetadataLookup, ports expressed purely in
// appbudget.CategoryMeta/TagMeta, so this file itself never imports
// category or tag.
//
// The multi-owner reads (payees for the budget participants) loop the
// existing single-owner repo queries: a budget's participant set is tiny
// (owner + a few accepted users), so N small queries are fine and avoid a new
// dynamic-IN query surface.
package budgetrepo

import (
	"context"

	appbudget "github.com/econumo/econumo/internal/app/budget"
	dompayee "github.com/econumo/econumo/internal/domain/payee"
	"github.com/econumo/econumo/internal/shared/vo"
)

type categoryMetadataLookup interface {
	CategoriesByOwners(ctx context.Context, userIDs []vo.Id) ([]appbudget.CategoryMeta, error)
}
type tagMetadataLookup interface {
	TagsByOwners(ctx context.Context, userIDs []vo.Id) ([]appbudget.TagMeta, error)
}
type payeeRepo interface {
	ListByOwner(ctx context.Context, userID vo.Id) ([]*dompayee.Payee, error)
}

// MetadataLookup adapts the category + tag + payee repositories to
// app/budget.MetadataLookup.
type MetadataLookup struct {
	categories categoryMetadataLookup
	tags       tagMetadataLookup
	payees     payeeRepo
}

var _ appbudget.MetadataLookup = (*MetadataLookup)(nil)

// NewMetadataLookup wraps the category + tag metadata lookups + the payee
// repository. categories is typically server.BudgetCategoryMetadataLookup and
// tags is typically server.BudgetTagMetadataLookup.
func NewMetadataLookup(categories categoryMetadataLookup, tags tagMetadataLookup, payees payeeRepo) *MetadataLookup {
	return &MetadataLookup{categories: categories, tags: tags, payees: payees}
}

// PayeesByOwners returns all payees owned by the given users.
func (l *MetadataLookup) PayeesByOwners(ctx context.Context, userIDs []vo.Id) ([]appbudget.PayeeMeta, error) {
	var out []appbudget.PayeeMeta
	seen := map[string]bool{}
	for _, uid := range userIDs {
		payees, err := l.payees.ListByOwner(ctx, uid)
		if err != nil {
			return nil, err
		}
		for _, p := range payees {
			if seen[p.Id().String()] {
				continue
			}
			seen[p.Id().String()] = true
			out = append(out, appbudget.PayeeMeta{ID: p.Id().String(), Name: p.Name()})
		}
	}
	return out, nil
}

// CategoriesByOwners returns all categories owned by the given users.
func (l *MetadataLookup) CategoriesByOwners(ctx context.Context, userIDs []vo.Id) ([]appbudget.CategoryMeta, error) {
	return l.categories.CategoriesByOwners(ctx, userIDs)
}

// TagsByOwners returns all tags owned by the given users.
func (l *MetadataLookup) TagsByOwners(ctx context.Context, userIDs []vo.Id) ([]appbudget.TagMeta, error) {
	return l.tags.TagsByOwners(ctx, userIDs)
}
