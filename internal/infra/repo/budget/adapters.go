// Adapters wiring the budget service's MetadataLookup cross-module port to the
// category / tag / payee metadata lookups. Lives here (infra) so app/budget
// depends only on its own small interfaces. The UserLookup, AccountLookup, and
// category/tag/payee-metadata counterparts live in internal/server: they need
// the account/category/tag/payee features' types, which an infra package must
// not import — CategoriesByOwners/TagsByOwners/PayeesByOwners delegate to
// categoryMetadataLookup/tagMetadataLookup/payeeMetadataLookup, ports
// expressed purely in appbudget.CategoryMeta/TagMeta/PayeeMeta, so this file
// itself never imports category, tag, or payee.
package budgetrepo

import (
	"context"

	appbudget "github.com/econumo/econumo/internal/app/budget"
	"github.com/econumo/econumo/internal/shared/vo"
)

type categoryMetadataLookup interface {
	CategoriesByOwners(ctx context.Context, userIDs []vo.Id) ([]appbudget.CategoryMeta, error)
}
type tagMetadataLookup interface {
	TagsByOwners(ctx context.Context, userIDs []vo.Id) ([]appbudget.TagMeta, error)
}
type payeeMetadataLookup interface {
	PayeesByOwners(ctx context.Context, userIDs []vo.Id) ([]appbudget.PayeeMeta, error)
}

// MetadataLookup adapts the category + tag + payee metadata lookups to
// app/budget.MetadataLookup.
type MetadataLookup struct {
	categories categoryMetadataLookup
	tags       tagMetadataLookup
	payees     payeeMetadataLookup
}

var _ appbudget.MetadataLookup = (*MetadataLookup)(nil)

// NewMetadataLookup wraps the category + tag + payee metadata lookups.
// categories is typically server.BudgetCategoryMetadataLookup, tags is
// typically server.BudgetTagMetadataLookup, and payees is typically
// server.BudgetPayeeMetadataLookup.
func NewMetadataLookup(categories categoryMetadataLookup, tags tagMetadataLookup, payees payeeMetadataLookup) *MetadataLookup {
	return &MetadataLookup{categories: categories, tags: tags, payees: payees}
}

// PayeesByOwners returns all payees owned by the given users.
func (l *MetadataLookup) PayeesByOwners(ctx context.Context, userIDs []vo.Id) ([]appbudget.PayeeMeta, error) {
	return l.payees.PayeesByOwners(ctx, userIDs)
}

// CategoriesByOwners returns all categories owned by the given users.
func (l *MetadataLookup) CategoriesByOwners(ctx context.Context, userIDs []vo.Id) ([]appbudget.CategoryMeta, error) {
	return l.categories.CategoriesByOwners(ctx, userIDs)
}

// TagsByOwners returns all tags owned by the given users.
func (l *MetadataLookup) TagsByOwners(ctx context.Context, userIDs []vo.Id) ([]appbudget.TagMeta, error) {
	return l.tags.TagsByOwners(ctx, userIDs)
}
