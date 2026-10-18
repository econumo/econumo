package recurring

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/port"
)

// RemoveMismatchedCategories is the transaction feature's migration of the
// same name, for templates: a template carrying a category of the other kind
// would now fail every post. note renders the appended line (shared with the
// transaction feature, which owns the wording).
func RemoveMismatchedCategories(ctx context.Context, repo Repository, tx port.TxRunner, clk port.Clock, note func(model.MismatchedCategory) string) (int, error) {
	n := 0
	err := tx.WithTx(ctx, func(ctx context.Context) error {
		rows, err := repo.ListMismatchedCategories(ctx)
		if err != nil {
			return err
		}
		now := clk.Now()
		for _, m := range rows {
			rt, err := repo.GetByID(ctx, m.ID)
			if err != nil {
				return err
			}
			rt.CategoryID = nil
			rt.Description = model.AppendNote(rt.Description, note(m), model.MaxDescriptionLength)
			rt.UpdatedAt = now
			if err := repo.Save(ctx, rt); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	return n, err
}
