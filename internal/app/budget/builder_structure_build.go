package budget

import (
	"context"
	"fmt"
	"sort"

	dombudget "github.com/econumo/econumo/internal/domain/budget"
	domcurrency "github.com/econumo/econumo/internal/domain/currency"
	"github.com/econumo/econumo/internal/shared/vo"
)

// structElement is the in-progress parent element accumulated during the walk
// before the bulkConvert resolves spent/available amounts.
type structElement struct {
	id             string
	typ            dombudget.ElementType
	name           string
	icon           string
	ownerID        *string
	currencyID     vo.Id
	isArchived     bool
	folderID       *string
	position       int16
	budgeted       vo.DecimalNumber
	budgetedBefore vo.DecimalNumber
	children       []structChild
}

type structChild struct {
	id         string
	typ        dombudget.ElementType
	name       string
	icon       string
	ownerID    string
	isArchived bool
	subIndex   string // for looking up converted amounts
}

// buildStructure walks envelopes -> tags -> standalone categories, accumulates a
// toConvert map, runs one bulkConvert, then emits the pruned, sorted parent
// elements.
func (s *Service) buildStructure(ctx context.Context, b *budgetAggregate, f filters, limits map[string]budgetedAmount, spending map[string]*elementSpending) (StructureResult, error) {
	options := s.elementOptions(b)
	folders := make([]FolderResult, 0, len(b.folders))
	for _, fl := range b.folders {
		folders = append(folders, FolderResult{Id: fl.Id().String(), Name: fl.Name(), Position: int(fl.Position())})
	}
	sortByPositionThenID(folders, func(f FolderResult) int { return f.Position }, func(f FolderResult) string { return f.Id })

	toConvert := map[string][]domcurrency.ConvertItem{}
	categoryUsed := map[string]bool{}
	budgetCurrencyID := b.budget.CurrencyId()
	var elements []*structElement

	zero := vo.NewDecimal("0")

	// envelope/category/tag categories must be resolvable; the categories map is
	// expense-only, so envelope children reference it.
	envelopeCats, err := s.envelopeCategories(ctx, b)
	if err != nil {
		return StructureResult{}, err
	}

	// --- Envelopes ---
	for _, env := range b.envelopes {
		index := elementKey(env.Id().String(), dombudget.ElementEnvelope)
		opt := options[index]
		currencyID := budgetCurrencyID
		if opt.currencyID != nil {
			currencyID = *opt.currencyID
		}
		bud := limits[index]
		budgeted, budgetedBefore := orZero(bud.budgeted, zero), orZero(bud.budgetedBefore, zero)
		el := &structElement{
			id: env.Id().String(), typ: dombudget.ElementEnvelope, name: env.Name(), icon: env.Icon(),
			ownerID: nil, currencyID: currencyID, isArchived: env.IsArchived(),
			folderID: optFolder(opt), position: optPosition(opt), budgeted: budgeted, budgetedBefore: budgetedBefore,
		}
		for _, catID := range envelopeCats[env.Id().String()] {
			if categoryUsed[catID] {
				continue
			}
			cat, ok := f.categories[catID]
			if !ok {
				continue // income or not a participant category
			}
			subIndex := elementKey(catID, dombudget.ElementCategory)
			cs := categorySpendingFor(spending, subIndex, catID)
			addSpendingConvert(toConvert, index, subIndex, cs, currencyID, budgetCurrencyID)
			el.children = append(el.children, structChild{
				id: catID, typ: dombudget.ElementCategory, name: cat.Name, icon: cat.Icon,
				ownerID: cat.OwnerID, isArchived: cat.IsArchived, subIndex: subIndex,
			})
			categoryUsed[catID] = true
		}
		if !env.IsArchived() || !budgeted.IsZero() || !budgetedBefore.IsZero() || len(el.children) > 0 {
			elements = append(elements, el)
		}
	}

	// --- Tags ---
	for tagID, tag := range f.tags {
		index := elementKey(tagID, dombudget.ElementTag)
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
			id: tagID, typ: dombudget.ElementTag, name: tag.Name, icon: "tag",
			ownerID: strPtr(tag.OwnerID), currencyID: currencyID, isArchived: tag.IsArchived,
			folderID: optFolder(opt), position: optPosition(opt), budgeted: budgeted, budgetedBefore: budgetedBefore,
		}
		if es != nil {
			for catID, cs := range es.spendingInCategories {
				cat, ok := f.categories[catID]
				if !ok {
					continue
				}
				subIndex := elementKey(catID, dombudget.ElementCategory)
				addSpendingConvert(toConvert, index, subIndex, cs, currencyID, budgetCurrencyID)
				el.children = append(el.children, structChild{
					id: catID, typ: dombudget.ElementCategory, name: cat.Name, icon: cat.Icon,
					ownerID: cat.OwnerID, isArchived: cat.IsArchived, subIndex: subIndex,
				})
			}
		}
		if !tag.IsArchived || !budgeted.IsZero() || !budgetedBefore.IsZero() || len(el.children) > 0 {
			elements = append(elements, el)
		}
	}

	// --- standalone Categories ---
	for catID, cat := range f.categories {
		if categoryUsed[catID] {
			continue
		}
		index := elementKey(catID, dombudget.ElementCategory)
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
			id: catID, typ: dombudget.ElementCategory, name: cat.Name, icon: cat.Icon,
			ownerID: strPtr(cat.OwnerID), currencyID: currencyID, isArchived: cat.IsArchived,
			folderID: optFolder(opt), position: optPosition(opt), budgeted: budgeted, budgetedBefore: budgetedBefore,
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

	// One bulk conversion for everything.
	amounts, err := s.convertor.BulkConvert(ctx, f.periodStart, f.periodEnd, toConvert)
	if err != nil {
		return StructureResult{}, err
	}
	get := func(key string) vo.DecimalNumber {
		if v, ok := amounts[key]; ok {
			return v
		}
		return zero
	}

	result := []ParentElementResult{}
	for _, el := range elements {
		index := elementKey(el.id, el.typ)
		spent := get(fmt.Sprintf("spent_%s", index))
		spentBudget := get(fmt.Sprintf("spent-budget_%s", index))
		spentBefore := get(fmt.Sprintf("spent-before_%s", index))

		// An element with no children must emit "children":[], never null. Start from
		// a non-nil empty slice so the JSON matches (a nil slice marshals to null).
		children := []ChildElementResult{}
		for _, ch := range el.children {
			subSpent := get(fmt.Sprintf("%s_spent_%s", index, ch.subIndex))
			subBudget := get(fmt.Sprintf("%s_spent-budget_%s", index, ch.subIndex))
			if ch.isArchived && subSpent.IsZero() {
				continue
			}
			if el.typ == dombudget.ElementTag && subSpent.IsZero() {
				continue
			}
			children = append(children, ChildElementResult{
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
			(el.typ != dombudget.ElementEnvelope || len(children) == 0) {
			continue
		}

		result = append(result, ParentElementResult{
			Id: el.id, Type: int(el.typ.Int16()), Name: el.name, Icon: el.icon,
			CurrencyId: el.currencyID.String(), IsArchived: boolToInt(el.isArchived),
			FolderId: el.folderID, Position: int(el.position),
			Budgeted: el.budgeted.String(), Available: available.Sub(spent).String(),
			Spent: spent.String(), BudgetSpent: spentBudget.String(),
			Children: children, OwnerUserId: el.ownerID,
		})
	}
	sortByPositionThenID(result, func(p ParentElementResult) int { return p.Position }, func(p ParentElementResult) string { return p.Id })

	return StructureResult{Folders: folders, Elements: result}, nil
}

// addSpendingConvert appends the spent / spent-budget / spent-before convert
// items for a child category under a parent (envelope or tag), keyed both
// per-child and per-parent.
func addSpendingConvert(toConvert map[string][]domcurrency.ConvertItem, index, subIndex string, cs *categorySpending, elementCurrency, budgetCurrency vo.Id) {
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
		out[elementKey(e.ExternalId().String(), e.Type())] = elementOption{
			currencyID: e.CurrencyId(), folderID: e.FolderId(), position: e.Position(),
		}
	}
	return out
}

// elementOption captures the per-element row (currency/folder/position).
type elementOption struct {
	currencyID *vo.Id
	folderID   *vo.Id
	position   int16
}

func optFolder(o elementOption) *string {
	if o.folderID == nil {
		return nil
	}
	s := o.folderID.String()
	return &s
}

func optPosition(o elementOption) int16 { return o.position }

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
		ids, err := s.repo.EnvelopeCategoryIDs(ctx, env.Id())
		if err != nil {
			return nil, err
		}
		strs := make([]string, len(ids))
		for i, id := range ids {
			strs[i] = id.String()
		}
		out[env.Id().String()] = strs
	}
	return out, nil
}
