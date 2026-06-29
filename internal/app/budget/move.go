package budget

import (
	"context"
	"sort"
	"time"

	dombudget "github.com/econumo/econumo/internal/domain/budget"
	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// MoveElementList repositions budget elements and moves them between folders
// (canUpdate). Each item identifies an element by its external id + type.
func (s *Service) MoveElementList(ctx context.Context, userID vo.Id, req MoveElementListRequest) (*MoveElementListResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, validateBlank(map[string]string{"budgetId": ""})
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
	byExternal := map[string]*dombudget.BudgetElement{}
	for _, e := range b.elements {
		k := e.ExternalId().String()
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
					return validateBlank(map[string]string{"folderId": ""})
				}
				folderID = &fid
			}
			el.UpdateFolder(folderID, now)
			el.UpdatePosition(int16(item.Position), now)
			if serr := s.repo.SaveElement(txCtx, el); serr != nil {
				return serr
			}
		}
		// Always finish with restoreElementsOrder, which renumbers every element's
		// position contiguously within its folder (and the no-folder group) in
		// position order, skipping position-unset rows.
		return s.restoreElementsOrder(txCtx, b.budget.Id(), now)
	})
	if err != nil {
		return nil, err
	}
	return &MoveElementListResult{}, nil
}

// shiftElements bumps the positions of same-group (same folder) elements with
// position >= startPosition up by one, freeing startPosition for a new element.
// Iterates in position order; the counter starts at startPosition and
// pre-increments per match.
func (s *Service) shiftElements(ctx context.Context, b *budgetAggregate, folderID *vo.Id, startPosition int16, now time.Time) error {
	elems := append([]*dombudget.BudgetElement(nil), b.elements...)
	sort.SliceStable(elems, func(i, j int) bool {
		if elems[i].Position() != elems[j].Position() {
			return elems[i].Position() < elems[j].Position()
		}
		return elems[i].Id().String() < elems[j].Id().String()
	})
	pos := startPosition
	for _, e := range elems {
		// same group?
		if folderID == nil {
			if e.FolderId() != nil {
				continue
			}
		} else {
			if e.FolderId() == nil || !e.FolderId().Equal(*folderID) {
				continue
			}
		}
		if e.Position() < startPosition {
			continue
		}
		pos++
		e.UpdatePosition(pos, now)
		if serr := s.repo.SaveElement(ctx, e); serr != nil {
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
	userIDs := []vo.Id{b.budget.UserId()}
	for _, a := range b.access {
		if a.IsAccepted() && a.Role() != roleGuest() {
			userIDs = append(userIDs, a.UserId())
		}
	}

	// Index existing elements by "<externalId>-<typeAlias>".
	byKey := map[string]*dombudget.BudgetElement{}
	for _, e := range b.elements {
		byKey[elementKey(e.ExternalId().String(), e.Type())] = e
	}
	seen := map[string]bool{}
	created := map[string]*dombudget.BudgetElement{}
	dirty := map[string]*dombudget.BudgetElement{}
	// live marks element keys that participate in the renumbering (a non-archived
	// participant element that is not an envelope-child category). Archived /
	// child / non-participant elements are forced to position 0 and excluded.
	live := map[string]bool{}

	ensure := func(externalID vo.Id, typ dombudget.ElementType) (*dombudget.BudgetElement, string) {
		key := elementKey(externalID.String(), typ)
		seen[key] = true
		if e, ok := byKey[key]; ok {
			return e, key
		}
		// Missing element: create it at posMax so it sorts to the END of its group
		// during renumber.
		e := dombudget.NewBudgetElement(s.repo.NextIdentity(), budgetID, externalID, typ, nil, nil, posMax, now)
		byKey[key] = e
		created[key] = e
		return e, key
	}
	mark := func(e *dombudget.BudgetElement) { dirty[e.Id().String()] = e }
	forceUnset := func(e *dombudget.BudgetElement) {
		if !e.IsPositionUnset() {
			e.UpdatePosition(dombudget.PositionUnset, now)
			mark(e)
		}
	}

	// --- envelopes (+ collect child categories) ---
	childCategories := map[string]bool{}
	for _, env := range b.envelopes {
		e, key := ensure(env.Id(), dombudget.ElementEnvelope)
		if env.IsArchived() {
			forceUnset(e)
		} else {
			live[key] = true
		}
		catIDs, cerr := s.repo.EnvelopeCategoryIDs(ctx, env.Id())
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
		e, key := ensure(cid, dombudget.ElementCategory)
		if childCategories[c.ID] {
			// A category that belongs to an envelope is hidden from the top level:
			// position unset + no folder.
			forceUnset(e)
			if e.FolderId() != nil {
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
		e, key := ensure(tid, dombudget.ElementTag)
		if t.IsArchived {
			forceUnset(e)
		} else {
			live[key] = true
		}
	}

	// Renumber: iterate LIVE elements in position-ASC order, assigning contiguous
	// 0-based positions within each folder, then within the no-folder group.
	// Archived/child/non-participant elements stay at position 0 (excluded).
	all := make([]*dombudget.BudgetElement, 0, len(byKey))
	keyOf := map[*dombudget.BudgetElement]string{}
	for k, e := range byKey {
		all = append(all, e)
		keyOf[e] = k
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].Position() != all[j].Position() {
			return all[i].Position() < all[j].Position()
		}
		// Deterministic tie-break for equal positions (map iteration is random):
		// by element id. Matches a stable position-ASC ordering.
		return all[i].Id().String() < all[j].Id().String()
	})

	renumber := func(match func(*dombudget.BudgetElement) bool) {
		pos := int16(0)
		for _, e := range all {
			if !live[keyOf[e]] || !match(e) {
				continue
			}
			if e.Position() != pos {
				e.UpdatePosition(pos, now)
				mark(e)
			}
			pos++
		}
	}
	for _, f := range b.folders {
		fid := f.Id()
		renumber(func(e *dombudget.BudgetElement) bool {
			return e.FolderId() != nil && e.FolderId().Equal(fid)
		})
	}
	renumber(func(e *dombudget.BudgetElement) bool { return e.FolderId() == nil })

	// Persist created + dirtied elements.
	for _, e := range created {
		dirty[e.Id().String()] = e
	}
	for _, e := range dirty {
		if serr := s.repo.SaveElement(ctx, e); serr != nil {
			return serr
		}
	}

	// Delete elements whose entity no longer participates (not seen).
	for key, e := range byKey {
		if !seen[key] {
			if serr := s.repo.DeleteElement(ctx, e.Id()); serr != nil {
				return serr
			}
		}
	}
	return nil
}
