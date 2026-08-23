package budget

import (
	"context"
	"sort"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/sortkey"
	"github.com/econumo/econumo/internal/shared/vo"
)

// MoveElement places one budget element immediately after req.AfterId (nil =
// first) within req.FolderId (nil = the no-folder group), and writes one row.
// The element is identified by its EXTERNAL id with no type discriminator, so
// the first match wins -- the same rule the endpoint this replaces used.
//
// Budget elements are shared per budget, so this is gated on canUpdate: any
// editor reorders for everyone.
func (s *Service) MoveElement(ctx context.Context, userID vo.Id, req model.MoveElementRequest) (*model.MoveElementResult, error) {
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
	if aerr := s.requireNotArchived(b); aerr != nil {
		return nil, aerr
	}

	var folderID *vo.Id
	if req.FolderId != nil && *req.FolderId != "" {
		fid, ferr := vo.ParseId(*req.FolderId)
		if ferr != nil {
			return nil, model.ValidateBlank(map[string]string{"folderId": ""})
		}
		if !b.hasFolder(fid) {
			return nil, accessDenied()
		}
		folderID = &fid
	}
	afterID := ""
	if req.AfterId != nil {
		afterID = *req.AfterId
	}

	// First-seen wins: the request keys elements by external id only.
	var moved *model.BudgetElement
	for _, e := range b.elements {
		if e.ExternalID.String() == req.Id {
			moved = e
			break
		}
	}

	if moved != nil && folderID != nil && sideMixed(b.elements, *folderID, moved.Type) {
		return nil, folderSideMixedErr()
	}

	now := s.clock.Now()
	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		if moved != nil {
			// Siblings are the elements already in the TARGET group, excluding the
			// moved one -- which may be arriving from another folder.
			siblings := groupElements(b.elements, folderID, moved.ExternalID)
			key, kerr := sortkey.Place(siblings, afterID, sortkey.GrowsDown)
			if kerr != nil {
				return kerr
			}
			moved.UpdateFolder(folderID, now)
			moved.UpdateSortKey(key, now)
			if serr := s.elements.SaveElement(txCtx, moved); serr != nil {
				return serr
			}
		}
		return s.syncElements(txCtx, b.budget.ID, now)
	}); err != nil {
		return nil, err
	}
	return &model.MoveElementResult{}, nil
}

// groupElements returns the key-ordered siblings sharing a folder, skipping
// unset (archived / envelope-child / non-participant) rows and the moved row.
//
// Siblings are keyed by EXTERNAL id, not by the budgets_elements row id, because
// that is the id the wire carries: get-budget reports an element as its
// envelope/category/tag id, so move-element's afterId is one of those. Keying
// them by row id makes every anchor lookup miss, and Place then silently appends
// -- which reads as rows jumping to the end of the group.
func groupElements(elements []*model.BudgetElement, folderID *vo.Id, exclude vo.Id) []sortkey.Item {
	out := make([]sortkey.Item, 0, len(elements))
	for _, e := range elements {
		if e.ExternalID.Equal(exclude) || e.IsSortKeyUnset() || !inFolder(e, folderID) {
			continue
		}
		out = append(out, sortkey.Item{ID: e.ExternalID.String(), Key: e.SortKey})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Key != out[j].Key {
			return out[i].Key < out[j].Key
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func inFolder(e *model.BudgetElement, folderID *vo.Id) bool {
	if folderID == nil {
		return e.FolderID == nil
	}
	return e.FolderID != nil && e.FolderID.Equal(*folderID)
}

// folderSide reports the folder's derived side over its member elements: no
// members = neutral (both false). Archived members count — a folder holding
// only an archived income category is still income-sided.
func folderSide(elements []*model.BudgetElement, folderID vo.Id) (income, expense bool) {
	for _, e := range elements {
		if e.FolderID == nil || !e.FolderID.Equal(folderID) {
			continue
		}
		if e.Type.IsIncomeSide() {
			income = true
		} else {
			expense = true
		}
	}
	return income, expense
}

// sideMixed reports whether placing an element of type typ into the folder
// would mix income and expense sides. Same-side members never conflict, so the
// element being re-placed inside its own folder needs no exclusion.
func sideMixed(elements []*model.BudgetElement, folderID vo.Id, typ model.ElementType) bool {
	income, expense := folderSide(elements, folderID)
	if typ.IsIncomeSide() {
		return expense
	}
	return income
}

func folderSideMixedErr() error {
	return errs.NewValidation("Validation failed", errs.FieldError{
		Key: "folderId", Message: "A folder cannot contain both income and expenses", Code: errs.CodeBudgetFolderSideMixed,
	})
}

// syncElements reconciles the budgets_elements rows with the entities that
// currently participate in the budget. Ordering is NOT its job -- keys are set
// by MoveElement and by envelope creation, and removing a row leaves its
// siblings correctly ordered. What remains is the bookkeeping that has to happen
// after any element-mutating use case:
//
//   - create a row for every participant envelope / category (both sides) / tag
//     that lacks one, appended to the end of the no-folder group;
//   - force archived elements and envelope-child categories to the unset key
//     (and, for children, no folder) so they drop out of the listing;
//   - give a live element that has no key one, which is how an unarchived
//     element re-enters the listing;
//   - delete rows whose entity no longer participates.
//
// Types are reconciled in place by external id -- a side change never deletes
// a row (limits cascade).
//
// All budget element-mutating use cases (move, envelope create/update/delete)
// run this as their last step.
func (s *Service) syncElements(ctx context.Context, budgetID vo.Id, now time.Time) error {
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

	// Index existing elements by "<externalId>-<typeAlias>" AND by external id.
	// The external index is what makes a type (side) change an in-place update:
	// one row per external id (UNIQUE(budget_id, external_id)), and a
	// delete+recreate would cascade the row's limits away -- while the recreate
	// would trip the unique constraint, because saves run before the
	// delete-unseen pass below.
	byKey := map[string]*model.BudgetElement{}
	byExternal := map[string]*model.BudgetElement{}
	for _, e := range b.elements {
		byKey[elementKey(e.ExternalID.String(), e.Type)] = e
		byExternal[e.ExternalID.String()] = e
	}
	seen := map[string]bool{}
	created := map[string]*model.BudgetElement{}
	dirty := map[string]*model.BudgetElement{}
	// live marks element keys that belong in the listing: a non-archived
	// participant element that is not an envelope-child category.
	live := map[string]bool{}

	mark := func(e *model.BudgetElement) { dirty[e.ID.String()] = e }

	ensure := func(externalID vo.Id, typ model.ElementType) (*model.BudgetElement, string) {
		if e, ok := byExternal[externalID.String()]; ok && e.Type != typ {
			delete(byKey, elementKey(externalID.String(), e.Type))
			e.UpdateType(typ, now)
			byKey[elementKey(externalID.String(), typ)] = e
			mark(e)
		}
		key := elementKey(externalID.String(), typ)
		seen[key] = true
		if e, ok := byKey[key]; ok {
			return e, key
		}
		// A missing element starts keyless; it is given one below, appended to the
		// end of the no-folder group.
		e := model.NewBudgetElement(s.elements.NextIdentity(), budgetID, externalID, typ, nil, nil, now)
		byKey[key] = e
		byExternal[externalID.String()] = e
		created[key] = e
		return e, key
	}
	forceUnset := func(e *model.BudgetElement) {
		if !e.IsSortKeyUnset() {
			e.UpdateSortKey("", now)
			mark(e)
		}
	}

	// --- envelopes (+ collect child categories) ---
	childCategories := map[string]bool{}
	for _, env := range b.envelopes {
		// The element row stores the envelope's (immutable) side; default to
		// expense only when no row exists yet.
		typ := model.ElementEnvelope
		if e, ok := byExternal[env.ID.String()]; ok && e.Type == model.ElementIncomeEnvelope {
			typ = model.ElementIncomeEnvelope
		}
		e, key := ensure(env.ID, typ)
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

	// --- categories (both sides; the type encodes the side) ---
	cats, err := s.metadata.CategoriesByOwners(ctx, userIDs)
	if err != nil {
		return err
	}
	for _, c := range cats {
		typ := model.ElementCategory
		if c.IsIncome {
			typ = model.ElementIncomeCategory
		}
		cid, perr := vo.ParseId(c.ID)
		if perr != nil {
			return perr
		}
		e, key := ensure(cid, typ)
		if childCategories[c.ID] {
			// A category that belongs to an envelope is hidden from the top level:
			// unset key + no folder.
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

	if aerr := s.assignMissingKeys(byKey, live, mark, now); aerr != nil {
		return aerr
	}

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

// assignMissingKeys gives every live element that has no key one, appending to
// the end of the no-folder group. That covers rows just created for a new
// participant and rows that were archived (key cleared) and have come back.
// Ordering is by element id so the result does not depend on map iteration.
func (s *Service) assignMissingKeys(byKey map[string]*model.BudgetElement, live map[string]bool, mark func(*model.BudgetElement), now time.Time) error {
	needsKey := make([]*model.BudgetElement, 0)
	tail := sortkey.Key("")
	for key, e := range byKey {
		if !live[key] {
			continue
		}
		if e.IsSortKeyUnset() {
			needsKey = append(needsKey, e)
			continue
		}
		if e.FolderID == nil && e.SortKey > tail {
			tail = e.SortKey
		}
	}
	if len(needsKey) == 0 {
		return nil
	}
	sort.SliceStable(needsKey, func(i, j int) bool { return needsKey[i].ID.String() < needsKey[j].ID.String() })
	for _, e := range needsKey {
		var k sortkey.Key
		var err error
		if tail == "" {
			k = sortkey.Seed(sortkey.GrowsDown)
		} else {
			k, err = sortkey.Between(tail, "")
		}
		if err != nil {
			return err
		}
		e.UpdateSortKey(k, now)
		mark(e)
		tail = k
	}
	return nil
}
