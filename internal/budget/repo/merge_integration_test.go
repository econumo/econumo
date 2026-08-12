package repo_test

// Integration coverage for the three queries a classification merge adds:
// ListElementsByExternal, RepointElement and ListLimitsByElement. The algorithm
// itself is unit-tested in internal/budget/merge_test.go; what needs a real
// database is the SQL — especially that period matching survives the legacy
// RFC3339 rows, which no Go-written fixture can produce any more.

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

const (
	mergeSrcExternal = "cccc1111-0000-0000-0000-0000000000c1"
	mergeDstExternal = "cccc2222-0000-0000-0000-0000000000c2"
)

func saveMergeElement(t *testing.T, repo elementSaver, ctx context.Context, id, externalID string, folder *vo.Id) *model.BudgetElement {
	t.Helper()
	e := &model.BudgetElement{
		ID: vo.MustParseId(id), BudgetID: vo.MustParseId(budgetID),
		ExternalID: vo.MustParseId(externalID), Type: model.ElementCategory,
		FolderID: folder, CreatedAt: fixedTime, UpdatedAt: fixedTime,
	}
	if err := repo.SaveElement(ctx, e); err != nil {
		t.Fatalf("SaveElement %s: %v", id, err)
	}
	return e
}

// elementSaver keeps the helper honest about what it needs from the repo.
type elementSaver interface {
	SaveElement(ctx context.Context, e *model.BudgetElement) error
}

func TestBudgetRepo_ListElementsByExternal_SpansBudgets(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	saveBudget(t, repo, ctx)

	saveMergeElement(t, repo, ctx, "e1111111-0000-0000-0000-0000000000e1", mergeSrcExternal, nil)
	saveMergeElement(t, repo, ctx, "e2222222-0000-0000-0000-0000000000e2", mergeDstExternal, nil)

	got, err := repo.ListElementsByExternal(ctx, vo.MustParseId(mergeSrcExternal))
	if err != nil {
		t.Fatalf("ListElementsByExternal: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d elements, want 1", len(got))
	}
	if got[0].ExternalID.String() != mergeSrcExternal {
		t.Errorf("external id = %s, want %s", got[0].ExternalID, mergeSrcExternal)
	}
}

// TestBudgetRepo_RepointElement covers what SaveElement deliberately cannot do:
// its upsert leaves external_id alone, so re-pointing needs its own statement.
func TestBudgetRepo_RepointElement(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	saveBudget(t, repo, ctx)

	const elemID = "e3333333-0000-0000-0000-0000000000e3"
	saveMergeElement(t, repo, ctx, elemID, mergeSrcExternal, nil)
	later := fixedTime.Add(48 * time.Hour)

	if err := repo.RepointElement(ctx, vo.MustParseId(elemID), vo.MustParseId(mergeDstExternal), later); err != nil {
		t.Fatalf("RepointElement: %v", err)
	}

	got, err := repo.GetElementByExternal(ctx, vo.MustParseId(budgetID), vo.MustParseId(mergeDstExternal))
	if err != nil {
		t.Fatalf("target lookup after repoint: %v", err)
	}
	if got.ID.String() != elemID {
		t.Errorf("element id = %s, want %s (the same row, re-pointed)", got.ID, elemID)
	}
	if _, err := repo.GetElementByExternal(ctx, vo.MustParseId(budgetID), vo.MustParseId(mergeSrcExternal)); err == nil {
		t.Error("source external id still resolves to an element")
	}
}

func TestBudgetRepo_ListLimitsByElement_ReturnsEveryPeriod(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	saveBudget(t, repo, ctx)

	const elemID = "e4444444-0000-0000-0000-0000000000e4"
	saveMergeElement(t, repo, ctx, elemID, mergeSrcExternal, nil)
	for _, p := range []time.Time{aprPeriod, mayPeriod} {
		if err := repo.SaveLimit(ctx, &model.BudgetElementLimit{
			ID: vo.NewId(), ElementID: vo.MustParseId(elemID), Period: p,
			Amount: vo.NewDecimal("100"), CreatedAt: fixedTime, UpdatedAt: fixedTime,
		}); err != nil {
			t.Fatalf("SaveLimit %v: %v", p, err)
		}
	}

	got, err := repo.ListLimitsByElement(ctx, vo.MustParseId(elemID))
	if err != nil {
		t.Fatalf("ListLimitsByElement: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d limits, want 2 (every period, no filter)", len(got))
	}
}

// TestBudgetRepo_GetLimit_MatchesAcrossPeriodFormats is the guard the merge
// depends on. The 20260811 migration should make mixed formats unreachable, but
// every limit the current code writes shares one format — so without a
// deliberately raw legacy row, an implementation matching periods by string
// equality passes the whole suite and fails only on real user data.
func TestBudgetRepo_GetLimit_MatchesAcrossPeriodFormats(t *testing.T) {
	repo, db := newRepo(t)
	ctx := context.Background()
	saveBudget(t, repo, ctx)

	const elemID = "e5555555-0000-0000-0000-0000000000e5"
	saveMergeElement(t, repo, ctx, elemID, mergeSrcExternal, nil)

	// Write the legacy RFC3339 form directly; the repo can no longer produce it.
	// Rebind because raw test SQL is authored with sqlite '?' placeholders.
	// PostgreSQL is immune to this class of bug anyway (period is TIMESTAMP(0),
	// so the textual variance is not representable) — running it on both engines
	// just proves the lookup stays correct either way.
	limitID := vo.NewId().String()
	db.Exec(t, db.Rebind(`INSERT INTO budgets_elements_limits (id, element_id, period, created_at, updated_at, amount)
		VALUES (?, ?, ?, ?, ?, ?)`),
		limitID, elemID, "2024-04-01T00:00:00Z", fixedTime, fixedTime, "250")

	got, err := repo.GetLimit(ctx, vo.MustParseId(elemID), aprPeriod)
	if err != nil {
		t.Fatalf("GetLimit did not match the legacy period format: %v", err)
	}
	if got.Amount.String() != "250" {
		t.Errorf("amount = %s, want 250", got.Amount)
	}

	all, err := repo.ListLimitsByElement(ctx, vo.MustParseId(elemID))
	if err != nil {
		t.Fatalf("ListLimitsByElement: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("got %d limits, want 1", len(all))
	}
}
