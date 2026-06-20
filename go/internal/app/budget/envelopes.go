package budget

import (
	"context"

	dombudget "github.com/econumo/econumo/internal/domain/budget"
	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// CreateEnvelope creates an envelope + its budget element, assigns categories,
// and returns the new element (canUpdate). New envelope: zero budgeted/spent.
func (s *Service) CreateEnvelope(ctx context.Context, userID vo.Id, req CreateEnvelopeRequest) (*CreateEnvelopeResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, validateBlank(map[string]string{"budgetId": ""})
	}
	envelopeID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, validateBlank(map[string]string{"id": ""})
	}
	if err := dombudget.ValidateName("Envelope", req.Name); err != nil {
		return nil, err
	}
	curID, err := vo.ParseId(req.CurrencyId)
	if err != nil {
		return nil, validateBlank(map[string]string{"currencyId": ""})
	}
	var folderID *vo.Id
	if req.FolderId != nil && *req.FolderId != "" {
		fid, ferr := vo.ParseId(*req.FolderId)
		if ferr != nil {
			return nil, validateBlank(map[string]string{"folderId": ""})
		}
		folderID = &fid
	}
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	if !s.canUpdate(b, userID) {
		return nil, accessDenied()
	}
	position := nextElementPosition(b)
	now := s.clock.Now()
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		env := dombudget.NewBudgetEnvelope(envelopeID, budgetID, req.Name, req.Icon, now)
		if serr := s.repo.SaveEnvelope(txCtx, env); serr != nil {
			return serr
		}
		el := dombudget.NewBudgetElement(s.repo.NextIdentity(), budgetID, envelopeID, dombudget.ElementEnvelope, &curID, folderID, int16(position), now)
		if serr := s.repo.SaveElement(txCtx, el); serr != nil {
			return serr
		}
		for _, raw := range req.Categories {
			catID, perr := vo.ParseId(raw)
			if perr != nil {
				return validateBlank(map[string]string{"categories": ""})
			}
			if serr := s.repo.AddEnvelopeCategory(txCtx, envelopeID, catID); serr != nil {
				return serr
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &CreateEnvelopeResult{Item: newEnvelopeElementResult(envelopeID, req.Name, req.Icon, curID, folderID, position, false)}, nil
}

// UpdateEnvelope updates an envelope's name/icon/archived + categories (canUpdate).
func (s *Service) UpdateEnvelope(ctx context.Context, userID vo.Id, req UpdateEnvelopeRequest) (*UpdateEnvelopeResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, validateBlank(map[string]string{"budgetId": ""})
	}
	envelopeID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, validateBlank(map[string]string{"id": ""})
	}
	if err := dombudget.ValidateName("Envelope", req.Name); err != nil {
		return nil, err
	}
	curID, err := vo.ParseId(req.CurrencyId)
	if err != nil {
		return nil, validateBlank(map[string]string{"currencyId": ""})
	}
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	if !s.canUpdate(b, userID) {
		return nil, accessDenied()
	}
	now := s.clock.Now()
	var position int16
	var folderID *vo.Id
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		env, gerr := s.repo.GetEnvelope(txCtx, envelopeID)
		if gerr != nil {
			return gerr
		}
		env.UpdateName(req.Name, now)
		env.UpdateIcon(req.Icon, now)
		env.SetArchived(req.IsArchived == 1, now)
		if serr := s.repo.SaveEnvelope(txCtx, env); serr != nil {
			return serr
		}
		// element: update display currency.
		el, eerr := s.repo.GetElementByExternal(txCtx, budgetID, envelopeID)
		if eerr == nil {
			el.UpdateCurrency(&curID, now)
			if serr := s.repo.SaveElement(txCtx, el); serr != nil {
				return serr
			}
			position = el.Position()
			folderID = el.FolderId()
		}
		// replace category assignments.
		existing, cerr := s.repo.EnvelopeCategoryIDs(txCtx, envelopeID)
		if cerr != nil {
			return cerr
		}
		want := map[string]bool{}
		for _, raw := range req.Categories {
			catID, perr := vo.ParseId(raw)
			if perr != nil {
				return validateBlank(map[string]string{"categories": ""})
			}
			want[catID.String()] = true
			if serr := s.repo.AddEnvelopeCategory(txCtx, envelopeID, catID); serr != nil {
				return serr
			}
		}
		for _, ex := range existing {
			if !want[ex.String()] {
				if serr := s.repo.RemoveEnvelopeCategory(txCtx, envelopeID, ex); serr != nil {
					return serr
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &UpdateEnvelopeResult{Item: newEnvelopeElementResult(envelopeID, req.Name, req.Icon, curID, folderID, int(position), req.IsArchived == 1)}, nil
}

// DeleteEnvelope removes an envelope + its element (canDelete).
func (s *Service) DeleteEnvelope(ctx context.Context, userID vo.Id, req DeleteEnvelopeRequest) (*DeleteEnvelopeResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, validateBlank(map[string]string{"budgetId": ""})
	}
	envelopeID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, validateBlank(map[string]string{"id": ""})
	}
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	if !s.canDelete(b, userID) {
		return nil, accessDenied()
	}
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		el, eerr := s.repo.GetElementByExternal(txCtx, budgetID, envelopeID)
		if eerr == nil {
			if serr := s.repo.DeleteElement(txCtx, el.Id()); serr != nil {
				return serr
			}
		}
		return s.repo.DeleteEnvelope(txCtx, envelopeID)
	})
	if err != nil {
		return nil, err
	}
	return &DeleteEnvelopeResult{}, nil
}

// nextElementPosition returns max(element position)+1 for a budget.
func nextElementPosition(b *budgetAggregate) int {
	max := -1
	for _, e := range b.elements {
		if int(e.Position()) > max {
			max = int(e.Position())
		}
	}
	return max + 1
}

// newEnvelopeElementResult builds the ParentElementResult for a freshly
// created/updated envelope (no spending yet -> zero money fields).
func newEnvelopeElementResult(id vo.Id, name, icon string, currencyID vo.Id, folderID *vo.Id, position int, archived bool) ParentElementResult {
	var fid *string
	if folderID != nil {
		s := folderID.String()
		fid = &s
	}
	return ParentElementResult{
		Id: id.String(), Type: int(dombudget.ElementEnvelope.Int16()), Name: name, Icon: icon,
		CurrencyId: currencyID.String(), IsArchived: boolToInt(archived), FolderId: fid, Position: position,
		Budgeted: "0", Available: "0", Spent: "0", BudgetSpent: "0",
		Children: []ChildElementResult{}, OwnerUserId: nil,
	}
}
