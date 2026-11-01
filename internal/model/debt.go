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
	WholeCategory                       bool       // no payee, no tag, no start date
	StartMonth, EndMonth                time.Time  // planning bounds (first-of-month)
	PaidOff                             bool
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
