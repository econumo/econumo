// Adapters wiring the budget service's MetadataLookup cross-module port to the
// existing category / tag / payee repositories. Lives here (infra) so
// app/budget depends only on its own small interfaces. The UserLookup,
// AccountLookup, and category-metadata counterparts live in internal/server:
// they need the account/category features' types, which an infra package must
// not import — CategoriesByOwners delegates to categoryMetadataLookup, a port
// expressed purely in appbudget.CategoryMeta, so this file itself never
// imports category.
//
// The multi-owner reads (tags/payees for the budget participants) loop the
// existing single-owner repo queries: a budget's participant set is tiny
// (owner + a few accepted users), so N small queries are fine and avoid a new
// dynamic-IN query surface.
package budgetrepo

import (
	"context"

	appbudget "github.com/econumo/econumo/internal/app/budget"
	dompayee "github.com/econumo/econumo/internal/domain/payee"
	domtag "github.com/econumo/econumo/internal/domain/tag"
	"github.com/econumo/econumo/internal/shared/vo"
)

type categoryMetadataLookup interface {
	CategoriesByOwners(ctx context.Context, userIDs []vo.Id) ([]appbudget.CategoryMeta, error)
}
type tagRepo interface {
	ListByOwner(ctx context.Context, userID vo.Id) ([]*domtag.Tag, error)
}
type payeeRepo interface {
	ListByOwner(ctx context.Context, userID vo.Id) ([]*dompayee.Payee, error)
}

// MetadataLookup adapts the category + tag + payee repositories to
// app/budget.MetadataLookup.
type MetadataLookup struct {
	categories categoryMetadataLookup
	tags       tagRepo
	payees     payeeRepo
}

var _ appbudget.MetadataLookup = (*MetadataLookup)(nil)

// NewMetadataLookup wraps the category metadata lookup + tag + payee
// repositories. categories is typically server.BudgetCategoryMetadataLookup.
func NewMetadataLookup(categories categoryMetadataLookup, tags tagRepo, payees payeeRepo) *MetadataLookup {
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
	var out []appbudget.TagMeta
	seen := map[string]bool{}
	for _, uid := range userIDs {
		tags, err := l.tags.ListByOwner(ctx, uid)
		if err != nil {
			return nil, err
		}
		for _, t := range tags {
			if seen[t.Id().String()] {
				continue
			}
			seen[t.Id().String()] = true
			out = append(out, appbudget.TagMeta{
				ID: t.Id().String(), OwnerID: t.UserId().String(), Name: t.Name(), IsArchived: t.IsArchived(),
			})
		}
	}
	return out, nil
}
