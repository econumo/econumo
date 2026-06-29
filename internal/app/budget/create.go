package budget

import (
	"context"
	"time"

	dombudget "github.com/econumo/econumo/internal/domain/budget"
	"github.com/econumo/econumo/internal/domain/shared/datetime"
	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// CreateBudget creates a budget, seeds its category + tag elements, marks it the
// user's active budget, and returns the full built budget. Mirrors
// BudgetService.createBudget + BudgetElementService.create{Categories,Tags}Elements.
func (s *Service) CreateBudget(ctx context.Context, userID vo.Id, req CreateBudgetRequest) (*CreateBudgetResult, error) {
	budgetID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, validateBlank(map[string]string{"id": ""})
	}
	if err := dombudget.ValidateName("Budget", req.Name); err != nil {
		return nil, err
	}

	now := s.clock.Now()
	startDate := now
	if req.StartDate != "" {
		if t, perr := time.Parse(datetime.DateLayout, req.StartDate); perr == nil {
			startDate = t
		}
	}

	// Resolve currency: explicit id, else the user's default currency code.
	currencyID := req.CurrencyId
	if currencyID == "" {
		code, cerr := s.users.CurrencyCode(ctx, userID.String())
		if cerr != nil {
			return nil, cerr
		}
		id, cerr := s.currency.GetIDByCode(ctx, code)
		if cerr != nil {
			return nil, cerr
		}
		currencyID = id
	}
	curID, err := vo.ParseId(currencyID)
	if err != nil {
		return nil, validateBlank(map[string]string{"currencyId": ""})
	}

	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		budget := dombudget.NewBudget(budgetID, userID, req.Name, curID, startDate, now)
		if serr := s.repo.Save(txCtx, budget); serr != nil {
			return serr
		}
		for _, raw := range req.ExcludedAccounts {
			aid, perr := vo.ParseId(raw)
			if perr != nil {
				return validateBlank(map[string]string{"excludedAccounts": ""})
			}
			if serr := s.repo.ExcludeAccount(txCtx, budgetID, aid); serr != nil {
				return serr
			}
		}
		pos, serr := s.seedCategoryElements(txCtx, userID, budgetID, 0, now)
		if serr != nil {
			return serr
		}
		if serr := s.seedTagElements(txCtx, userID, budgetID, pos, now); serr != nil {
			return serr
		}
		return s.users.SetActiveBudget(txCtx, userID, budgetID)
	})
	if err != nil {
		return nil, err
	}

	// Build the result from the freshly created budget at the current month.
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	result, err := s.BuildBudget(ctx, userID, b, firstOfMonth(now), now)
	if err != nil {
		return nil, err
	}
	return &CreateBudgetResult{Item: result}, nil
}

// seedCategoryElements creates a budget element for each NON-INCOME category of
// the user; archived categories get POSITION_UNSET, others get an incrementing
// position. Returns the next free position.
func (s *Service) seedCategoryElements(ctx context.Context, userID, budgetID vo.Id, startPos int, now time.Time) (int, error) {
	cats, err := s.metadata.CategoriesByOwners(ctx, []vo.Id{userID})
	if err != nil {
		return startPos, err
	}
	pos := startPos
	for _, c := range cats {
		if c.IsIncome {
			continue
		}
		position := dombudget.PositionUnset
		if !c.IsArchived {
			position = pos
			pos++
		}
		extID, perr := vo.ParseId(c.ID)
		if perr != nil {
			return pos, perr
		}
		el := dombudget.NewBudgetElement(s.repo.NextIdentity(), budgetID, extID, dombudget.ElementCategory, nil, nil, int16(position), now)
		if serr := s.repo.SaveElement(ctx, el); serr != nil {
			return pos, serr
		}
	}
	return pos, nil
}

// seedTagElements creates a budget element for each tag of the user (archived ->
// POSITION_UNSET).
func (s *Service) seedTagElements(ctx context.Context, userID, budgetID vo.Id, startPos int, now time.Time) error {
	tags, err := s.metadata.TagsByOwners(ctx, []vo.Id{userID})
	if err != nil {
		return err
	}
	pos := startPos
	for _, t := range tags {
		position := dombudget.PositionUnset
		if !t.IsArchived {
			position = pos
			pos++
		}
		extID, perr := vo.ParseId(t.ID)
		if perr != nil {
			return perr
		}
		el := dombudget.NewBudgetElement(s.repo.NextIdentity(), budgetID, extID, dombudget.ElementTag, nil, nil, int16(position), now)
		if serr := s.repo.SaveElement(ctx, el); serr != nil {
			return serr
		}
	}
	return nil
}
