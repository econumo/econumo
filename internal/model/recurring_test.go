package model_test

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

func d(s string) time.Time {
	t, err := time.Parse("2006-01-02 15:04:05", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestNextOccurrence(t *testing.T) {
	cases := []struct {
		name         string
		current      string
		schedule     model.RecurringSchedule
		scheduledDay int16
		want         string
	}{
		{"weekly", "2026-07-14 00:00:00", model.RecurringScheduleWeekly, 14, "2026-07-21 00:00:00"},
		{"biweekly", "2026-07-14 00:00:00", model.RecurringScheduleBiweekly, 14, "2026-07-28 00:00:00"},
		{"monthly plain", "2026-07-14 00:00:00", model.RecurringScheduleMonthly, 14, "2026-08-14 00:00:00"},
		{"monthly clamp to feb", "2027-01-31 00:00:00", model.RecurringScheduleMonthly, 31, "2027-02-28 00:00:00"},
		{"monthly clamp leap feb", "2028-01-31 00:00:00", model.RecurringScheduleMonthly, 31, "2028-02-29 00:00:00"},
		{"monthly recovers after clamp", "2027-02-28 00:00:00", model.RecurringScheduleMonthly, 31, "2027-03-31 00:00:00"},
		{"monthly 30 skips feb", "2027-01-30 00:00:00", model.RecurringScheduleMonthly, 30, "2027-02-28 00:00:00"},
		{"monthly year rollover", "2026-12-15 00:00:00", model.RecurringScheduleMonthly, 15, "2027-01-15 00:00:00"},
		{"quarterly", "2026-11-30 00:00:00", model.RecurringScheduleQuarterly, 30, "2027-02-28 00:00:00"},
		{"yearly", "2028-02-29 00:00:00", model.RecurringScheduleYearly, 29, "2029-02-28 00:00:00"},
		{"keeps time of day", "2026-07-14 09:30:00", model.RecurringScheduleWeekly, 14, "2026-07-21 09:30:00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := model.NextOccurrence(d(tc.current), tc.schedule, tc.scheduledDay)
			if want := d(tc.want); !got.Equal(want) {
				t.Fatalf("NextOccurrence(%s, %s, %d) = %s, want %s", tc.current, tc.schedule, tc.scheduledDay, got, want)
			}
		})
	}
}

func TestParseRecurringSchedule(t *testing.T) {
	for _, alias := range []string{"weekly", "biweekly", "monthly", "quarterly", "yearly"} {
		s, ok := model.ParseRecurringSchedule(alias)
		if !ok || string(s) != alias {
			t.Fatalf("ParseRecurringSchedule(%q) = %q, %v", alias, s, ok)
		}
	}
	if _, ok := model.ParseRecurringSchedule("daily"); ok {
		t.Fatal("ParseRecurringSchedule accepted an unknown alias")
	}
}

func TestNewRecurringTransaction_DerivesScheduledDay(t *testing.T) {
	now := d("2026-07-14 10:00:00")
	rt := model.NewRecurringTransaction(model.RecurringNewState{
		ID: vo.NewId(), UserID: vo.NewId(), Type: model.TransactionTypeExpense,
		AccountID: vo.NewId(), Amount: "50", Schedule: model.RecurringScheduleMonthly,
		NextPaymentAt: d("2026-07-31 00:00:00"), CreatedAt: now,
	})
	if rt.ScheduledDay != 31 {
		t.Fatalf("ScheduledDay = %d, want 31", rt.ScheduledDay)
	}
	if !rt.UpdatedAt.Equal(now) {
		t.Fatalf("UpdatedAt = %s, want %s", rt.UpdatedAt, now)
	}
}

func TestRecurringAdvance_UsesScheduledDay(t *testing.T) {
	now := d("2027-03-05 10:00:00")
	rt := model.RecurringFromState(model.RecurringNewState{
		ID: vo.NewId(), UserID: vo.NewId(), Type: model.TransactionTypeExpense,
		AccountID: vo.NewId(), Amount: "50", Schedule: model.RecurringScheduleMonthly,
		NextPaymentAt: d("2027-02-28 00:00:00"), ScheduledDay: 31,
		CreatedAt: d("2027-01-01 00:00:00"), UpdatedAt: d("2027-01-01 00:00:00"),
	})
	rt.Advance(now)
	if want := d("2027-03-31 00:00:00"); !rt.NextPaymentAt.Equal(want) {
		t.Fatalf("NextPaymentAt = %s, want %s", rt.NextPaymentAt, want)
	}
	if !rt.UpdatedAt.Equal(now) {
		t.Fatalf("UpdatedAt not stamped")
	}
}

func TestRecurringUpdate_TransferClearsClassifiers_AndRederivesDay(t *testing.T) {
	now := d("2026-07-14 10:00:00")
	cat := vo.NewId()
	recip := vo.NewId()
	rt := model.NewRecurringTransaction(model.RecurringNewState{
		ID: vo.NewId(), UserID: vo.NewId(), Type: model.TransactionTypeExpense,
		AccountID: vo.NewId(), Amount: "50", CategoryID: &cat,
		Schedule: model.RecurringScheduleMonthly, NextPaymentAt: d("2026-07-31 00:00:00"), CreatedAt: now,
	})
	later := d("2026-07-15 10:00:00")
	rt.Update(model.RecurringNewState{
		ID: rt.ID, UserID: rt.UserID, Type: model.TransactionTypeTransfer,
		AccountID: rt.AccountID, AccountRecipID: &recip, Amount: "60",
		CategoryID: &cat, Schedule: model.RecurringScheduleWeekly,
		NextPaymentAt: d("2026-08-05 00:00:00"),
	}, later)
	if rt.CategoryID != nil {
		t.Fatal("transfer must clear CategoryID")
	}
	if rt.AccountRecipID == nil || !rt.AccountRecipID.Equal(recip) {
		t.Fatal("transfer must keep AccountRecipID")
	}
	if rt.ScheduledDay != 5 {
		t.Fatalf("ScheduledDay = %d, want 5 (re-derived)", rt.ScheduledDay)
	}
	if !rt.UpdatedAt.Equal(later) {
		t.Fatal("UpdatedAt not stamped")
	}
}

func TestRecurringUpdate_SameNextPaymentAt_PreservesScheduledDay(t *testing.T) {
	created := d("2027-01-01 10:00:00")
	rt := model.NewRecurringTransaction(model.RecurringNewState{
		ID: vo.NewId(), UserID: vo.NewId(), Type: model.TransactionTypeExpense,
		AccountID: vo.NewId(), Amount: "50", Schedule: model.RecurringScheduleMonthly,
		NextPaymentAt: d("2027-01-31 00:00:00"), CreatedAt: created,
	})
	if rt.ScheduledDay != 31 {
		t.Fatalf("ScheduledDay = %d, want 31", rt.ScheduledDay)
	}

	rt.Advance(d("2027-02-01 10:00:00"))
	if want := d("2027-02-28 00:00:00"); !rt.NextPaymentAt.Equal(want) {
		t.Fatalf("NextPaymentAt = %s, want %s (clamped)", rt.NextPaymentAt, want)
	}
	if rt.ScheduledDay != 31 {
		t.Fatalf("ScheduledDay = %d, want 31 (unchanged by Advance)", rt.ScheduledDay)
	}

	later := d("2027-02-02 10:00:00")
	rt.Update(model.RecurringNewState{
		ID: rt.ID, UserID: rt.UserID, Type: rt.Type,
		AccountID: rt.AccountID, Amount: "75",
		Schedule: model.RecurringScheduleMonthly, NextPaymentAt: rt.NextPaymentAt,
	}, later)
	if rt.Amount != "75" {
		t.Fatalf("Amount = %q, want 75", rt.Amount)
	}
	if rt.ScheduledDay != 31 {
		t.Fatalf("ScheduledDay = %d, want 31 (preserved when NextPaymentAt unchanged)", rt.ScheduledDay)
	}

	rt.Advance(d("2027-03-01 10:00:00"))
	if want := d("2027-03-31 00:00:00"); !rt.NextPaymentAt.Equal(want) {
		t.Fatalf("NextPaymentAt after clamp recovery = %s, want %s", rt.NextPaymentAt, want)
	}
}
