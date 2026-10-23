package budget

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type firstTxStub struct {
	spendingStub
	first *time.Time
}

func (f *firstTxStub) FirstTransactionDate(context.Context, []vo.Id) (*time.Time, error) {
	return f.first, nil
}

func reportSvc(first *time.Time) *Service {
	return &Service{
		read:  &firstTxStub{first: first},
		clock: frozenClock{t: time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)},
	}
}

func month(y int, m time.Month) time.Time { return time.Date(y, m, 1, 0, 0, 0, 0, time.UTC) }

var someAccounts = []vo.Id{vo.MustParseId("aaaa0000-0000-7000-8000-000000000001")}

func TestReportWindow_Defaults(t *testing.T) {
	first := time.Date(2026, 2, 20, 9, 0, 0, 0, time.UTC)
	w, err := reportSvc(&first).resolveReportWindow(context.Background(), "", "", someAccounts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !w.from.Equal(month(2026, 2)) || w.months != 4 {
		t.Fatalf("got %v/%d, want 2026-02/4", w.from, w.months)
	}
}

func TestReportWindow_NoTransactionsIsCurrentMonth(t *testing.T) {
	w, err := reportSvc(nil).resolveReportWindow(context.Background(), "", "", someAccounts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !w.from.Equal(month(2026, 5)) || w.months != 1 {
		t.Fatalf("got %v/%d, want 2026-05/1", w.from, w.months)
	}
}

func TestReportWindow_SnapsAndIsInclusive(t *testing.T) {
	w, err := reportSvc(nil).resolveReportWindow(context.Background(), "2026-01-17", "2026-03-31 12:00:00", someAccounts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !w.from.Equal(month(2026, 1)) || w.months != 3 {
		t.Fatalf("got %v/%d, want 2026-01/3", w.from, w.months)
	}
}

func TestReportWindow_FromAfterToCollapses(t *testing.T) {
	w, err := reportSvc(nil).resolveReportWindow(context.Background(), "2026-08-01", "2026-03-01", someAccounts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !w.from.Equal(month(2026, 3)) || w.months != 1 {
		t.Fatalf("got %v/%d, want 2026-03/1", w.from, w.months)
	}
}

func TestReportWindow_EndCap(t *testing.T) {
	cap := month(2026, 3)
	w, err := reportSvc(nil).resolveReportWindow(context.Background(), "2026-01-01", "", someAccounts, &cap)
	if err != nil {
		t.Fatal(err)
	}
	if !w.from.Equal(month(2026, 1)) || w.months != 3 {
		t.Fatalf("got %v/%d, want 2026-01/3", w.from, w.months)
	}
}

func fieldKey(t *testing.T, err error) string {
	t.Helper()
	var v *errs.ValidationError
	if !errors.As(err, &v) {
		t.Fatalf("want validation error, got %v", err)
	}
	return v.Fields[0].Key
}

func TestReportWindow_TooManyMonths(t *testing.T) {
	s := reportSvc(nil)
	if _, err := s.resolveReportWindow(context.Background(), "2000-01-01", "2020-01-01", someAccounts, nil); err == nil {
		t.Fatal("241 months must fail")
	} else if k := fieldKey(t, err); k != "to" {
		t.Fatalf("field = %q, want to", k)
	}
	if _, err := s.resolveReportWindow(context.Background(), "2000-01-01", "2019-12-01", someAccounts, nil); err != nil {
		t.Fatalf("240 months must pass: %v", err)
	}
}

func TestReportWindow_MalformedDates(t *testing.T) {
	s := reportSvc(nil)
	_, err := s.resolveReportWindow(context.Background(), "nope", "", someAccounts, nil)
	if k := fieldKey(t, err); k != "from" {
		t.Fatalf("field = %q, want from", k)
	}
	_, err = s.resolveReportWindow(context.Background(), "", "nope", someAccounts, nil)
	if k := fieldKey(t, err); k != "to" {
		t.Fatalf("field = %q, want to", k)
	}
}

func TestParseFlowGroupBy(t *testing.T) {
	for in, want := range map[string]string{"": "category", "category": "category", "tag": "tag", "payee": "payee"} {
		got, err := parseFlowGroupBy(in)
		if err != nil || got != want {
			t.Errorf("%q -> %q, %v; want %q", in, got, err, want)
		}
	}
	_, err := parseFlowGroupBy("label")
	if k := fieldKey(t, err); k != "groupBy" {
		t.Fatalf("field = %q, want groupBy", k)
	}
}
