package budget

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

// CreateBudget creates a budget, seeds its category + tag elements, marks it the
// user's active budget, and returns the full built budget.
func (s *Service) CreateBudget(ctx context.Context, userID vo.Id, req model.CreateBudgetRequest) (*model.CreateBudgetResult, error) {
	budgetID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"id": ""})
	}
	if err := model.ValidateName("model.Budget", req.Name); err != nil {
		return nil, err
	}

	now := s.clock.Now()
	currentMonth := localMonth(now, reqctx.Location(ctx))
	startDate := currentMonth
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
		return nil, model.ValidateBlank(map[string]string{"currencyId": ""})
	}

	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		budget := model.NewBudget(budgetID, userID, req.Name, curID, startDate, now)
		if serr := s.budgets.Save(txCtx, budget); serr != nil {
			return serr
		}
		for _, raw := range req.ExcludedAccounts {
			aid, perr := vo.ParseId(raw)
			if perr != nil {
				return model.ValidateBlank(map[string]string{"excludedAccounts": ""})
			}
			if serr := s.budgets.ExcludeAccount(txCtx, budgetID, aid); serr != nil {
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
	result, err := s.BuildBudget(ctx, userID, b, currentMonth, now)
	if err != nil {
		return nil, err
	}
	return &model.CreateBudgetResult{Item: result}, nil
}

// seedCategoryElements creates a budget element for each non-income category of
// the user; archived categories get the unset position, others get an
// incrementing position. Returns the next free position.
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
		position := model.PositionUnset
		if !c.IsArchived {
			position = pos
			pos++
		}
		extID, perr := vo.ParseId(c.ID)
		if perr != nil {
			return pos, perr
		}
		el := model.NewBudgetElement(s.elements.NextIdentity(), budgetID, extID, model.ElementCategory, nil, nil, int16(position), now)
		if serr := s.elements.SaveElement(ctx, el); serr != nil {
			return pos, serr
		}
	}
	return pos, nil
}

// seedTagElements creates a budget element for each tag of the user (archived ->
// unset position).
func (s *Service) seedTagElements(ctx context.Context, userID, budgetID vo.Id, startPos int, now time.Time) error {
	tags, err := s.metadata.TagsByOwners(ctx, []vo.Id{userID})
	if err != nil {
		return err
	}
	pos := startPos
	for _, t := range tags {
		position := model.PositionUnset
		if !t.IsArchived {
			position = pos
			pos++
		}
		extID, perr := vo.ParseId(t.ID)
		if perr != nil {
			return perr
		}
		el := model.NewBudgetElement(s.elements.NextIdentity(), budgetID, extID, model.ElementTag, nil, nil, int16(position), now)
		if serr := s.elements.SaveElement(ctx, el); serr != nil {
			return serr
		}
	}
	return nil
}
