package budget

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// MergeElements folds one classification's budget presence into another's,
// across every budget the source appears in — including budgets shared with
// connected users, which is why the scope is the external id alone rather than
// one owner's budgets.
//
// Limits are SUMMED rather than replaced. The caller is re-pointing the
// spending at the same time, so keeping only the target's limit would make
// every past period where both had one read as over budget.
//
// The caller owns the transaction: this runs inside the merge's tx so a later
// failure rolls the budget changes back with it.
func (s *Service) MergeElements(ctx context.Context, oldExternalID, newExternalID vo.Id) error {
	sources, err := s.elements.ListElementsByExternal(ctx, oldExternalID)
	if err != nil {
		return err
	}
	now := s.clock.Now()
	for _, src := range sources {
		target, terr := s.elements.GetElementByExternal(ctx, src.BudgetID, newExternalID)
		if terr != nil {
			if _, ok := errs.AsNotFound(terr); !ok {
				return terr
			}
			// No conflict in this budget: hand the element over in place. That
			// keeps its folder and sort position, so the budget row stays exactly
			// where the user put it and simply becomes the surviving element.
			if rerr := s.elements.RepointElement(ctx, src.ID, newExternalID, now); rerr != nil {
				return rerr
			}
			continue
		}
		if merr := s.mergeElementLimits(ctx, src, target, now); merr != nil {
			return merr
		}
		// The source's own limits cascade off the element.
		if derr := s.elements.DeleteElement(ctx, src.ID); derr != nil {
			return derr
		}
	}
	return nil
}

// mergeElementLimits adds each of src's per-period limits into dst's.
//
// GetLimit normalizes the period with datetime() on SQLite, which is what makes
// this correct across any legacy RFC3339 rows: a raw string match would miss the
// pair and leave a second row for the same month, double-counting the limit.
func (s *Service) mergeElementLimits(ctx context.Context, src, dst *model.BudgetElement, now time.Time) error {
	limits, err := s.limits.ListLimitsByElement(ctx, src.ID)
	if err != nil {
		return err
	}
	for _, l := range limits {
		existing, gerr := s.limits.GetLimit(ctx, dst.ID, l.Period)
		if gerr != nil {
			if _, ok := errs.AsNotFound(gerr); !ok {
				return gerr
			}
			if serr := s.limits.SaveLimit(ctx, &model.BudgetElementLimit{
				ID:        s.limits.NextIdentity(),
				ElementID: dst.ID,
				Period:    l.Period,
				Amount:    l.Amount,
				CreatedAt: now,
				UpdatedAt: now,
			}); serr != nil {
				return serr
			}
			continue
		}
		existing.Amount = existing.Amount.Add(l.Amount)
		existing.UpdatedAt = now
		if serr := s.limits.SaveLimit(ctx, existing); serr != nil {
			return serr
		}
	}
	return nil
}
