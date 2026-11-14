package model

import (
	"slices"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// DebtElementType is the kind of budget element a debt is attached to.
type DebtElementType string

const (
	DebtElementCategory DebtElementType = "category"
	DebtElementEnvelope DebtElementType = "envelope"
)

func (t DebtElementType) Valid() bool { return t == DebtElementCategory || t == DebtElementEnvelope }

// DebtStatus: a paid-off debt has an end (EndedAt) and is read-only.
type DebtStatus string

const (
	DebtActive  DebtStatus = "active"
	DebtPaidOff DebtStatus = "paid_off"
)

type Debt struct {
	ID, BudgetID, CurrencyID vo.Id
	// ElementID is the external id of the budget element the debt is
	// attached to: the category or the envelope id.
	ElementID   vo.Id
	ElementType DebtElementType
	Status      DebtStatus
	// EndedAt is the last day payments count, set while paid off.
	EndedAt *time.Time
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

func (d *Debt) IsPaidOff() bool { return d.Status == DebtPaidOff }

// SetTerms sets the debt up with t, keeping the debt's payment and schedule
// equal to the terms'.
func (d *Debt) SetTerms(t DebtTerms) {
	d.Current = &t
	d.Payment, d.Schedule = t.Payment, t.Schedule
}

// DebtScope is what a budget counts debt payments on: its everyday (not
// savings) member accounts, soft-deleted ones included, the accounts an
// element's Spent counts.
type DebtScope struct {
	BudgetID   vo.Id
	AccountIDs []vo.Id
}

// DebtBudget is a budget as the debt service sees it for one user.
type DebtBudget struct {
	ID, CurrencyID vo.Id
	Name           string
	PeriodStartDay int
	IsArchived     bool
	// CanPlan: the user is the owner, an admin or a user (not a guest).
	CanPlan    bool
	AccountIDs []vo.Id
}

// DebtElementInfo is a budget element as a debt reads it. Eligible: it can
// carry a debt, a top-level expense category or an expense envelope of the
// budget. CategoryIDs are the categories whose expenses its Spent counts (an
// envelope's current children). An unknown id has a zero Type.
type DebtElementInfo struct {
	ID          vo.Id
	Type        DebtElementType
	Name, Icon  string
	CurrencyID  vo.Id
	CategoryIDs []vo.Id
	Eligible    bool
}

// DebtClassification is a category as a debt's presentation reads it.
type DebtClassification struct {
	Name, Icon string
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

type DebtRateChange struct {
	ID, DebtID    vo.Id
	EffectiveDate time.Time
	AnnualRate    vo.DecimalNumber
	Payment       vo.DecimalNumber
	CreatedAt     time.Time
}

type DebtSplit struct {
	DebtID, TransactionID vo.Id
	Interest              vo.DecimalNumber
}

// DebtView is a debt's element and range as the debt service, the budget SQL
// glue and the projected read share them. Name and Icon are the element's.
type DebtView struct {
	ID, BudgetID, CurrencyID string
	ElementID                string
	ElementType              DebtElementType
	// CategoryIDs are the element's categories, whose untagged expenses pay
	// the debt.
	CategoryIDs []string
	Name, Icon  string
	CreatedAt   time.Time
	SetUp       bool
	From        *time.Time // inclusive; nil = unbounded
	Until       *time.Time // inclusive (the end of a paid-off debt); nil = open
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

// CarriesKey reports whether a budget row of the category with this tag holds
// the debt's payments: an untagged expense in one of its element's categories.
func (d DebtView) CarriesKey(categoryID, tagID *string) bool {
	if categoryID == nil || (tagID != nil && *tagID != "") {
		return false
	}
	return slices.Contains(d.CategoryIDs, *categoryID)
}

// DebtMatches: an untagged expense in the element's categories, dated within
// the debt's range.
func DebtMatches(d DebtView, categoryID, tagID *string, at time.Time) bool {
	day := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC)
	if d.From != nil && day.Before(*d.From) {
		return false
	}
	if d.Until != nil && day.After(*d.Until) {
		return false
	}
	return d.CarriesKey(categoryID, tagID)
}

// AssignDebt picks the debt a transaction pays. Ranges of one element's
// debts never overlap, but a category's own debt can overlap the debt of the
// envelope it now sits in (it moved in once its debt was paid off, or the
// envelope's debt has no start date): a payment inside a range of the
// category's own debt belongs to it, whatever the debts' age, any other to
// the envelope's.
func AssignDebt(views []DebtView, categoryID, tagID *string, at time.Time) (DebtView, bool) {
	var found DebtView
	var ok bool
	for _, v := range views {
		if !DebtMatches(v, categoryID, tagID, at) {
			continue
		}
		if v.ElementType == DebtElementCategory {
			return v, true
		}
		if !ok {
			found, ok = v, true
		}
	}
	return found, ok
}
