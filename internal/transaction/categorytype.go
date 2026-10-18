package transaction

import (
	"context"

	"github.com/econumo/econumo/internal/infra/i18n"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/port"
)

// RemoveMismatchedCategories clears the category from every income
// transaction carrying an expense category and every expense carrying an
// income one, and notes the removal in the transaction's description, in the
// account owner's language, so nothing is lost silently. Rows like these were
// written before checkReferences refused them. A data migration (CLI
// migration:remove-mismatched-categories), so it takes only what it touches.
// Idempotent: a cleared row no longer matches.
func RemoveMismatchedCategories(ctx context.Context, repo Repository, tx port.TxRunner, clk port.Clock) (int, error) {
	n := 0
	err := tx.WithTx(ctx, func(ctx context.Context) error {
		rows, err := repo.ListMismatchedCategories(ctx)
		if err != nil {
			return err
		}
		now := clk.Now()
		for _, m := range rows {
			t, err := repo.GetByID(ctx, m.ID)
			if err != nil {
				return err
			}
			t.CategoryID = nil
			t.Description = model.AppendNote(t.Description, CategoryRemovedNote(m), model.MaxDescriptionLength)
			t.UpdatedAt = now
			if err := repo.Save(ctx, t); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	return n, err
}

// CategoryRemovedNote is the line the category-type migration appends to a
// transaction or template whose category it removed.
func CategoryRemovedNote(m model.MismatchedCategory) string {
	key := "transactions.category_removed.expense"
	if m.CategoryIncome {
		key = "transactions.category_removed.income"
	}
	return i18n.T(m.OwnerLanguage, key, map[string]any{"name": m.CategoryName})
}
