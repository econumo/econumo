package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	recurringrepo "github.com/econumo/econumo/internal/recurring/repo"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	userA    = "0197c000-0000-7000-8000-00000000000a"
	accountA = "0197c000-0000-7000-8000-00000000000b"
	accountB = "0197c000-0000-7000-8000-00000000000c"
	rtA      = "0197c000-0000-7000-8000-00000000000d"
	label1   = "0197c000-0000-7000-8000-0000000000a1"
	label2   = "0197c000-0000-7000-8000-0000000000a2"
	label3   = "0197c000-0000-7000-8000-0000000000a3"
)

var fixedTime = time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)

func newRepo(t *testing.T) (*recurringrepo.Repo, *fixture.Builder) {
	t.Helper()
	db := dbtest.New(t)
	return recurringrepo.NewRepo(db.Engine, db.TX), fixture.New(t, db)
}

func seed(t *testing.T, f *fixture.Builder) {
	t.Helper()
	f.User(fixture.User{ID: userA})
	f.Account(fixture.Account{ID: accountA, UserID: userA})
	f.Account(fixture.Account{ID: accountB, UserID: userA})
}

func template(id string) *model.RecurringTransaction {
	return model.NewRecurringTransaction(model.RecurringNewState{
		ID: vo.MustParseId(id), UserID: vo.MustParseId(userA),
		Type: model.TransactionTypeExpense, AccountID: vo.MustParseId(accountA),
		Amount: "50.5", Description: "rent",
		Schedule:      model.RecurringScheduleMonthly,
		NextPaymentAt: time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC),
		CreatedAt:     fixedTime,
	})
}

func TestRecurringRepo_SaveGetRoundTrip(t *testing.T) {
	repo, f := newRepo(t)
	ctx := context.Background()
	seed(t, f)

	if err := repo.Save(ctx, template(rtA)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.GetByID(ctx, vo.MustParseId(rtA))
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if vo.NewDecimal(got.Amount).String() != vo.NewDecimal("50.5").String() ||
		got.Schedule != model.RecurringScheduleMonthly || got.ScheduledDay != 31 {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	if !got.NextPaymentAt.Equal(time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("NextPaymentAt = %s", got.NextPaymentAt)
	}
}

func TestRecurringRepo_UpsertUpdates(t *testing.T) {
	repo, f := newRepo(t)
	ctx := context.Background()
	seed(t, f)

	rt := template(rtA)
	if err := repo.Save(ctx, rt); err != nil {
		t.Fatalf("Save: %v", err)
	}
	rt.Advance(fixedTime.Add(time.Hour))
	if err := repo.Save(ctx, rt); err != nil {
		t.Fatalf("re-Save: %v", err)
	}
	got, _ := repo.GetByID(ctx, vo.MustParseId(rtA))
	if !got.NextPaymentAt.Equal(time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("NextPaymentAt after advance = %s, want 2026-08-31", got.NextPaymentAt)
	}
}

func TestRecurringRepo_ListByAccountIDs(t *testing.T) {
	repo, f := newRepo(t)
	ctx := context.Background()
	seed(t, f)

	rtB := "0197c000-0000-7000-8000-00000000000e"
	a := template(rtA)
	b := template(rtB)
	b.AccountID = vo.MustParseId(accountB)
	for _, rt := range []*model.RecurringTransaction{a, b} {
		if err := repo.Save(ctx, rt); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	got, err := repo.ListByAccountIDs(ctx, []vo.Id{vo.MustParseId(accountA)})
	if err != nil || len(got) != 1 || got[0].ID.String() != rtA {
		t.Fatalf("ListByAccountIDs(accountA) = %v items, err %v", len(got), err)
	}
	both, _ := repo.ListByAccountIDs(ctx, []vo.Id{vo.MustParseId(accountA), vo.MustParseId(accountB)})
	if len(both) != 2 {
		t.Fatalf("ListByAccountIDs(both) = %d items, want 2", len(both))
	}
	none, err := repo.ListByAccountIDs(ctx, nil)
	if err != nil || len(none) != 0 {
		t.Fatalf("empty id list must return empty slice, no error")
	}
}

func TestRecurringRepo_Delete_AndGetMissing(t *testing.T) {
	repo, f := newRepo(t)
	ctx := context.Background()
	seed(t, f)

	if err := repo.Save(ctx, template(rtA)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := repo.Delete(ctx, vo.MustParseId(rtA)); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.GetByID(ctx, vo.MustParseId(rtA)); err == nil {
		t.Fatal("GetByID after delete must return not-found")
	}
}

func equalOrdered(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestRecurringRepo_ReplaceLabels_RoundTrip(t *testing.T) {
	repo, f := newRepo(t)
	ctx := context.Background()
	seed(t, f)
	f.Label(fixture.Label{ID: label1, UserID: userA})
	f.Label(fixture.Label{ID: label2, UserID: userA})
	if err := repo.Save(ctx, template(rtA)); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Inserted in DESCENDING label id order: if LabelsByRecurringIDs merely
	// reflected insertion/scan order instead of "ORDER BY ... label_id", this
	// would come back [label2, label1] and the ordered assertion below would
	// catch it.
	if err := repo.ReplaceLabels(ctx, vo.MustParseId(rtA), []vo.Id{vo.MustParseId(label2), vo.MustParseId(label1)}); err != nil {
		t.Fatalf("ReplaceLabels: %v", err)
	}

	got, err := repo.LabelsByRecurringIDs(ctx, []vo.Id{vo.MustParseId(rtA)})
	if err != nil {
		t.Fatalf("LabelsByRecurringIDs: %v", err)
	}
	if !equalOrdered(got[rtA], []string{label1, label2}) {
		t.Fatalf("labels = %v, want [%s %s] in ascending label_id order", got[rtA], label1, label2)
	}
}

// TestRecurringRepo_ReplaceLabels_Idempotent exercises the delete-then-insert
// contract directly: calling ReplaceLabels twice with the SAME set must not
// error (would fail on a naive INSERT without delete-first) and must not
// duplicate the pair (would inflate the label count on a second call).
func TestRecurringRepo_ReplaceLabels_Idempotent(t *testing.T) {
	repo, f := newRepo(t)
	ctx := context.Background()
	seed(t, f)
	f.Label(fixture.Label{ID: label1, UserID: userA})
	if err := repo.Save(ctx, template(rtA)); err != nil {
		t.Fatalf("Save: %v", err)
	}

	ids := []vo.Id{vo.MustParseId(label1)}
	if err := repo.ReplaceLabels(ctx, vo.MustParseId(rtA), ids); err != nil {
		t.Fatalf("ReplaceLabels (1st): %v", err)
	}
	if err := repo.ReplaceLabels(ctx, vo.MustParseId(rtA), ids); err != nil {
		t.Fatalf("ReplaceLabels (2nd, re-save): %v", err)
	}

	got, err := repo.LabelsByRecurringIDs(ctx, []vo.Id{vo.MustParseId(rtA)})
	if err != nil {
		t.Fatalf("LabelsByRecurringIDs: %v", err)
	}
	if !equalOrdered(got[rtA], []string{label1}) {
		t.Fatalf("labels = %v, want exactly one [%s] (no duplication)", got[rtA], label1)
	}
}

func TestRecurringRepo_ReplaceLabels_ClearsOnEmpty(t *testing.T) {
	repo, f := newRepo(t)
	ctx := context.Background()
	seed(t, f)
	f.Label(fixture.Label{ID: label1, UserID: userA})
	if err := repo.Save(ctx, template(rtA)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := repo.ReplaceLabels(ctx, vo.MustParseId(rtA), []vo.Id{vo.MustParseId(label1)}); err != nil {
		t.Fatalf("ReplaceLabels (set): %v", err)
	}
	if err := repo.ReplaceLabels(ctx, vo.MustParseId(rtA), nil); err != nil {
		t.Fatalf("ReplaceLabels (clear): %v", err)
	}

	got, err := repo.LabelsByRecurringIDs(ctx, []vo.Id{vo.MustParseId(rtA)})
	if err != nil {
		t.Fatalf("LabelsByRecurringIDs: %v", err)
	}
	if len(got[rtA]) != 0 {
		t.Fatalf("labels after clear = %v, want none", got[rtA])
	}
}

func TestRecurringRepo_LabelsByRecurringIDs_MultipleTemplates(t *testing.T) {
	repo, f := newRepo(t)
	ctx := context.Background()
	seed(t, f)
	f.Label(fixture.Label{ID: label1, UserID: userA})
	f.Label(fixture.Label{ID: label2, UserID: userA})
	f.Label(fixture.Label{ID: label3, UserID: userA})
	rtB := "0197c000-0000-7000-8000-00000000000e"
	rtC := "0197c000-0000-7000-8000-00000000000f"
	for _, id := range []string{rtA, rtB, rtC} {
		if err := repo.Save(ctx, template(id)); err != nil {
			t.Fatalf("Save %s: %v", id, err)
		}
	}
	// rtA inserted in DESCENDING label id order, same rationale as
	// TestRecurringRepo_ReplaceLabels_RoundTrip: the ordered assertion below
	// only passes if the query orders by label_id rather than reflecting
	// insertion/scan order.
	if err := repo.ReplaceLabels(ctx, vo.MustParseId(rtA), []vo.Id{vo.MustParseId(label2), vo.MustParseId(label1)}); err != nil {
		t.Fatalf("ReplaceLabels rtA: %v", err)
	}
	if err := repo.ReplaceLabels(ctx, vo.MustParseId(rtB), []vo.Id{vo.MustParseId(label3)}); err != nil {
		t.Fatalf("ReplaceLabels rtB: %v", err)
	}
	// rtC gets no labels at all -- must be absent from the result map, not a
	// present-but-empty entry (the caller ranges over the map).

	got, err := repo.LabelsByRecurringIDs(ctx, []vo.Id{vo.MustParseId(rtA), vo.MustParseId(rtB), vo.MustParseId(rtC)})
	if err != nil {
		t.Fatalf("LabelsByRecurringIDs: %v", err)
	}
	if !equalOrdered(got[rtA], []string{label1, label2}) {
		t.Fatalf("rtA labels = %v, want [%s %s] in ascending label_id order", got[rtA], label1, label2)
	}
	if !equalOrdered(got[rtB], []string{label3}) {
		t.Fatalf("rtB labels = %v, want [%s]", got[rtB], label3)
	}
	if _, ok := got[rtC]; ok {
		t.Fatalf("rtC should have no entry, got %v", got[rtC])
	}
}

func TestRecurringRepo_LabelsByRecurringIDs_EmptyIDs(t *testing.T) {
	repo, _ := newRepo(t)
	got, err := repo.LabelsByRecurringIDs(context.Background(), nil)
	if err != nil || got != nil {
		t.Fatalf("empty ids should yield nil,nil (guards the IN() syntax error on PostgreSQL); got %v, %v", got, err)
	}
}
