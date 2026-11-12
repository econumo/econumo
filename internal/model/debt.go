package model

import (
	"slices"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type Debt struct {
	ID, BudgetID, CurrencyID vo.Id
	Name                     string
	// Rules are ordered by CreatedAt, then ID; a payment counts when it
	// matches any of them.
	Rules []DebtRule
	// StartDate bounds the payments that count; on a set-up debt it is the
	// day the loan began, the as-of of its first snapshot (the origin).
	StartDate *time.Time
	// AmountBorrowed is the origin's balance, held while the debt is set up.
	AmountBorrowed vo.DecimalNumber
	// Current is nil until the debt is set up (amount borrowed, start date
	// and last payment date entered); Payment and Schedule are held either way.
	Current  *DebtTerms
	Payment  vo.DecimalNumber
	Schedule RecurringSchedule
	// CreatedDay is the creation date in the creating user's timezone, fixed
	// at creation so a later timezone change cannot move the planning start.
	CreatedDay           time.Time
	CreatedAt, UpdatedAt time.Time
}

func (d *Debt) IsSetUp() bool { return d.Current != nil }

// SetTerms sets the debt up with t, keeping the debt's payment and schedule
// equal to the terms'.
func (d *Debt) SetTerms(t DebtTerms) {
	d.Current = &t
	d.Payment, d.Schedule = t.Payment, t.Schedule
}

// DebtRule is one way a payment counts for a debt: an expense in the
// category, narrowed by a payee and by a tag or a label when they are set.
type DebtRule struct {
	ID, CategoryID          vo.Id
	PayeeID, TagID, LabelID *vo.Id
	CreatedAt               time.Time
}

func (r DebtRule) IsWhole() bool { return r.PayeeID == nil && r.TagID == nil && r.LabelID == nil }

func (r DebtRule) SameKey(o DebtRule) bool {
	return r.CategoryID.Equal(o.CategoryID) && sameOptionalID(r.PayeeID, o.PayeeID) &&
		sameOptionalID(r.TagID, o.TagID) && sameOptionalID(r.LabelID, o.LabelID)
}

// Overlaps reports whether one transaction can match both rules. Two different
// labels never overlap: a payment carrying both goes to the older debt.
func (r DebtRule) Overlaps(o DebtRule) bool {
	return r.CategoryID.Equal(o.CategoryID) && optionalIDsMeet(r.PayeeID, o.PayeeID) &&
		optionalIDsMeet(r.TagID, o.TagID) && optionalIDsMeet(r.LabelID, o.LabelID)
}

// Covers reports whether every transaction matching o also matches r.
func (r DebtRule) Covers(o DebtRule) bool {
	return r.CategoryID.Equal(o.CategoryID) && optionalIDCovers(r.PayeeID, o.PayeeID) &&
		optionalIDCovers(r.TagID, o.TagID) && optionalIDCovers(r.LabelID, o.LabelID)
}

func sameOptionalID(a, b *vo.Id) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func optionalIDsMeet(a, b *vo.Id) bool { return a == nil || b == nil || a.Equal(*b) }

func optionalIDCovers(a, b *vo.Id) bool { return a == nil || (b != nil && a.Equal(*b)) }

func sameStartDate(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func compareCreated(at time.Time, aID vo.Id, bt time.Time, bID vo.Id) int {
	if c := at.Compare(bt); c != 0 {
		return c
	}
	return strings.Compare(aID.String(), bID.String())
}

func debtKeyConflict() error {
	return errs.NewValidation("Validation failed", errs.FieldError{Key: "rules",
		Message: "These payment rules could match the same payments.", Code: errs.CodeDebtKeyConflict})
}

// rulesConflict checks a rule of d against a rule of another debt of the
// budget. Identical keys may coexist with distinct start dates, which
// partition the payments; any other overlap could count a payment twice.
func rulesConflict(d *Debt, r DebtRule, other *Debt, o DebtRule) error {
	if !r.Overlaps(o) {
		return nil
	}
	if !r.SameKey(o) {
		return debtKeyConflict()
	}
	if sameStartDate(d.StartDate, other.StartDate) {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "startDate",
			Message: "Another debt with the same category, payee, tag and reporting tag already starts on this date.",
			Code:    errs.CodeDebtStartDateTaken})
	}
	return nil
}

// CheckDebtRules refuses rules of d that could match a payment another rule
// of d, or a rule of another debt of the budget, also matches. Two rules of
// one category with different labels coexist, in one debt or across debts.
func CheckDebtRules(d *Debt, budgetDebts []*Debt) error {
	for i, r := range d.Rules {
		for _, o := range d.Rules[i+1:] {
			if r.Overlaps(o) {
				return debtKeyConflict()
			}
		}
	}
	for _, other := range budgetDebts {
		if other.ID.Equal(d.ID) {
			continue
		}
		for _, r := range d.Rules {
			for _, o := range other.Rules {
				if err := rulesConflict(d, r, other, o); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func sharesRuleKey(a, b *Debt) bool {
	for _, r := range a.Rules {
		for _, o := range b.Rules {
			if r.SameKey(o) {
				return true
			}
		}
	}
	return false
}

// DebtUntil is the start of the earliest later debt of the budget with a rule
// identical to one of d's: from then on that debt takes over, and d stops
// matching on every one of its rules (one loan, one range).
func DebtUntil(d *Debt, budgetDebts []*Debt) *time.Time {
	var until *time.Time
	for _, o := range budgetDebts {
		if o.ID.Equal(d.ID) || o.StartDate == nil || (d.StartDate != nil && !o.StartDate.After(*d.StartDate)) {
			continue
		}
		if !sharesRuleKey(d, o) || (until != nil && !o.StartDate.Before(*until)) {
			continue
		}
		start := *o.StartDate
		until = &start
	}
	return until
}

// IsWholeCategoryRule: r takes the whole category's plans only when no other
// debt of the budget has a whole rule on it too (debts told apart by their
// start dates share the category's months).
func IsWholeCategoryRule(d *Debt, r DebtRule, budgetDebts []*Debt) bool {
	if !r.IsWhole() {
		return false
	}
	for _, o := range budgetDebts {
		if o.ID.Equal(d.ID) {
			continue
		}
		for _, or := range o.Rules {
			if or.IsWhole() && or.CategoryID.Equal(r.CategoryID) {
				return false
			}
		}
	}
	return true
}

// SoleWholeCategory is the category a debt can hand its plans back to: only
// a debt whose one rule is whole-category has one.
func SoleWholeCategory(d *Debt, budgetDebts []*Debt) (vo.Id, bool) {
	if len(d.Rules) != 1 || !IsWholeCategoryRule(d, d.Rules[0], budgetDebts) {
		return vo.Id{}, false
	}
	return d.Rules[0].CategoryID, true
}

// NormalizeBudgetRules resolves the conflicts a classification delete or
// merge can leave in a budget's debts, never refusing: oldest debt first
// (CreatedAt, then ID), a rule that conflicts with an older debt's rule is
// dropped, then a rule covered by another surviving rule of the same debt (of
// two identical, the newer). The cross-debt pass goes first so a narrower rule
// survives when the rule covering it is lost to an older debt. It returns the
// debts whose rules changed.
func NormalizeBudgetRules(budgetDebts []*Debt) []*Debt {
	ordered := slices.Clone(budgetDebts)
	slices.SortStableFunc(ordered, func(a, b *Debt) int { return compareCreated(a.CreatedAt, a.ID, b.CreatedAt, b.ID) })
	var changed []*Debt
	for i, d := range ordered {
		free := make([]DebtRule, 0, len(d.Rules))
		for _, r := range d.Rules {
			if !conflictsWithOlder(d, r, ordered[:i]) {
				free = append(free, r)
			}
		}
		kept := make([]DebtRule, 0, len(free))
		for j, r := range free {
			if !coveredByAnother(free, j) {
				kept = append(kept, r)
			}
		}
		if len(kept) != len(d.Rules) {
			d.Rules = kept
			changed = append(changed, d)
		}
	}
	return changed
}

func conflictsWithOlder(d *Debt, r DebtRule, older []*Debt) bool {
	for _, o := range older {
		for _, or := range o.Rules {
			if rulesConflict(d, r, o, or) != nil {
				return true
			}
		}
	}
	return false
}

func coveredByAnother(rules []DebtRule, j int) bool {
	r := rules[j]
	for k, o := range rules {
		if k == j || !o.Covers(r) {
			continue
		}
		if !r.Covers(o) || compareCreated(o.CreatedAt, o.ID, r.CreatedAt, r.ID) < 0 {
			return true
		}
	}
	return false
}

// DebtScope is what a budget counts debt payments on: its member accounts,
// soft-deleted ones included.
type DebtScope struct {
	BudgetID   vo.Id
	AccountIDs []vo.Id
}

// DebtBudget is a budget as the debt service sees it for one user.
type DebtBudget struct {
	ID, CurrencyID vo.Id
	Name           string
	PeriodStartDay int
	// CurrencyIDs are the budget's own currency and its element rows'.
	CurrencyIDs []vo.Id
	IsArchived  bool
	// CanPlan: the user is the owner, an admin or a user (not a guest).
	CanPlan bool
	// ParticipantIDs are the owner and the accepted non-guests, whose
	// categories the budget shows.
	ParticipantIDs []vo.Id
	AccountIDs     []vo.Id
}

// DebtClassification is a category, payee, tag or label a debt rule names.
type DebtClassification struct {
	OwnerID    vo.Id
	Name, Icon string
	IsExpense  bool
}

type DebtTerms struct {
	AsOf            time.Time
	Balance         vo.DecimalNumber
	Payment         vo.DecimalNumber
	Schedule        RecurringSchedule
	LastPaymentDate time.Time
}

type DebtSnapshot struct {
	ID, DebtID vo.Id
	AsOf       time.Time
	Balance    vo.DecimalNumber
	Payment    vo.DecimalNumber
	Schedule   RecurringSchedule
	// LastPaymentDate's day-of-month anchors month-based schedules.
	LastPaymentDate time.Time
	CreatedAt       time.Time
}

type DebtSplit struct {
	DebtID, TransactionID vo.Id
	Interest              vo.DecimalNumber
}

// DebtRuleView is a DebtRule as the budget reads match it.
type DebtRuleView struct {
	CategoryID              string
	PayeeID, TagID, LabelID *string
}

func (r DebtRuleView) matches(categoryID, payeeID, tagID *string, labelIDs []string) bool {
	if categoryID == nil || *categoryID != r.CategoryID {
		return false
	}
	if r.PayeeID != nil && (payeeID == nil || *payeeID != *r.PayeeID) {
		return false
	}
	if r.TagID != nil && (tagID == nil || *tagID != *r.TagID) {
		return false
	}
	return r.LabelID == nil || slices.Contains(labelIDs, *r.LabelID)
}

// DebtView is the matching rules and range shared by the debt service, the
// budget SQL glue and the projected read.
type DebtView struct {
	ID, BudgetID, CurrencyID string
	Rules                    []DebtRuleView
	Name, Icon               string
	CreatedAt                time.Time
	SetUp                    bool
	From                     *time.Time // inclusive; nil = unbounded
	Until                    *time.Time // exclusive (DebtUntil); nil = unbounded
	// FirstDay and LastDay are the dates the planning bounds come from; a
	// budget plans the debt in the periods holding them (PlanningBounds).
	// StartMonth and EndMonth are their calendar months.
	FirstDay, LastDay    time.Time
	StartMonth, EndMonth time.Time
	PaidOff              bool
}

// PlanningBounds are the first and last period keys a budget whose periods
// start on startDay can plan the debt in. A view without days falls back to
// its calendar months.
func (d DebtView) PlanningBounds(startDay int) (first, last time.Time) {
	first, last = d.StartMonth, d.EndMonth
	if !d.FirstDay.IsZero() {
		first = PeriodOf(d.FirstDay, startDay)
	}
	if !d.LastDay.IsZero() {
		last = PeriodOf(d.LastDay, startDay)
	}
	return first, last
}

// ElementDataTransfer moves one budget row's plans and comments onto another
// row of BudgetID when both are in CurrencyID.
type ElementDataTransfer struct {
	BudgetID         vo.Id
	FromID, ToID     vo.Id
	FromType, ToType ElementType
	CurrencyID       vo.Id
	// SumOnClash adds a moved plan to the target's plan for that month;
	// otherwise the target's wins and the moved one is dropped.
	SumOnClash bool
	// FirstDay and LastDay bound the data moved (zero = open): the budget
	// moves its periods from the one holding FirstDay to the one holding
	// LastDay; data outside them stays on the source row.
	FirstDay, LastDay time.Time
	// AfterDay, when set, moves only the periods after the one holding it,
	// up to LastDay. A day bound cannot say "the next period" for a budget
	// whose periods start on another day.
	AfterDay time.Time
	// BeforeDay, when set, moves only the periods before the one holding it.
	BeforeDay time.Time
}

// CarriesKey reports whether a budget row of the category with this tag can
// hold the debt's payments: some rule names the category, with no tag or
// this one.
func (d DebtView) CarriesKey(categoryID, tagID *string) bool {
	if categoryID == nil {
		return false
	}
	for _, r := range d.Rules {
		if r.CategoryID == *categoryID && (r.TagID == nil || (tagID != nil && *tagID == *r.TagID)) {
			return true
		}
	}
	return false
}

func DebtMatches(d DebtView, categoryID, payeeID, tagID *string, labelIDs []string, at time.Time) bool {
	if d.From != nil && at.Before(*d.From) {
		return false
	}
	if d.Until != nil && !at.Before(*d.Until) {
		return false
	}
	for _, r := range d.Rules {
		if r.matches(categoryID, payeeID, tagID, labelIDs) {
			return true
		}
	}
	return false
}

// AssignDebt picks the debt a transaction pays: of the matching views, the
// oldest (CreatedAt, then ID), so a payment carrying two debts' labels counts
// once.
func AssignDebt(views []DebtView, categoryID, payeeID, tagID *string, labelIDs []string, at time.Time) (DebtView, bool) {
	var best DebtView
	found := false
	for _, v := range views {
		if !DebtMatches(v, categoryID, payeeID, tagID, labelIDs, at) {
			continue
		}
		if !found || v.CreatedAt.Before(best.CreatedAt) || (v.CreatedAt.Equal(best.CreatedAt) && v.ID < best.ID) {
			best, found = v, true
		}
	}
	return best, found
}
