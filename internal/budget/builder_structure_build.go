package budget

import (
	"context"
	"fmt"
	"sort"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/sortkey"
	"github.com/econumo/econumo/internal/shared/vo"
)

// The presentation-only Uncategorized element sorts last via structElement's
// sortLast flag rather than a magic position, so it cannot collide with a real
// element's key.

// structElement is the in-progress parent element accumulated during the walk
// before the bulkConvert resolves spent/available amounts.
type structElement struct {
	id             string
	typ            model.ElementType
	name           string
	icon           string
	ownerID        *string
	currencyID     vo.Id
	isArchived     bool
	folderID       *string
	sortKey        sortkey.Key
	sortLast       bool
	budgeted       vo.DecimalNumber
	budgetedBefore vo.DecimalNumber
	children       []structChild
}

type structChild struct {
	id         string
	typ        model.ElementType
	name       string
	icon       string
	ownerID    string
	isArchived bool
	subIndex   string // for looking up converted amounts
}

// buildStructure walks envelopes -> tags -> standalone categories, accumulates a
// toConvert map, runs one bulkConvert, then emits the pruned, sorted parent
// elements.
func (s *Service) buildStructure(ctx context.Context, b *budgetAggregate, f filters, limits map[string]budgetedAmount, spending map[string]*elementSpending) (model.StructureResult, error) {
	options := s.elementOptions(b)

	// Income-sided envelopes and folders exist only in the plan view; the
	// budget view's wire contract is frozen without them.
	incomeEnvelopes := map[string]bool{}
	incomeFolders := map[string]bool{}
	for _, e := range b.elements {
		if e.Type == model.ElementIncomeEnvelope {
			incomeEnvelopes[e.ExternalID.String()] = true
		}
		if e.Type.IsIncomeSide() && e.FolderID != nil {
			incomeFolders[e.FolderID.String()] = true
		}
	}

	sorted := append([]*model.BudgetFolder(nil), b.folders...)
	sortBudgetFolders(sorted)
	folders := make([]model.BudgetFolderResult, 0, len(sorted))
	for _, fl := range sorted {
		if incomeFolders[fl.ID.String()] {
			continue
		}
		// position on the wire is the dense 0-based index; the key that produced
		// this order never leaves the server.
		folders = append(folders, model.BudgetFolderResult{Id: fl.ID.String(), Name: fl.Name, Position: len(folders)})
	}

	toConvert := map[string][]model.ConvertItem{}
	categoryUsed := map[string]bool{}
	budgetCurrencyID := b.budget.CurrencyID
	var elements []*structElement
	var kept []*structElement

	zero := vo.NewDecimal("0")

	// envelope/category/tag categories must be resolvable; the categories map is
	// expense-only, so envelope children reference it.
	envelopeCats, err := s.envelopeCategories(ctx, b)
	if err != nil {
		return model.StructureResult{}, err
	}

	// --- Envelopes ---
	for _, env := range b.envelopes {
		if incomeEnvelopes[env.ID.String()] {
			continue
		}
		index := elementKey(env.ID.String(), model.ElementEnvelope)
		opt := options[index]
		currencyID := budgetCurrencyID
		if opt.currencyID != nil {
			currencyID = *opt.currencyID
		}
		bud := limits[index]
		budgeted, budgetedBefore := orZero(bud.budgeted, zero), orZero(bud.budgetedBefore, zero)
		el := &structElement{
			id: env.ID.String(), typ: model.ElementEnvelope, name: env.Name, icon: env.Icon,
			ownerID: nil, currencyID: currencyID, isArchived: env.IsArchived,
			folderID: optFolder(opt), sortKey: optSortKey(opt), budgeted: budgeted, budgetedBefore: budgetedBefore,
		}
		for _, catID := range envelopeCats[env.ID.String()] {
			if categoryUsed[catID] {
				continue
			}
			cat, ok := f.categories[catID]
			if !ok {
				continue // income or not a participant category
			}
			subIndex := elementKey(catID, model.ElementCategory)
			cs := categorySpendingFor(spending, subIndex, catID)
			addSpendingConvert(toConvert, index, subIndex, cs, currencyID, budgetCurrencyID)
			el.children = append(el.children, structChild{
				id: catID, typ: model.ElementCategory, name: cat.Name, icon: cat.Icon,
				ownerID: cat.OwnerID, isArchived: cat.IsArchived, subIndex: subIndex,
			})
			categoryUsed[catID] = true
		}
		if !env.IsArchived || !budgeted.IsZero() || !budgetedBefore.IsZero() || len(el.children) > 0 {
			elements = append(elements, el)
		}
	}

	// --- Tags ---
	for tagID, tag := range f.tags {
		index := elementKey(tagID, model.ElementTag)
		bud := limits[index]
		budgeted, budgetedBefore := orZero(bud.budgeted, zero), orZero(bud.budgetedBefore, zero)
		es, hasSpending := spending[index]
		// A tag shows only if it participates in this budget: it has spending in or
		// before the period, OR a limit assigned (current or carried-over). Without
		// either it is just one of the user's many unrelated tags and stays hidden.
		// ("Non-zero available" reduces to budgetedBefore != 0 or spentBefore != 0,
		// both already covered here.) This deliberately keeps a budgeted-but-unspent
		// tag visible, rather than dropping it the moment its last transaction is
		// removed and making its limit vanish.
		if !hasSpending && budgeted.IsZero() && budgetedBefore.IsZero() {
			continue
		}
		opt := options[index]
		currencyID := budgetCurrencyID
		if opt.currencyID != nil {
			currencyID = *opt.currencyID
		}
		el := &structElement{
			id: tagID, typ: model.ElementTag, name: tag.Name, icon: "tag",
			ownerID: strPtr(tag.OwnerID), currencyID: currencyID, isArchived: tag.IsArchived,
			folderID: optFolder(opt), sortKey: optSortKey(opt), budgeted: budgeted, budgetedBefore: budgetedBefore,
		}
		if es != nil {
			for catID, cs := range es.spendingInCategories {
				var name, icon, ownerID string
				var isArchived bool
				if catID == model.UncategorizedID {
					// Not a real category, so it can't be looked up in f.categories -
					// without this branch the child (and its spending) would be
					// silently dropped and the tag would render as an all-zero ghost.
					name, icon = model.UncategorizedName, model.UncategorizedIcon
				} else {
					cat, ok := f.categories[catID]
					if !ok {
						continue
					}
					name, icon, ownerID, isArchived = cat.Name, cat.Icon, cat.OwnerID, cat.IsArchived
				}
				subIndex := elementKey(catID, model.ElementCategory)
				addSpendingConvert(toConvert, index, subIndex, cs, currencyID, budgetCurrencyID)
				el.children = append(el.children, structChild{
					id: catID, typ: model.ElementCategory, name: name, icon: icon,
					ownerID: ownerID, isArchived: isArchived, subIndex: subIndex,
				})
			}
		}
		if !tag.IsArchived || !budgeted.IsZero() || !budgetedBefore.IsZero() || len(el.children) > 0 {
			elements = append(elements, el)
		}
	}

	// --- Labels (budget-neutral: never folded into toConvert keys the
	// elements loop below reads) ---
	labelSpending, err := s.buildLabelSpending(ctx, f)
	if err != nil {
		return model.StructureResult{}, err
	}
	// Labels convert into the BUDGET currency only: a label has no per-element
	// currency option because it has no element.
	for labelID, byCategory := range labelSpending {
		// Spend can reference a label outside f.labels (e.g. a since-revoked
		// collaborator's label still attached to transactions on included
		// accounts); the emit loop below drops it, so skip the conversion too.
		if _, ok := f.labels[labelID]; !ok {
			continue
		}
		for catID, spends := range byCategory {
			for _, a := range spends {
				key := fmt.Sprintf("label-spent_%s", labelID)
				toConvert[key] = append(toConvert[key], convItem(a, budgetCurrencyID))
				// Per-category key mirrors the element path's parent/child shape, so a
				// child's amount converts on the same terms as its parent's share of it.
				subKey := fmt.Sprintf("label-spent_%s_%s", labelID, catID)
				toConvert[subKey] = append(toConvert[subKey], convItem(a, budgetCurrencyID))
			}
		}
	}

	// --- standalone Categories ---
	for catID, cat := range f.categories {
		if categoryUsed[catID] {
			continue
		}
		index := elementKey(catID, model.ElementCategory)
		opt := options[index]
		currencyID := budgetCurrencyID
		if opt.currencyID != nil {
			currencyID = *opt.currencyID
		}
		bud := limits[index]
		budgeted, budgetedBefore := orZero(bud.budgeted, zero), orZero(bud.budgetedBefore, zero)
		cs := categorySpendingFor(spending, index, catID)
		hasSpent := cs != nil && (len(cs.currenciesSpent) > 0 || len(cs.currenciesSpentBefore) > 0)
		if cat.IsArchived && !hasSpent && budgeted.IsZero() && budgetedBefore.IsZero() {
			continue
		}
		el := &structElement{
			id: catID, typ: model.ElementCategory, name: cat.Name, icon: cat.Icon,
			ownerID: strPtr(cat.OwnerID), currencyID: currencyID, isArchived: cat.IsArchived,
			folderID: optFolder(opt), sortKey: optSortKey(opt), budgeted: budgeted, budgetedBefore: budgetedBefore,
		}
		// a standalone category's own spending is keyed without a sub-prefix.
		if cs != nil {
			for _, sp := range cs.currenciesSpent {
				toConvert[fmt.Sprintf("spent_%s", index)] = append(toConvert[fmt.Sprintf("spent_%s", index)], convItem(sp, currencyID))
				toConvert[fmt.Sprintf("spent-budget_%s", index)] = append(toConvert[fmt.Sprintf("spent-budget_%s", index)], convItem(sp, budgetCurrencyID))
			}
			for _, sp := range cs.currenciesSpentBefore {
				toConvert[fmt.Sprintf("spent-before_%s", index)] = append(toConvert[fmt.Sprintf("spent-before_%s", index)], convItem(sp, currencyID))
			}
		}
		elements = append(elements, el)
	}

	// --- Uncategorized (presentation-only; never persisted) ---
	{
		index := elementKey(model.UncategorizedID, model.ElementCategory)
		cs := categorySpendingFor(spending, index, model.UncategorizedID)
		hasSpent := cs != nil && (len(cs.currenciesSpent) > 0 || len(cs.currenciesSpentBefore) > 0)
		if hasSpent {
			el := &structElement{
				id: model.UncategorizedID, typ: model.ElementCategory, name: model.UncategorizedName, icon: model.UncategorizedIcon,
				ownerID: nil, currencyID: budgetCurrencyID, isArchived: false,
				folderID: nil, sortLast: true, budgeted: zero, budgetedBefore: zero,
			}
			// same convert-key shape as the standalone-category pass: an element's own
			// spending is keyed without a sub-prefix.
			for _, sp := range cs.currenciesSpent {
				toConvert[fmt.Sprintf("spent_%s", index)] = append(toConvert[fmt.Sprintf("spent_%s", index)], convItem(sp, budgetCurrencyID))
				toConvert[fmt.Sprintf("spent-budget_%s", index)] = append(toConvert[fmt.Sprintf("spent-budget_%s", index)], convItem(sp, budgetCurrencyID))
			}
			for _, sp := range cs.currenciesSpentBefore {
				toConvert[fmt.Sprintf("spent-before_%s", index)] = append(toConvert[fmt.Sprintf("spent-before_%s", index)], convItem(sp, budgetCurrencyID))
			}
			elements = append(elements, el)
		}
	}

	// One bulk conversion for everything.
	amounts, err := s.convertor.BulkConvert(ctx, f.periodStart, f.periodEnd, toConvert)
	if err != nil {
		return model.StructureResult{}, err
	}
	get := func(key string) vo.DecimalNumber {
		if v, ok := amounts[key]; ok {
			return v
		}
		return zero
	}

	result := []model.ParentElementResult{}
	for _, el := range elements {
		index := elementKey(el.id, el.typ)
		spent := get(fmt.Sprintf("spent_%s", index))
		spentBudget := get(fmt.Sprintf("spent-budget_%s", index))
		spentBefore := get(fmt.Sprintf("spent-before_%s", index))

		// An element with no children must emit "children":[], never null. Start from
		// a non-nil empty slice so the JSON matches (a nil slice marshals to null).
		children := []model.ChildElementResult{}
		for _, ch := range el.children {
			subSpent := get(fmt.Sprintf("%s_spent_%s", index, ch.subIndex))
			subBudget := get(fmt.Sprintf("%s_spent-budget_%s", index, ch.subIndex))
			if ch.isArchived && subSpent.IsZero() {
				continue
			}
			if el.typ == model.ElementTag && subSpent.IsZero() {
				continue
			}
			children = append(children, model.ChildElementResult{
				Id: ch.id, Type: int(ch.typ.Int16()), Name: ch.name, Icon: ch.icon,
				IsArchived: boolToInt(ch.isArchived), Spent: subSpent.String(), BudgetSpent: subBudget.String(),
				OwnerUserId: ch.ownerID,
			})
		}
		// Children carry no position; tag children come from a map walk, so order
		// by id for a deterministic response (frontend reorders when needed).
		sort.Slice(children, func(i, j int) bool { return children[i].Id < children[j].Id })

		available := el.budgetedBefore.Sub(spentBefore)
		if el.isArchived && available.IsZero() && spent.IsZero() && el.budgeted.IsZero() &&
			(el.typ != model.ElementEnvelope || len(children) == 0) {
			continue
		}

		kept = append(kept, el)
		result = append(result, model.ParentElementResult{
			Id: el.id, Type: int(el.typ.Int16()), Name: el.name, Icon: el.icon,
			CurrencyId: el.currencyID.String(), IsArchived: boolToInt(el.isArchived),
			FolderId: el.folderID,
			Budgeted: el.budgeted.String(), Available: available.Sub(spent).String(),
			Spent: spent.String(), BudgetSpent: spentBudget.String(),
			Children: children, OwnerUserId: el.ownerID,
		})
	}
	assignElementPositions(kept, result)
	sortByPositionThenID(result, func(p model.ParentElementResult) int { return p.Position }, func(p model.ParentElementResult) string { return p.Id })

	labels := []model.LabelSpendResult{}
	for labelID, meta := range f.labels {
		// Spend is the only visibility trigger: a label has no limit that could
		// keep it on screen the way a budgeted-but-unspent tag stays visible.
		spent := get(fmt.Sprintf("label-spent_%s", labelID))
		if spent.IsZero() {
			continue
		}
		// A label's children partition ITS OWN total; the overlap is across
		// labels, never within one. Non-nil empty slice so the wire carries []
		// rather than null when every child was filtered out.
		children := []model.ChildElementResult{}
		for catID := range labelSpending[labelID] {
			subSpent := get(fmt.Sprintf("label-spent_%s_%s", labelID, catID))
			if subSpent.IsZero() {
				continue
			}
			var name, icon, ownerID string
			var isArchived bool
			if catID == model.UncategorizedID {
				// Not a real category, so it can't be looked up in f.categories -
				// without this branch the spend would be dropped and the children
				// would no longer sum to the label's total.
				name, icon = model.UncategorizedName, model.UncategorizedIcon
			} else {
				cat, ok := f.categories[catID]
				if !ok {
					continue
				}
				name, icon, ownerID, isArchived = cat.Name, cat.Icon, cat.OwnerID, cat.IsArchived
			}
			children = append(children, model.ChildElementResult{
				Id: catID, Type: int(model.ElementCategory.Int16()), Name: name, Icon: icon,
				IsArchived: boolToInt(isArchived), Spent: subSpent.String(), BudgetSpent: subSpent.String(),
				OwnerUserId: ownerID,
			})
		}
		// Children carry no position and come from a map walk, so order by id for
		// a deterministic response, as the element path does.
		sort.Slice(children, func(i, j int) bool { return children[i].Id < children[j].Id })

		labels = append(labels, model.LabelSpendResult{
			Id: labelID, Name: meta.Name, Icon: meta.Icon,
			IsArchived: boolToInt(meta.IsArchived), Spent: spent.String(),
			OwnerUserId: meta.OwnerID, Children: children,
		})
	}
	sortBySortKeyThenID(labels,
		func(l model.LabelSpendResult) string { return f.labels[l.Id].SortKey },
		func(l model.LabelSpendResult) string { return l.Id })

	return model.StructureResult{Folders: folders, Elements: result, Labels: labels}, nil
}

// addSpendingConvert appends the spent / spent-budget / spent-before convert
// items for a child category under a parent (envelope or tag), keyed both
// per-child and per-parent.
func addSpendingConvert(toConvert map[string][]model.ConvertItem, index, subIndex string, cs *categorySpending, elementCurrency, budgetCurrency vo.Id) {
	if cs == nil {
		return
	}
	for _, sp := range cs.currenciesSpent {
		toConvert[fmt.Sprintf("%s_spent_%s", index, subIndex)] = append(toConvert[fmt.Sprintf("%s_spent_%s", index, subIndex)], convItem(sp, elementCurrency))
		toConvert[fmt.Sprintf("spent_%s", index)] = append(toConvert[fmt.Sprintf("spent_%s", index)], convItem(sp, elementCurrency))
		toConvert[fmt.Sprintf("%s_spent-budget_%s", index, subIndex)] = append(toConvert[fmt.Sprintf("%s_spent-budget_%s", index, subIndex)], convItem(sp, budgetCurrency))
		toConvert[fmt.Sprintf("spent-budget_%s", index)] = append(toConvert[fmt.Sprintf("spent-budget_%s", index)], convItem(sp, budgetCurrency))
	}
	for _, sp := range cs.currenciesSpentBefore {
		toConvert[fmt.Sprintf("%s_spent-before_%s", index, subIndex)] = append(toConvert[fmt.Sprintf("%s_spent-before_%s", index, subIndex)], convItem(sp, elementCurrency))
		toConvert[fmt.Sprintf("spent-before_%s", index)] = append(toConvert[fmt.Sprintf("spent-before_%s", index)], convItem(sp, elementCurrency))
	}
}

// categorySpendingFor returns the categorySpending for a category under an
// element index (nil if absent).
func categorySpendingFor(spending map[string]*elementSpending, index, categoryID string) *categorySpending {
	es, ok := spending[index]
	if !ok {
		return nil
	}
	return es.spendingInCategories[categoryID]
}

// elementOptions maps "<externalId>-<typeAlias>" -> the budget element row.
func (s *Service) elementOptions(b *budgetAggregate) map[string]elementOption {
	out := map[string]elementOption{}
	for _, e := range b.elements {
		out[elementKey(e.ExternalID.String(), e.Type)] = elementOption{
			currencyID: e.CurrencyID, folderID: e.FolderID, sortKey: e.SortKey,
		}
	}
	return out
}

// elementOption captures the per-element row (currency/folder/sort key).
type elementOption struct {
	currencyID *vo.Id
	folderID   *vo.Id
	sortKey    sortkey.Key
}

func optFolder(o elementOption) *string {
	if o.folderID == nil {
		return nil
	}
	s := o.folderID.String()
	return &s
}

func optSortKey(o elementOption) sortkey.Key { return o.sortKey }

func orZero(d, zero vo.DecimalNumber) vo.DecimalNumber {
	if d.String() == "" {
		return zero
	}
	return d
}

// envelopeCategories returns envelopeID -> []categoryID for the budget's envelopes.
func (s *Service) envelopeCategories(ctx context.Context, b *budgetAggregate) (map[string][]string, error) {
	out := map[string][]string{}
	for _, env := range b.envelopes {
		ids, err := s.envelopes.EnvelopeCategoryIDs(ctx, env.ID)
		if err != nil {
			return nil, err
		}
		strs := make([]string, len(ids))
		for i, id := range ids {
			strs[i] = id.String()
		}
		out[env.ID.String()] = strs
	}
	return out, nil
}

// assignElementPositions stamps each element's dense 0-based index WITHIN ITS
// FOLDER, which is what the wire contract calls "position". kept and result are
// parallel: kept[i] is the element result[i] was built from.
//
// The order comes from the sort keys, which never leave the server. The
// presentation-only Uncategorized element carries sortLast so it always trails
// its group without needing a sentinel key that a real element could collide
// with.
func assignElementPositions(kept []*structElement, result []model.ParentElementResult) {
	order := make([]int, len(kept))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		x, y := kept[order[a]], kept[order[b]]
		if x.sortLast != y.sortLast {
			return !x.sortLast
		}
		if x.sortKey != y.sortKey {
			return x.sortKey < y.sortKey
		}
		return x.id < y.id
	})
	perFolder := map[string]int{}
	for _, i := range order {
		folder := ""
		if kept[i].folderID != nil {
			folder = *kept[i].folderID
		}
		result[i].Position = perFolder[folder]
		perFolder[folder]++
	}
}
