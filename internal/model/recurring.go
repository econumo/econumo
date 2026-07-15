package model

import (
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

type RecurringSchedule string

const (
	RecurringScheduleWeekly    RecurringSchedule = "weekly"
	RecurringScheduleBiweekly  RecurringSchedule = "biweekly"
	RecurringScheduleMonthly   RecurringSchedule = "monthly"
	RecurringScheduleQuarterly RecurringSchedule = "quarterly"
	RecurringScheduleYearly    RecurringSchedule = "yearly"
)

func ParseRecurringSchedule(s string) (RecurringSchedule, bool) {
	switch RecurringSchedule(s) {
	case RecurringScheduleWeekly, RecurringScheduleBiweekly, RecurringScheduleMonthly,
		RecurringScheduleQuarterly, RecurringScheduleYearly:
		return RecurringSchedule(s), true
	}
	return "", false
}

// NextOccurrence advances from the SCHEDULED date (posting late must not drift
// the schedule). Month-based schedules clamp to the shortest month but return
// to scheduledDay afterwards (31st -> Feb 28 -> Mar 31), which is why the day
// is carried separately instead of being re-read from the current date.
func NextOccurrence(current time.Time, schedule RecurringSchedule, scheduledDay int16) time.Time {
	switch schedule {
	case RecurringScheduleWeekly:
		return current.AddDate(0, 0, 7)
	case RecurringScheduleBiweekly:
		return current.AddDate(0, 0, 14)
	}
	months := 1
	switch schedule {
	case RecurringScheduleQuarterly:
		months = 3
	case RecurringScheduleYearly:
		months = 12
	}
	y, m, _ := current.Date()
	hh, mi, ss := current.Clock()
	first := time.Date(y, m+time.Month(months), 1, hh, mi, ss, 0, current.Location())
	day := int(scheduledDay)
	if last := daysInMonth(first.Year(), first.Month()); day > last {
		day = last
	}
	return time.Date(first.Year(), first.Month(), day, hh, mi, ss, 0, current.Location())
}

func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

type RecurringTransaction struct {
	ID             vo.Id
	UserID         vo.Id
	Type           TransactionType
	AccountID      vo.Id
	AccountRecipID *vo.Id
	Amount         string
	CategoryID     *vo.Id
	PayeeID        *vo.Id
	TagID          *vo.Id
	Description    string
	Schedule       RecurringSchedule
	NextPaymentAt  time.Time
	ScheduledDay   int16
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type RecurringNewState struct {
	ID             vo.Id
	UserID         vo.Id
	Type           TransactionType
	AccountID      vo.Id
	AccountRecipID *vo.Id
	Amount         string
	CategoryID     *vo.Id
	PayeeID        *vo.Id
	TagID          *vo.Id
	Description    string
	Schedule       RecurringSchedule
	NextPaymentAt  time.Time
	ScheduledDay   int16
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func NewRecurringTransaction(s RecurringNewState) *RecurringTransaction {
	rt := recurringFrom(s)
	rt.ScheduledDay = int16(s.NextPaymentAt.Day())
	rt.UpdatedAt = s.CreatedAt
	return rt
}

func RecurringFromState(s RecurringNewState) *RecurringTransaction {
	return recurringFrom(s)
}

func recurringFrom(s RecurringNewState) *RecurringTransaction {
	rt := &RecurringTransaction{
		ID: s.ID, UserID: s.UserID, Type: s.Type, AccountID: s.AccountID,
		AccountRecipID: s.AccountRecipID, Amount: s.Amount,
		CategoryID: s.CategoryID, PayeeID: s.PayeeID, TagID: s.TagID,
		Description: s.Description, Schedule: s.Schedule,
		NextPaymentAt: s.NextPaymentAt, ScheduledDay: s.ScheduledDay,
		CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
	}
	rt.normalize()
	return rt
}

func (rt *RecurringTransaction) Update(s RecurringNewState, now time.Time) {
	rt.Type = s.Type
	rt.AccountID = s.AccountID
	rt.AccountRecipID = s.AccountRecipID
	rt.Amount = s.Amount
	rt.CategoryID = s.CategoryID
	rt.PayeeID = s.PayeeID
	rt.TagID = s.TagID
	rt.Description = s.Description
	rt.Schedule = s.Schedule
	if !s.NextPaymentAt.Equal(rt.NextPaymentAt) {
		rt.ScheduledDay = int16(s.NextPaymentAt.Day())
	}
	rt.NextPaymentAt = s.NextPaymentAt
	rt.normalize()
	rt.UpdatedAt = now
}

func (rt *RecurringTransaction) Advance(now time.Time) {
	rt.NextPaymentAt = NextOccurrence(rt.NextPaymentAt, rt.Schedule, rt.ScheduledDay)
	rt.UpdatedAt = now
}

// Same invariant as Transaction.Update: transfers carry no classifiers,
// non-transfers carry no recipient.
func (rt *RecurringTransaction) normalize() {
	if rt.Type.IsTransfer() {
		rt.CategoryID, rt.PayeeID, rt.TagID = nil, nil, nil
	} else {
		rt.AccountRecipID = nil
	}
}
