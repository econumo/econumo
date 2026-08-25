// Package wiring builds cross-feature adapters for tests, mirroring what
// internal/server assembles in production. Test harnesses construct feature
// services directly, so without this they would each have to repeat (or stub)
// the same glue.
package wiring

import (
	appbudget "github.com/econumo/econumo/internal/budget"
	budgetrepo "github.com/econumo/econumo/internal/budget/repo"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/shared/port"
)

// BudgetMerger builds the budget-element merger that the category and tag
// services take. Tests get the real implementation rather than a stub, so a
// merge exercises the limit arithmetic and element re-pointing for real.
func BudgetMerger(engine string, txm *backend.TxManager, clk port.Clock) *appbudget.MergeService {
	repo := budgetrepo.NewRepo(engine, txm)
	return appbudget.NewMergeService(repo, repo, clk)
}
