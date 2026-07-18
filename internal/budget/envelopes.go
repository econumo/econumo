package budget

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// CreateEnvelope creates an envelope + its budget element, assigns categories,
// and returns the new element (canUpdate). A new envelope has zero budgeted/spent.
func (s *Service) CreateEnvelope(ctx context.Context, userID vo.Id, req model.CreateEnvelopeRequest) (*model.CreateEnvelopeResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"budgetId": ""})
	}
	envelopeID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"id": ""})
	}
	if err := model.ValidateName("Envelope", req.Name); err != nil {
		return nil, err
	}
	curID, err := vo.ParseId(req.CurrencyId)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"currencyId": ""})
	}
	var folderID *vo.Id
	if req.FolderId != nil && *req.FolderId != "" {
		fid, ferr := vo.ParseId(*req.FolderId)
		if ferr != nil {
			return nil, model.ValidateBlank(map[string]string{"folderId": ""})
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
	// The new envelope element is created at position 0 (the front of its group);
	// restoreElementsOrder then renumbers the rest. The response reports position 0
	// because it is built from the just-created element, before restore mutates the
	// reloaded rows.
	const newPosition = 0
	now := s.clock.Now()
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		if eerr := s.requireFreeEnvelopeID(txCtx, envelopeID); eerr != nil {
			return eerr
		}
		// Shift existing same-group (same folder) elements up by one to free
		// position 0, so the new element is the unique position-0 row in its group
		// before restoreElementsOrder runs.
		if serr := s.shiftElements(txCtx, b, folderID, newPosition, now); serr != nil {
			return serr
		}
		env := model.NewBudgetEnvelope(envelopeID, budgetID, req.Name, req.Icon, now)
		if serr := s.envelopes.SaveEnvelope(txCtx, env); serr != nil {
			return serr
		}
		el := model.NewBudgetElement(s.elements.NextIdentity(), budgetID, envelopeID, model.ElementEnvelope, &curID, folderID, int16(newPosition), now)
		if serr := s.elements.SaveElement(txCtx, el); serr != nil {
			return serr
		}
		for _, raw := range req.Categories {
			catID, perr := vo.ParseId(raw)
			if perr != nil {
				return model.ValidateBlank(map[string]string{"categories": ""})
			}
			if serr := s.envelopes.AddEnvelopeCategory(txCtx, envelopeID, catID); serr != nil {
				return serr
			}
		}
		return s.restoreElementsOrder(txCtx, budgetID, now)
	})
	if err != nil {
		return nil, err
	}
	children, err := s.envelopeChildren(ctx, b, req.Categories)
	if err != nil {
		return nil, err
	}
	return &model.CreateEnvelopeResult{Item: newEnvelopeElementResult(envelopeID, req.Name, req.Icon, curID, folderID, newPosition, false, children)}, nil
}

// UpdateEnvelope updates an envelope's name/icon/archived + categories (canUpdate).
func (s *Service) UpdateEnvelope(ctx context.Context, userID vo.Id, req model.UpdateEnvelopeRequest) (*model.UpdateEnvelopeResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"budgetId": ""})
	}
	envelopeID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"id": ""})
	}
	if err := model.ValidateName("Envelope", req.Name); err != nil {
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
	if !s.canUpdate(b, userID) {
		return nil, accessDenied()
	}
	if !b.hasEnvelope(envelopeID) {
		return nil, accessDenied()
	}
	now := s.clock.Now()
	var position int16
	var folderID *vo.Id
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		env, gerr := s.envelopes.GetEnvelope(txCtx, envelopeID)
		if gerr != nil {
			return gerr
		}
		env.UpdateName(req.Name, now)
		env.UpdateIcon(req.Icon, now)
		env.SetArchived(req.IsArchived == 1, now)
		if serr := s.envelopes.SaveEnvelope(txCtx, env); serr != nil {
			return serr
		}
		el, eerr := s.elements.GetElementByExternal(txCtx, budgetID, envelopeID)
		if eerr == nil {
			el.UpdateCurrency(&curID, now)
			if serr := s.elements.SaveElement(txCtx, el); serr != nil {
				return serr
			}
			position = el.Position
			folderID = el.FolderID
		}
		// Replace category assignments.
		existing, cerr := s.envelopes.EnvelopeCategoryIDs(txCtx, envelopeID)
		if cerr != nil {
			return cerr
		}
		want := map[string]bool{}
		for _, raw := range req.Categories {
			catID, perr := vo.ParseId(raw)
			if perr != nil {
				return model.ValidateBlank(map[string]string{"categories": ""})
			}
			want[catID.String()] = true
			if serr := s.envelopes.AddEnvelopeCategory(txCtx, envelopeID, catID); serr != nil {
				return serr
			}
		}
		for _, ex := range existing {
			if !want[ex.String()] {
				if serr := s.envelopes.RemoveEnvelopeCategory(txCtx, envelopeID, ex); serr != nil {
					return serr
				}
			}
		}
		return s.restoreElementsOrder(txCtx, budgetID, now)
	})
	if err != nil {
		return nil, err
	}
	children, err := s.envelopeChildren(ctx, b, req.Categories)
	if err != nil {
		return nil, err
	}
	return &model.UpdateEnvelopeResult{Item: newEnvelopeElementResult(envelopeID, req.Name, req.Icon, curID, folderID, int(position), req.IsArchived == 1, children)}, nil
}

// DeleteEnvelope removes an envelope + its element (canDelete).
func (s *Service) DeleteEnvelope(ctx context.Context, userID vo.Id, req model.DeleteEnvelopeRequest) (*model.DeleteEnvelopeResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"budgetId": ""})
	}
	envelopeID, err := vo.ParseId(req.Id)
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
	if !b.hasEnvelope(envelopeID) {
		return nil, accessDenied()
	}
	now := s.clock.Now()
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		el, eerr := s.elements.GetElementByExternal(txCtx, budgetID, envelopeID)
		if eerr == nil {
			if serr := s.elements.DeleteElement(txCtx, el.ID); serr != nil {
				return serr
			}
		}
		if serr := s.envelopes.DeleteEnvelope(txCtx, envelopeID); serr != nil {
			return serr
		}
		return s.restoreElementsOrder(txCtx, budgetID, now)
	})
	if err != nil {
		return nil, err
	}
	return &model.DeleteEnvelopeResult{}, nil
}

// nextElementPosition returns max(element position)+1 for a budget.
func nextElementPosition(b *budgetAggregate) int {
	max := -1
	for _, e := range b.elements {
		if int(e.Position) > max {
			max = int(e.Position)
		}
	}
	return max + 1
}

// newEnvelopeElementResult builds the model.ParentElementResult for a freshly
// created/updated envelope (no spending yet -> zero money fields). children are
// the envelope's category children (also zero spending).
func newEnvelopeElementResult(id vo.Id, name, icon string, currencyID vo.Id, folderID *vo.Id, position int, archived bool, children []model.ChildElementResult) model.ParentElementResult {
	var fid *string
	if folderID != nil {
		s := folderID.String()
		fid = &s
	}
	if children == nil {
		children = []model.ChildElementResult{}
	}
	return model.ParentElementResult{
		Id: id.String(), Type: int(model.ElementEnvelope.Int16()), Name: name, Icon: icon,
		CurrencyId: currencyID.String(), IsArchived: boolToInt(archived), FolderId: fid, Position: position,
		Budgeted: "0", Available: "0", Spent: "0", BudgetSpent: "0",
		Children: children, OwnerUserId: nil,
	}
}

// envelopeChildren resolves a set of category ids to the response child shape
// (category metadata + zero spending). Order follows the requested category ids;
// the API comparison is order-insensitive.
func (s *Service) envelopeChildren(ctx context.Context, b *budgetAggregate, categoryIDs []string) ([]model.ChildElementResult, error) {
	if len(categoryIDs) == 0 {
		return []model.ChildElementResult{}, nil
	}
	userIDs := []vo.Id{b.budget.UserID}
	for _, a := range b.access {
		if a.IsAccepted && a.Role != roleGuest() {
			userIDs = append(userIDs, a.UserID)
		}
	}
	cats, err := s.metadata.CategoriesByOwners(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	byID := map[string]model.CategoryMeta{}
	for _, c := range cats {
		if c.IsIncome { // only expense categories are eligible participants
			continue
		}
		byID[c.ID] = c
	}
	out := make([]model.ChildElementResult, 0, len(categoryIDs))
	for _, cid := range categoryIDs {
		c, ok := byID[cid]
		if !ok {
			continue
		}
		out = append(out, model.ChildElementResult{
			Id: c.ID, Type: int(model.ElementCategory.Int16()), Name: c.Name, Icon: c.Icon,
			IsArchived: boolToInt(c.IsArchived), Spent: "0", BudgetSpent: "0", OwnerUserId: c.OwnerID,
		})
	}
	return out, nil
}
