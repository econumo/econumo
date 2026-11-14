package debt

import (
	"context"
	"errors"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

var errNoElements = errors.New("debt: the budget elements peer is not installed")

func (s *Service) element(ctx context.Context, budgetID, elementID vo.Id) (model.DebtElementInfo, error) {
	if s.elements == nil {
		return model.DebtElementInfo{}, errNoElements
	}
	return s.elements.Element(ctx, budgetID, elementID)
}

// elementsOf describes the elements the debts are attached to, each once.
func (s *Service) elementsOf(ctx context.Context, debts []*model.Debt) (map[vo.Id]model.DebtElementInfo, error) {
	out := make(map[vo.Id]model.DebtElementInfo, len(debts))
	for _, d := range debts {
		if _, ok := out[d.ElementID]; ok {
			continue
		}
		el, err := s.element(ctx, d.BudgetID, d.ElementID)
		if err != nil {
			return nil, err
		}
		out[d.ElementID] = el
	}
	return out, nil
}

// onElement are the debts of the list attached to the element, d excluded.
func onElement(debts []*model.Debt, elementID vo.Id, d *model.Debt) []*model.Debt {
	var out []*model.Debt
	for _, o := range debts {
		if o.ElementID.Equal(elementID) && (d == nil || !o.ID.Equal(d.ID)) {
			out = append(out, o)
		}
	}
	return out
}

func createdBefore(a, b *model.Debt) bool {
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.Before(b.CreatedAt)
	}
	return compareIDs(a.ID, b.ID) < 0
}

// previousEnd is the latest end of the element's paid-off debts created
// before d (all of them for a debt not created yet): d starts after it.
func previousEnd(elementDebts []*model.Debt, d *model.Debt) *time.Time {
	var end *time.Time
	for _, o := range elementDebts {
		if o.EndedAt == nil || (d != nil && !createdBefore(o, d)) {
			continue
		}
		if end == nil || o.EndedAt.After(*end) {
			e := *o.EndedAt
			end = &e
		}
	}
	return end
}

// checkStart refuses a start on or before the end of the element's previous
// debt: one element's debts never share a day.
func checkStart(start, prev *time.Time) error {
	if start == nil || prev == nil || start.After(*prev) {
		return nil
	}
	return fieldError("startDate", "The start date must be after the end of the previous debt on this category or envelope.",
		errs.CodeDebtStartBeforePreviousEnd)
}

func compareIDs(a, b vo.Id) int {
	as, bs := a.String(), b.String()
	switch {
	case as < bs:
		return -1
	case as > bs:
		return 1
	}
	return 0
}

func notEligible() error {
	return fieldError("elementId", "A debt can be tracked on an expense category outside an envelope, or on an envelope.",
		errs.CodeDebtElementNotEligible)
}

func elementHasDebt() error {
	return fieldError("elementId", "This category or envelope already tracks a debt. Mark that debt as paid off first.",
		errs.CodeDebtElementHasDebt)
}

func cannotReopen() error {
	return &errs.ValidationError{Msg: "This debt cannot be reopened: its category or envelope tracks another debt that is active or started later.",
		MsgCode: errs.CodeDebtCannotReopen}
}

func paidOff() error {
	return &errs.ValidationError{Msg: "This debt is paid off. Reopen it to change it.", MsgCode: errs.CodeDebtPaidOff}
}
