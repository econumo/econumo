package server

import (
	"context"

	appbudget "github.com/econumo/econumo/internal/budget"
	appcategory "github.com/econumo/econumo/internal/category"
	"github.com/econumo/econumo/internal/shared/vo"
	apptag "github.com/econumo/econumo/internal/tag"
)

// classificationBudgetMerger adapts the budget merge service to the identical
// ports category and tag each declare. Both features name the same capability,
// so one adapter satisfies both rather than duplicating it per feature.
//
// It is built from budget's MergeService (not its Service) because budget
// already depends on the classification features to resolve element names —
// taking the full Service here would close that loop at construction time.
type classificationBudgetMerger struct{ svc *appbudget.MergeService }

var (
	_ appcategory.BudgetElementMerger = classificationBudgetMerger{}
	_ apptag.BudgetElementMerger      = classificationBudgetMerger{}
)

func (m classificationBudgetMerger) MergeElements(ctx context.Context, oldExternalID, newExternalID vo.Id) error {
	return m.svc.MergeElements(ctx, oldExternalID, newExternalID)
}
