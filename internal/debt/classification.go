package debt

import (
	"context"
	"fmt"
	"sort"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

const (
	KindCategory = "category"
	KindPayee    = "payee"
	KindTag      = "tag"
	KindLabel    = "label"
)

// InUse reports whether any debt is keyed on the classification.
func (s *Service) InUse(ctx context.Context, kind string, id vo.Id) (bool, error) {
	switch kind {
	case KindCategory:
		return s.repo.UsesCategory(ctx, id)
	case KindPayee:
		return s.repo.UsesPayee(ctx, id)
	case KindTag:
		return s.repo.UsesTag(ctx, id)
	case KindLabel:
		return s.repo.UsesLabel(ctx, id)
	}
	return false, fmt.Errorf("debt: unknown classification kind %q", kind)
}

// ReassignClassification moves the debts keyed on oldID to newID for a merge
// and refuses the merge when that would leave two debts of an owner able to
// match the same payment. It runs inside the merge's transaction, after the
// transactions have been re-pointed and the budget rows merged, so the
// payment splits are recomputed against the merged data. A whole-category
// debt on the target category takes the merged category row's plans, whether
// it moved or was there already.
func (s *Service) ReassignClassification(ctx context.Context, kind string, oldID, newID vo.Id) error {
	moved, err := s.keyedOn(ctx, kind, oldID)
	if err != nil {
		return err
	}
	var present []*model.Debt
	if kind == KindCategory {
		if present, err = s.repo.ListByCategory(ctx, newID); err != nil {
			return err
		}
	}
	if len(moved) == 0 && len(present) == 0 {
		return nil
	}
	locked := map[vo.Id]bool{}
	for _, d := range append(append([]*model.Debt{}, moved...), present...) {
		if locked[d.OwnerID] {
			continue
		}
		locked[d.OwnerID] = true
		if err := s.repo.LockOwner(ctx, d.OwnerID); err != nil {
			return err
		}
	}
	if len(moved) > 0 {
		switch kind {
		case KindCategory:
			err = s.repo.ReassignCategory(ctx, oldID, newID)
		case KindPayee:
			err = s.repo.ReassignPayee(ctx, oldID, newID)
		case KindLabel:
			err = s.repo.ReassignLabel(ctx, oldID, newID)
		default:
			err = s.repo.ReassignTag(ctx, oldID, newID)
		}
		if err != nil {
			return err
		}
	}

	categories := map[vo.Id]bool{}
	for _, m := range moved {
		d, err := s.repo.GetByID(ctx, m.ID)
		if err != nil {
			return err
		}
		others, err := s.repo.ListByCategory(ctx, d.CategoryID)
		if err != nil {
			return err
		}
		if checkKeys(d, others) != nil {
			return &errs.ValidationError{Msg: "Another debt could match the same payments.", MsgCode: errs.CodeDebtKeyConflict}
		}
		if kind == KindCategory && isWholeCategory(d) {
			if err := s.moveToDebt(ctx, d); err != nil {
				return err
			}
		}
		categories[d.CategoryID] = true
	}
	for _, d := range present {
		if isWholeCategory(d) {
			if err := s.moveToDebt(ctx, d); err != nil {
				return err
			}
		}
		categories[d.CategoryID] = true
	}
	for c := range categories {
		if _, err := s.syncCategory(ctx, c); err != nil {
			return err
		}
	}
	return nil
}

// LockForMerge locks, in id order, the owners of the debts keyed on either
// side of a merge. The merge takes them before writing anything, so its order
// matches every debt write's: the owner row first, then budget rows.
func (s *Service) LockForMerge(ctx context.Context, kind string, oldID, newID vo.Id) error {
	var owners []vo.Id
	seen := map[vo.Id]bool{}
	for _, id := range []vo.Id{oldID, newID} {
		debts, err := s.keyedOn(ctx, kind, id)
		if err != nil {
			return err
		}
		for _, d := range debts {
			if !seen[d.OwnerID] {
				seen[d.OwnerID] = true
				owners = append(owners, d.OwnerID)
			}
		}
	}
	sort.Slice(owners, func(i, j int) bool { return owners[i].String() < owners[j].String() })
	for _, o := range owners {
		if err := s.repo.LockOwner(ctx, o); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) keyedOn(ctx context.Context, kind string, id vo.Id) ([]*model.Debt, error) {
	switch kind {
	case KindCategory:
		return s.repo.ListByCategory(ctx, id)
	case KindPayee, KindTag, KindLabel:
		return s.repo.ListByClassification(ctx, kind, id)
	}
	return nil, fmt.Errorf("debt: unknown classification kind %q", kind)
}
