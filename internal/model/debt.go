package model

import (
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

type Debt struct {
	ID, OwnerID, CurrencyID, CategoryID vo.Id
	Name                                string
	PayeeID                             *vo.Id
	TagID                               *vo.Id
	StartDate                           *time.Time
	Current                             DebtTerms
	CreatedAt, UpdatedAt                time.Time
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
	PayeeID, TagID                      *string
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
}

func DebtMatches(d DebtView, categoryID, payeeID, tagID *string, at time.Time) bool {
	if categoryID == nil || *categoryID != d.CategoryID {
		return false
	}
	if d.PayeeID != nil && (payeeID == nil || *payeeID != *d.PayeeID) {
		return false
	}
	if d.TagID != nil && (tagID == nil || *tagID != *d.TagID) {
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
