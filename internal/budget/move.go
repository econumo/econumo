package budget

import (
	"context"
	"sort"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// MoveElementList repositions budget elements and moves them between folders
// (canUpdate). Each item identifies an element by its external id + type.
func (s *Service) MoveElementList(ctx context.Context, userID vo.Id, req model.MoveElementListRequest) (*model.MoveElementListResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"budgetId": ""})
	}
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	if !s.canUpdate(b, userID) {
		return nil, accessDenied()
	}

	// Index elements by their EXTERNAL id (first-seen wins): the move request keys
	// elements by external id with no type discriminator.
	byExternal := map[string]*model.BudgetElement{}
	for _, e := range b.elements {
		k := e.ExternalID.String()
		if _, seen := byExternal[k]; !seen {
			byExternal[k] = e
		}
	}

	now := s.clock.Now()
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		for _, item := range req.Items {
			el := byExternal[item.Id]
			if el == nil {
				continue
			}
			var folderID *vo.Id
			if item.FolderId != nil && *item.FolderId != "" {
				fid, ferr := vo.ParseId(*item.FolderId)
				if ferr != nil {
					return model.ValidateBlank(map[string]string{"folderId": ""})
				}
				folderID = &fid
			}
			el.UpdateFolder(folderID, now)
			el.UpdatePosition(int16(item.Position), now)
			if serr := s.elements.SaveElement(txCtx, el); serr != nil {
				return serr
			}
		}
		// Always finish with restoreElementsOrder, which renumbers every element's
		// position contiguously within its folder (and the no-folder group) in
		// position order, skipping position-unset rows.
		return s.restoreElementsOrder(txCtx, b.budget.ID, now)
	})
	if err != nil {
		return nil, err
	}
	return &model.MoveElementListResult{}, nil
}

// shiftElements bumps the positions of same-group (same folder) elements with
// position >= startPosition up by one, freeing startPosition for a new element.
// Iterates in position order; the counter starts at startPosition and
// pre-increments per match.
func (s *Service) shiftElements(ctx context.Context, b *budgetAggregate, folderID *vo.Id, startPosition int16, now time.Time) error {
	elems := append([]*model.BudgetElement(nil), b.elements...)
	sort.SliceStable(elems, func(i, j int) bool {
		if elems[i].Position != elems[j].Position {
			return elems[i].Position < elems[j].Position
		}
		return elems[i].ID.String() < elems[j].ID.String()
	})
	pos := startPosition
	for _, e := range elems {
		// same group?
		if folderID == nil {
			if e.FolderID != nil {
				continue
			}
		} else {
			if e.FolderID == nil || !e.FolderID.Equal(*folderID) {
				continue
			}
		}
		if e.Position < startPosition {
			continue
		}
		pos++
		e.UpdatePosition(pos, now)
		if serr := s.elements.SaveElement(ctx, e); serr != nil {
			return serr
		}
	}
	return nil
}

// posMax is the in-memory sort sentinel for "position unset but participating"
// elements. It only affects ordering before renumbering; it is never persisted
// (every such element is renumbered to a real position or stays unset).
const posMax = int16(0x7fff)

// restoreElementsOrder guarantees every participant envelope / expense-category /
// tag has a budget element, normalizes archived → position-unset and
// live-but-unset → end-of-list, forces envelope-child categories to unset +
// no-folder, then renumbers every live element contiguously (0-based) within each
// folder and within the no-folder group — iterating in position-ASC order.
// Finally it deletes elements whose entity no longer participates.
//
// All budget element-mutating use cases (move, envelope create/update/delete) run
// this as their last step.
func (s *Service) restoreElementsOrder(ctx context.Context, budgetID vo.Id, now time.Time) error {
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return err
	}
	// Participant users = owner + accepted non-reader access.
	userIDs := []vo.Id{b.budget.UserID}
	for _, a := range b.access {
		if a.IsAccepted && a.Role != roleGuest() {
			userIDs = append(userIDs, a.UserID)
		}
	}

	// Index existing elements by "<externalId>-<typeAlias>".
	byKey := map[string]*model.BudgetElement{}
	for _, e := range b.elements {
		byKey[elementKey(e.ExternalID.String(), e.Type)] = e
	}
	seen := map[string]bool{}
	created := map[string]*model.BudgetElement{}
	dirty := map[string]*model.BudgetElement{}
	// live marks element keys that participate in the renumbering (a non-archived
	// participant element that is not an envelope-child category). Archived /
	// child / non-participant elements are forced to position 0 and excluded.
	live := map[string]bool{}

	ensure := func(externalID vo.Id, typ model.ElementType) (*model.BudgetElement, string) {
		key := elementKey(externalID.String(), typ)
		seen[key] = true
		if e, ok := byKey[key]; ok {
			return e, key
		}
		// Missing element: create it at posMax so it sorts to the END of its group
		// during renumber.
		e := model.NewBudgetElement(s.elements.NextIdentity(), budgetID, externalID, typ, nil, nil, posMax, now)
		byKey[key] = e
		created[key] = e
		return e, key
	}
	mark := func(e *model.BudgetElement) { dirty[e.ID.String()] = e }
	forceUnset := func(e *model.BudgetElement) {
		if !e.IsPositionUnset() {
			e.UpdatePosition(model.PositionUnset, now)
			mark(e)
		}
	}

	// --- envelopes (+ collect child categories) ---
	childCategories := map[string]bool{}
	for _, env := range b.envelopes {
		e, key := ensure(env.ID, model.ElementEnvelope)
		if env.IsArchived {
			forceUnset(e)
		} else {
			live[key] = true
		}
		catIDs, cerr := s.envelopes.EnvelopeCategoryIDs(ctx, env.ID)
		if cerr != nil {
			return cerr
		}
		for _, c := range catIDs {
			childCategories[c.String()] = true
		}
	}

	// --- expense categories ---
	cats, err := s.metadata.CategoriesByOwners(ctx, userIDs)
	if err != nil {
		return err
	}
	for _, c := range cats {
		if c.IsIncome {
			continue // expense categories only
		}
		cid, perr := vo.ParseId(c.ID)
		if perr != nil {
			return perr
		}
		e, key := ensure(cid, model.ElementCategory)
		if childCategories[c.ID] {
			// A category that belongs to an envelope is hidden from the top level:
			// position unset + no folder.
			forceUnset(e)
			if e.FolderID != nil {
				e.UpdateFolder(nil, now)
				mark(e)
			}
		} else if c.IsArchived {
			forceUnset(e)
		} else {
			live[key] = true
		}
	}

	// --- tags ---
	tags, err := s.metadata.TagsByOwners(ctx, userIDs)
	if err != nil {
		return err
	}
	for _, t := range tags {
		tid, perr := vo.ParseId(t.ID)
		if perr != nil {
			return perr
		}
		e, key := ensure(tid, model.ElementTag)
		if t.IsArchived {
			forceUnset(e)
		} else {
			live[key] = true
		}
	}

	// Renumber: iterate LIVE elements in position-ASC order, assigning contiguous
	// 0-based positions within each folder, then within the no-folder group.
	// Archived/child/non-participant elements stay at position 0 (excluded).
	all := make([]*model.BudgetElement, 0, len(byKey))
	keyOf := map[*model.BudgetElement]string{}
	for k, e := range byKey {
		all = append(all, e)
		keyOf[e] = k
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].Position != all[j].Position {
			return all[i].Position < all[j].Position
		}
		// Deterministic tie-break for equal positions (map iteration is random):
		// by element id. Matches a stable position-ASC ordering.
		return all[i].ID.String() < all[j].ID.String()
	})

	renumber := func(match func(*model.BudgetElement) bool) {
		pos := int16(0)
		for _, e := range all {
			if !live[keyOf[e]] || !match(e) {
				continue
			}
			if e.Position != pos {
				e.UpdatePosition(pos, now)
				mark(e)
			}
			pos++
		}
	}
	for _, f := range b.folders {
		fid := f.ID
		renumber(func(e *model.BudgetElement) bool {
			return e.FolderID != nil && e.FolderID.Equal(fid)
		})
	}
	renumber(func(e *model.BudgetElement) bool { return e.FolderID == nil })

	// Persist created + dirtied elements.
	for _, e := range created {
		dirty[e.ID.String()] = e
	}
	for _, e := range dirty {
		if serr := s.elements.SaveElement(ctx, e); serr != nil {
			return serr
		}
	}

	// Delete elements whose entity no longer participates (not seen).
	for key, e := range byKey {
		if !seen[key] {
			if serr := s.elements.DeleteElement(ctx, e.ID); serr != nil {
				return serr
			}
		}
	}
	return nil
}
