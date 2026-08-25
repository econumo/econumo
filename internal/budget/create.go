package budget

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/sortkey"
	"github.com/econumo/econumo/internal/shared/vo"
)

// CreateBudget creates a budget, seeds its category + tag elements, marks it the
// user's active budget, and returns the full built budget.
func (s *Service) CreateBudget(ctx context.Context, userID vo.Id, req model.CreateBudgetRequest) (*model.CreateBudgetResult, error) {
	budgetID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"id": ""})
	}
	if err := model.ValidateName("Budget", req.Name); err != nil {
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

	// Resolve currency: explicit id (checked usable below), else the user's
	// stored default currency id (which every user holds).
	currencyID := req.CurrencyId
	if currencyID == "" {
		stored, cerr := s.users.DefaultCurrencyID(ctx, userID.String())
		if cerr != nil {
			return nil, cerr
		}
		currencyID = stored
	}
	curID, err := vo.ParseId(currencyID)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"currencyId": ""})
	}
	if req.CurrencyId != "" {
		if eerr := s.currency.EnsureUsable(ctx, userID.String(), curID.String()); eerr != nil {
			return nil, eerr
		}
	}

	// A budget without member accounts tracks nothing; refuse before anything is
	// written so a rejected create leaves no budget row behind.
	if len(req.AccountIds) == 0 {
		return nil, errs.NewValidation("Validation failed", errs.FieldError{
			Key: "accountIds", Message: "Select at least one account", Code: errs.CodeBudgetAccountsRequired,
		})
	}

	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		budget := model.NewBudget(budgetID, userID, req.Name, curID, startDate, now)
		if serr := s.budgets.Save(txCtx, budget); serr != nil {
			return serr
		}
		for _, raw := range req.AccountIds {
			aid, perr := vo.ParseId(raw)
			if perr != nil {
				return model.ValidateBlank(map[string]string{"accountIds": ""})
			}
			// ownsAccount reports a missing account as "not owned", so it must be
			// consulted before AccountsByIDs, which errors on an unknown id.
			owned, oerr := s.ownsAccount(txCtx, userID, aid)
			if oerr != nil {
				return oerr
			}
			if !owned {
				return model.ValidateBlank(map[string]string{"accountIds": ""})
			}
			views, verr := s.accounts.AccountsByIDs(txCtx, []vo.Id{aid})
			if verr != nil {
				return verr
			}
			if views[0].IsDeleted {
				return model.ValidateBlank(map[string]string{"accountIds": ""})
			}
			if serr := s.budgets.AddAccount(txCtx, budgetID, aid, now); serr != nil {
				return serr
			}
		}
		after, serr := s.seedCategoryElements(txCtx, userID, budgetID, "", now, nil)
		if serr != nil {
			return serr
		}
		if serr := s.seedTagElements(txCtx, userID, budgetID, after, now, nil); serr != nil {
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

// seedCategoryElements creates a budget element for each category of the user —
// expense categories first (type=category), then income categories
// (type=income_category), one continuous key chain. Archived categories get the
// unset key. Ids in skip already have an element in the budget (accept after an
// earlier membership) and are left untouched. Returns the key the next element
// follows.
func (s *Service) seedCategoryElements(ctx context.Context, userID, budgetID vo.Id, after sortkey.Key, now time.Time, skip map[vo.Id]bool) (sortkey.Key, error) {
	cats, err := s.metadata.CategoriesByOwners(ctx, []vo.Id{userID})
	if err != nil {
		return after, err
	}
	for _, wantIncome := range []bool{false, true} {
		for _, c := range cats {
			if c.IsIncome != wantIncome {
				continue
			}
			extID, perr := vo.ParseId(c.ID)
			if perr != nil {
				return after, perr
			}
			if skip[extID] {
				continue
			}
			typ := model.ElementCategory
			if c.IsIncome {
				typ = model.ElementIncomeCategory
			}
			key := sortkey.Key("")
			if !c.IsArchived {
				next, kerr := nextSeedKey(after)
				if kerr != nil {
					return after, kerr
				}
				key, after = next, next
			}
			el := model.NewBudgetElement(s.elements.NextIdentity(), budgetID, extID, typ, nil, nil, now)
			el.SetSortKey(key)
			if serr := s.elements.SaveElement(ctx, el); serr != nil {
				return after, serr
			}
		}
	}
	return after, nil
}

// nextSeedKey appends after the given key, seeding the group when it is empty.
func nextSeedKey(after sortkey.Key) (sortkey.Key, error) {
	if after == "" {
		return sortkey.Seed(sortkey.GrowsDown), nil
	}
	return sortkey.Between(after, "")
}

// seedTagElements creates a budget element for each tag of the user (archived ->
// unset key). Ids in skip are left untouched, as in seedCategoryElements.
func (s *Service) seedTagElements(ctx context.Context, userID, budgetID vo.Id, after sortkey.Key, now time.Time, skip map[vo.Id]bool) error {
	tags, err := s.metadata.TagsByOwners(ctx, []vo.Id{userID})
	if err != nil {
		return err
	}
	for _, t := range tags {
		extID, perr := vo.ParseId(t.ID)
		if perr != nil {
			return perr
		}
		if skip[extID] {
			continue
		}
		key := sortkey.Key("")
		if !t.IsArchived {
			next, kerr := nextSeedKey(after)
			if kerr != nil {
				return kerr
			}
			key, after = next, next
		}
		el := model.NewBudgetElement(s.elements.NextIdentity(), budgetID, extID, model.ElementTag, nil, nil, now)
		el.SetSortKey(key)
		if serr := s.elements.SaveElement(ctx, el); serr != nil {
			return serr
		}
	}
	return nil
}
