package budget

import (
	"context"
	"errors"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// UpdateBudget updates a budget's name/currency/member-accounts and returns its
// meta. Requires read access; a name change additionally requires update access.
func (s *Service) UpdateBudget(ctx context.Context, userID vo.Id, req model.UpdateBudgetRequest) (*model.UpdateBudgetResult, error) {
	budgetID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"id": ""})
	}
	if err := model.ValidateName("Budget", req.Name); err != nil {
		return nil, err
	}
	curID, err := vo.ParseId(req.CurrencyId)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"currencyId": ""})
	}

	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	// Any field change (name, currency, member accounts) requires edit rights;
	// a read-only guest must not alter budget metadata.
	if !s.canUpdate(b, userID) {
		return nil, accessDenied()
	}
	if aerr := s.requireNotArchived(b); aerr != nil {
		return nil, aerr
	}
	if !curID.Equal(b.budget.CurrencyID) {
		if eerr := s.currency.EnsureUsable(ctx, userID.String(), curID.String()); eerr != nil {
			return nil, eerr
		}
	}

	now := s.clock.Now()
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		b.budget.UpdateName(req.Name, now)
		b.budget.UpdateCurrency(curID, now)
		if req.EndDate != nil {
			if *req.EndDate == "" {
				if eerr := b.budget.EndAt(nil, now); eerr != nil {
					return eerr
				}
			} else {
				d, perr := time.Parse(datetime.DateLayout, *req.EndDate)
				if perr != nil {
					return model.ValidateBlank(map[string]string{"endDate": ""})
				}
				if eerr := b.budget.EndAt(&d, now); eerr != nil {
					if errors.Is(eerr, model.ErrBudgetEndBeforeStart) {
						return errs.NewValidation("Validation failed", errs.FieldError{
							Key: "endDate", Message: "The end month is before the budget start",
							Code: errs.CodeBudgetEndBeforeStart,
						})
					}
					return eerr
				}
			}
		}
		if serr := s.budgets.Save(txCtx, b.budget); serr != nil {
			return serr
		}
		// accountIds absent → membership untouched (older clients, MCP). Present →
		// replace-set over the caller's OWN accounts: add missing, remove absent
		// ones — but a member with closed-month history is permanent, so naming
		// a set that drops one fails the whole update.
		if req.AccountIds != nil {
			want := map[string]bool{}
			for _, raw := range req.AccountIds {
				aid, perr := vo.ParseId(raw)
				if perr != nil {
					return model.ValidateBlank(map[string]string{"accountIds": ""})
				}
				owned, oerr := s.ownsAccount(txCtx, userID, aid)
				if oerr != nil {
					return oerr
				}
				if !owned {
					continue
				}
				want[aid.String()] = true
			}
			var ownMembers []vo.Id
			for _, m := range b.accounts {
				owned, oerr := s.ownsAccount(txCtx, userID, m.AccountID)
				if oerr != nil {
					return oerr
				}
				if owned {
					ownMembers = append(ownMembers, m.AccountID)
				}
			}
			removable, rerr := s.removableAccounts(txCtx, b, ownMembers, now)
			if rerr != nil {
				return rerr
			}
			for _, m := range ownMembers {
				if want[m.String()] {
					continue
				}
				if !removable[m.String()] {
					return accountNotRemovable()
				}
				if serr := s.budgets.RemoveAccount(txCtx, budgetID, m); serr != nil {
					return serr
				}
			}
			for idStr := range want {
				aid, perr := vo.ParseId(idStr)
				if perr != nil {
					return perr
				}
				// Naming an existing member again is a no-op. Deleted members stay
				// listed in the filters block (they keep counting), so a client
				// round-tripping that list back names them — rejecting the id would
				// wedge every later update, since the removal rule keeps such a
				// member forever. Only a NEW member has to be a live account.
				if b.hasAccount(aid) {
					continue
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
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	b, err = s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	meta, err := s.buildMeta(ctx, b)
	if err != nil {
		return nil, err
	}
	return &model.UpdateBudgetResult{Item: meta}, nil
}

// DeleteBudget deletes a budget (owner|admin). Children cascade via FKs.
func (s *Service) DeleteBudget(ctx context.Context, userID vo.Id, req model.DeleteBudgetRequest) (*model.DeleteBudgetResult, error) {
	budgetID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"id": ""})
	}
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	if !s.canDelete(b, userID) {
		return nil, accessDenied()
	}
	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		if derr := s.budgets.Delete(txCtx, budgetID); derr != nil {
			return derr
		}
		// Every participant whose active-budget option points here would keep
		// requesting a budget that now 404s.
		if cerr := s.users.ClearActiveBudget(txCtx, b.budget.UserID, budgetID); cerr != nil {
			return cerr
		}
		for _, a := range b.access {
			if cerr := s.users.ClearActiveBudget(txCtx, a.UserID, budgetID); cerr != nil {
				return cerr
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return &model.DeleteBudgetResult{}, nil
}

// ResetBudget clears all element limits and resets the start month (owner|admin).
func (s *Service) ResetBudget(ctx context.Context, userID vo.Id, req model.ResetBudgetRequest) (*model.ResetBudgetResult, error) {
	budgetID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"id": ""})
	}
	startedAt, err := time.Parse(datetime.Layout, req.StartedAt)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"startedAt": ""})
	}
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	if !s.canReset(b, userID) {
		return nil, accessDenied()
	}
	if aerr := s.requireNotArchived(b); aerr != nil {
		return nil, aerr
	}
	now := s.clock.Now()
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		if serr := s.limits.DeleteLimitsByBudget(txCtx, budgetID); serr != nil {
			return serr
		}
		b.budget.StartFrom(startedAt, now)
		return s.budgets.Save(txCtx, b.budget)
	})
	if err != nil {
		return nil, err
	}
	b, err = s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	meta, err := s.buildMeta(ctx, b)
	if err != nil {
		return nil, err
	}
	return &model.ResetBudgetResult{Item: meta}, nil
}

// canReset = owner|admin (same as canDelete).
func (s *Service) canReset(b *budgetAggregate, userID vo.Id) bool { return s.canDelete(b, userID) }
