package server

import (
	"context"
	"errors"

	appbudget "github.com/econumo/econumo/internal/budget"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type budgetRecurringRepo interface {
	ListBySourceOrRecipientIDs(ctx context.Context, accountIDs []vo.Id) ([]*model.RecurringTransaction, error)
	LabelsByRecurringIDs(ctx context.Context, ids []vo.Id) (map[string][]string, error)
}

type budgetRecurringOwners interface {
	AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error)
}

type budgetRecurringGrants interface {
	HasWriteGrant(ctx context.Context, accountID, userID vo.Id) (bool, error)
}

// BudgetRecurringSource adapts the recurring repository to
// budget.RecurringSource.
type BudgetRecurringSource struct {
	repo   budgetRecurringRepo
	owners budgetRecurringOwners
	grants budgetRecurringGrants
}

var _ appbudget.RecurringSource = (*BudgetRecurringSource)(nil)

func NewBudgetRecurringSource(repo budgetRecurringRepo, owners budgetRecurringOwners, grants budgetRecurringGrants) *BudgetRecurringSource {
	return &BudgetRecurringSource{repo: repo, owners: owners, grants: grants}
}

// ListByAccountIDs loads the labels too: the repository's list leaves them
// unhydrated.
func (s *BudgetRecurringSource) ListByAccountIDs(ctx context.Context, accountIDs []vo.Id) ([]*model.RecurringTransaction, error) {
	items, err := s.repo.ListBySourceOrRecipientIDs(ctx, accountIDs)
	if err != nil || len(items) == 0 {
		return items, err
	}
	ids := make([]vo.Id, len(items))
	for i, rt := range items {
		ids[i] = rt.ID
	}
	labels, err := s.repo.LabelsByRecurringIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, rt := range items {
		for _, l := range labels[rt.ID.String()] {
			id, perr := vo.ParseId(l)
			if perr != nil {
				return nil, perr
			}
			rt.LabelIDs = append(rt.LabelIDs, id)
		}
	}
	return items, nil
}

// CanEdit is the recurring feature's write-access rule: the account owner, or
// a write grant (admin/user, never guest) on the account. A missing account
// means no one may act on the template, as the recurring writes refuse it.
func (s *BudgetRecurringSource) CanEdit(ctx context.Context, userID, accountID vo.Id) (bool, error) {
	owner, err := s.owners.AccountOwner(ctx, accountID)
	if err != nil {
		var nf *errs.NotFoundError
		if errors.As(err, &nf) {
			return false, nil
		}
		return false, err
	}
	if owner.Equal(userID) {
		return true, nil
	}
	return s.grants.HasWriteGrant(ctx, accountID, userID)
}
