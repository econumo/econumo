package model

import (
	"slices"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

type Debt struct {
	ID, OwnerID, CurrencyID, CategoryID vo.Id
	Name                                string
	PayeeID                             *vo.Id
	TagID                               *vo.Id
	LabelID                             *vo.Id
	StartDate                           *time.Time
	// Current is nil until the debt is set up (a balance and a last payment
	// date entered); Payment and Schedule are held either way.
	Current              *DebtTerms
	Payment              vo.DecimalNumber
	Schedule             RecurringSchedule
	CreatedAt, UpdatedAt time.Time
}

func (d *Debt) IsSetUp() bool { return d.Current != nil }

// SetTerms sets the debt up with t, keeping the debt's payment and schedule
// equal to the terms'.
func (d *Debt) SetTerms(t DebtTerms) {
	d.Current = &t
	d.Payment, d.Schedule = t.Payment, t.Schedule
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

// DebtView is the matching key and range shared by the debt service, the
// budget SQL glue and the projected read.
type DebtView struct {
	ID, OwnerID, CurrencyID, CategoryID string
	Name, Icon                          string
	PayeeID, TagID, LabelID             *string
	CreatedAt                           time.Time
	SetUp                               bool
	From                                *time.Time // inclusive; nil = unbounded
	Until                               *time.Time // exclusive (next same-key debt's start); nil = unbounded
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
// row in every budget where both are in CurrencyID.
type ElementDataTransfer struct {
	FromID, ToID     vo.Id
	FromType, ToType ElementType
	CurrencyID       vo.Id
	// SumOnClash adds a moved plan to the target's plan for that month;
	// otherwise the target's wins and the moved one is dropped.
	SumOnClash bool
	// FirstDay and LastDay bound the data moved (zero = open): each budget
	// moves its periods from the one holding FirstDay to the one holding
	// LastDay; data outside them stays on the source row.
	FirstDay, LastDay time.Time
	// AfterDay, when set, moves only the periods after the one holding it,
	// up to LastDay. A day bound cannot say "the next period" for budgets
	// whose periods start on different days.
	AfterDay time.Time
}

func DebtMatches(d DebtView, categoryID, payeeID, tagID *string, labelIDs []string, at time.Time) bool {
	if categoryID == nil || *categoryID != d.CategoryID {
		return false
	}
	if d.PayeeID != nil && (payeeID == nil || *payeeID != *d.PayeeID) {
		return false
	}
	if d.TagID != nil && (tagID == nil || *tagID != *d.TagID) {
		return false
	}
	if d.LabelID != nil && !slices.Contains(labelIDs, *d.LabelID) {
		return false
	}
	if d.From != nil && at.Before(*d.From) {
		return false
	}
	if d.Until != nil && !at.Before(*d.Until) {
		return false
	}
	return true
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
