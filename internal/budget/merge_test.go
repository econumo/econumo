package budget

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/sortkey"
	"github.com/econumo/econumo/internal/shared/vo"
)

type mergeClock struct{ t time.Time }

func (c mergeClock) Now() time.Time { return c.t }

// fakeElements/fakeLimits are in-memory ElementStore/LimitStore halves. Only the
// methods MergeElements touches are real; the rest satisfy the interface.
type fakeElements struct {
	byID     map[vo.Id]*model.BudgetElement
	nextID   int
	repoints int
	deletes  int
}

func newFakeElements() *fakeElements {
	return &fakeElements{byID: map[vo.Id]*model.BudgetElement{}}
}

func (f *fakeElements) NextIdentity() vo.Id { return vo.NewId() }

func (f *fakeElements) ListElements(ctx context.Context, budgetID vo.Id) ([]*model.BudgetElement, error) {
	var out []*model.BudgetElement
	for _, e := range f.byID {
		if e.BudgetID.Equal(budgetID) {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeElements) ListElementsByExternal(ctx context.Context, externalID vo.Id) ([]*model.BudgetElement, error) {
	var out []*model.BudgetElement
	for _, e := range f.byID {
		if e.ExternalID.Equal(externalID) {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeElements) GetElement(ctx context.Context, id vo.Id) (*model.BudgetElement, error) {
	if e, ok := f.byID[id]; ok {
		return e, nil
	}
	return nil, errs.NewNotFound("BudgetElement not found")
}

func (f *fakeElements) GetElementByExternal(ctx context.Context, budgetID, externalID vo.Id) (*model.BudgetElement, error) {
	for _, e := range f.byID {
		if e.BudgetID.Equal(budgetID) && e.ExternalID.Equal(externalID) {
			return e, nil
		}
	}
	return nil, errs.NewNotFound("BudgetElement not found")
}

func (f *fakeElements) SaveElement(ctx context.Context, e *model.BudgetElement) error {
	f.byID[e.ID] = e
	return nil
}

func (f *fakeElements) RepointElement(ctx context.Context, id, externalID vo.Id, updatedAt time.Time) error {
	e, ok := f.byID[id]
	if !ok {
		return errs.NewNotFound("BudgetElement not found")
	}
	e.ExternalID = externalID
	e.UpdatedAt = updatedAt
	f.repoints++
	return nil
}

func (f *fakeElements) DeleteElement(ctx context.Context, id vo.Id) error {
	delete(f.byID, id)
	f.deletes++
	return nil
}

type fakeLimits struct {
	rows []*model.BudgetElementLimit
}

func (f *fakeLimits) NextIdentity() vo.Id { return vo.NewId() }

func (f *fakeLimits) ListLimitsForPeriod(ctx context.Context, budgetID vo.Id, period time.Time) ([]*model.BudgetElementLimit, error) {
	return nil, nil
}

func (f *fakeLimits) ListLimitsFrom(ctx context.Context, budgetID vo.Id, from time.Time) ([]*model.BudgetElementLimit, error) {
	return nil, nil
}

func (f *fakeLimits) ListLimitsByElement(ctx context.Context, elementID vo.Id) ([]*model.BudgetElementLimit, error) {
	var out []*model.BudgetElementLimit
	for _, l := range f.rows {
		if l.ElementID.Equal(elementID) {
			out = append(out, l)
		}
	}
	return out, nil
}

func (f *fakeLimits) GetLimit(ctx context.Context, elementID vo.Id, period time.Time) (*model.BudgetElementLimit, error) {
	for _, l := range f.rows {
		if l.ElementID.Equal(elementID) && l.Period.Equal(period) {
			return l, nil
		}
	}
	return nil, errs.NewNotFound("BudgetElementLimit not found")
}

func (f *fakeLimits) SaveLimit(ctx context.Context, l *model.BudgetElementLimit) error {
	for i, existing := range f.rows {
		if existing.ID.Equal(l.ID) {
			f.rows[i] = l
			return nil
		}
	}
	f.rows = append(f.rows, l)
	return nil
}

func (f *fakeLimits) DeleteLimit(ctx context.Context, id vo.Id) error { return nil }

func (f *fakeLimits) DeleteLimitsByBudget(ctx context.Context, budgetID vo.Id) error { return nil }

type mergeEnv struct {
	svc      *MergeService
	elements *fakeElements
	limits   *fakeLimits
	now      time.Time
}

func newMergeEnv() *mergeEnv {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	els, lims := newFakeElements(), &fakeLimits{}
	return &mergeEnv{
		svc:      NewMergeService(els, lims, mergeClock{t: now}),
		elements: els,
		limits:   lims,
		now:      now,
	}
}

func (e *mergeEnv) addElement(budgetID, externalID vo.Id, folder *vo.Id, key string) *model.BudgetElement {
	el := &model.BudgetElement{
		ID: vo.NewId(), BudgetID: budgetID, ExternalID: externalID,
		Type: model.ElementCategory, FolderID: folder, SortKey: sortkey.Key(key),
	}
	e.elements.byID[el.ID] = el
	return el
}

func (e *mergeEnv) addLimit(elementID vo.Id, month time.Month, amount string) {
	e.limits.rows = append(e.limits.rows, &model.BudgetElementLimit{
		ID:        vo.NewId(),
		ElementID: elementID,
		Period:    time.Date(2026, month, 1, 0, 0, 0, 0, time.UTC),
		Amount:    vo.NewDecimal(amount),
	})
}

func (e *mergeEnv) limitFor(t *testing.T, elementID vo.Id, month time.Month) string {
	t.Helper()
	period := time.Date(2026, month, 1, 0, 0, 0, 0, time.UTC)
	l, err := e.limits.GetLimit(context.Background(), elementID, period)
	if err != nil {
		t.Fatalf("no limit for element %s in %s", elementID, month)
	}
	return l.Amount.String()
}

// TestMergeElements_NoTargetElement_RepointsInPlace pins the branch that keeps
// the budget row where the user put it: re-pointing preserves the element's id,
// folder and sort key, which delete-and-recreate would lose.
func TestMergeElements_NoTargetElement_RepointsInPlace(t *testing.T) {
	env := newMergeEnv()
	budgetID, srcCat, dstCat := vo.NewId(), vo.NewId(), vo.NewId()
	folder := vo.NewId()
	src := env.addElement(budgetID, srcCat, &folder, "c5")
	env.addLimit(src.ID, time.August, "200")

	if err := env.svc.MergeElements(context.Background(), srcCat, dstCat); err != nil {
		t.Fatalf("MergeElements: %v", err)
	}

	if env.elements.repoints != 1 {
		t.Errorf("repoints = %d, want 1", env.elements.repoints)
	}
	if env.elements.deletes != 0 {
		t.Errorf("deletes = %d, want 0 (nothing to collapse)", env.elements.deletes)
	}
	got, err := env.elements.GetElementByExternal(context.Background(), budgetID, dstCat)
	if err != nil {
		t.Fatalf("target element missing after merge: %v", err)
	}
	if !got.ID.Equal(src.ID) {
		t.Errorf("element id = %s, want %s (re-pointed, not recreated)", got.ID, src.ID)
	}
	if got.FolderID == nil || !got.FolderID.Equal(folder) {
		t.Error("folder lost by the merge")
	}
	if got.SortKey != sortkey.Key("c5") {
		t.Errorf("sort key = %q, want c5 (position lost)", got.SortKey)
	}
	if env.limitFor(t, src.ID, time.August) != "200" {
		t.Error("limit lost by the merge")
	}
	if !got.UpdatedAt.Equal(env.now) {
		t.Errorf("updatedAt = %v, want %v", got.UpdatedAt, env.now)
	}
}

// TestMergeElements_BothElements_SumsEveryPeriod is the core rule: spending is
// being combined, so the limits must be too — and for every month, not just the
// current one.
func TestMergeElements_BothElements_SumsEveryPeriod(t *testing.T) {
	env := newMergeEnv()
	budgetID, srcCat, dstCat := vo.NewId(), vo.NewId(), vo.NewId()
	src := env.addElement(budgetID, srcCat, nil, "c1")
	dst := env.addElement(budgetID, dstCat, nil, "c2")

	env.addLimit(src.ID, time.August, "200") // both
	env.addLimit(dst.ID, time.August, "300")
	env.addLimit(src.ID, time.September, "500") // source only
	env.addLimit(dst.ID, time.October, "700")   // target only
	env.addLimit(src.ID, time.December, "900")  // future-dated, still moves

	if err := env.svc.MergeElements(context.Background(), srcCat, dstCat); err != nil {
		t.Fatalf("MergeElements: %v", err)
	}

	for _, c := range []struct {
		month time.Month
		want  string
	}{
		{time.August, "500"},    // summed
		{time.September, "500"}, // carried over
		{time.October, "700"},   // untouched
		{time.December, "900"},  // future carried over
	} {
		if got := env.limitFor(t, dst.ID, c.month); got != c.want {
			t.Errorf("%s limit = %s, want %s", c.month, got, c.want)
		}
	}
	if _, ok := env.elements.byID[src.ID]; ok {
		t.Error("source element survived the merge")
	}
	if env.elements.repoints != 0 {
		t.Errorf("repoints = %d, want 0 (the target already had an element)", env.elements.repoints)
	}
}

// TestMergeElements_SpansEveryBudget covers the scope choice: elements are found
// by external id alone, so a classification used in several budgets — including
// ones shared with connected users — is merged in all of them.
func TestMergeElements_SpansEveryBudget(t *testing.T) {
	env := newMergeEnv()
	budgetA, budgetB := vo.NewId(), vo.NewId()
	srcCat, dstCat := vo.NewId(), vo.NewId()

	srcA := env.addElement(budgetA, srcCat, nil, "c1") // budget A: both present -> sum
	dstA := env.addElement(budgetA, dstCat, nil, "c2")
	env.addLimit(srcA.ID, time.August, "100")
	env.addLimit(dstA.ID, time.August, "50")

	srcB := env.addElement(budgetB, srcCat, nil, "c1") // budget B: target absent -> repoint
	env.addLimit(srcB.ID, time.August, "400")

	if err := env.svc.MergeElements(context.Background(), srcCat, dstCat); err != nil {
		t.Fatalf("MergeElements: %v", err)
	}

	if got := env.limitFor(t, dstA.ID, time.August); got != "150" {
		t.Errorf("budget A August = %s, want 150", got)
	}
	inB, err := env.elements.GetElementByExternal(context.Background(), budgetB, dstCat)
	if err != nil {
		t.Fatalf("budget B was not merged: %v", err)
	}
	if !inB.ID.Equal(srcB.ID) {
		t.Error("budget B element was recreated rather than re-pointed")
	}
	if env.limitFor(t, srcB.ID, time.August) != "400" {
		t.Error("budget B limit lost")
	}
}

// TestMergeElements_NoElements_IsANoop covers the common case: a classification
// that never appeared in any budget (e.g. a payee-like tag with no limits).
func TestMergeElements_NoElements_IsANoop(t *testing.T) {
	env := newMergeEnv()

	if err := env.svc.MergeElements(context.Background(), vo.NewId(), vo.NewId()); err != nil {
		t.Fatalf("MergeElements: %v", err)
	}
	if env.elements.repoints != 0 || env.elements.deletes != 0 {
		t.Errorf("touched storage: repoints=%d deletes=%d", env.elements.repoints, env.elements.deletes)
	}
}
