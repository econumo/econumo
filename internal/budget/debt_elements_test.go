package budget_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

type fakeDebts struct{ views []model.DebtView }

func (f *fakeDebts) Views(_ context.Context, ownerIDs []vo.Id) ([]model.DebtView, error) {
	owners := map[string]bool{}
	for _, id := range ownerIDs {
		owners[id.String()] = true
	}
	var out []model.DebtView
	for _, v := range f.views {
		if owners[v.OwnerID] {
			out = append(out, v)
		}
	}
	return out, nil
}

func (f *fakeDebts) Sync(context.Context, []vo.Id) error { return nil }

func (f *fakeDebts) Timelines(context.Context, model.DebtTimelineRequest) (map[string]model.DebtTimeline, error) {
	return map[string]model.DebtTimeline{}, nil
}

func (f *fakeDebts) remove(id string) {
	kept := f.views[:0]
	for _, v := range f.views {
		if v.ID != id {
			kept = append(kept, v)
		}
	}
	f.views = kept
}

type debtHarness struct {
	*commentHarness
	debts *fakeDebts
	eur   string
}

func newDebtHarness(t *testing.T) *debtHarness {
	t.Helper()
	h := newCommentHarness(t)
	d := &fakeDebts{}
	h.svc.SetDebts(d)
	eur := h.f.Currency(fixture.Currency{Code: "EUR", Symbol: "€", Name: "Euro"})
	return &debtHarness{commentHarness: h, debts: d, eur: eur}
}

func month(y int, m time.Month) time.Time { return time.Date(y, m, 1, 0, 0, 0, 0, time.UTC) }

// addDebt registers a debt of owner, plannable from May to August 2026.
func (h *debtHarness) addDebt(owner vo.Id, currencyID string) string {
	id := vo.NewId().String()
	h.debts.views = append(h.debts.views, model.DebtView{
		ID: id, OwnerID: owner.String(), CurrencyID: currencyID, CategoryID: vo.NewId().String(), Name: "Loan",
		StartMonth: month(2026, time.May), EndMonth: month(2026, time.August),
	})
	return id
}

func (h *debtHarness) sync(t *testing.T) {
	t.Helper()
	if err := h.svc.SyncElementsForTest(h.ctx, h.budgetID); err != nil {
		t.Fatalf("syncElements: %v", err)
	}
}

func (h *debtHarness) element(t *testing.T, externalID string) *model.BudgetElement {
	t.Helper()
	el, err := h.budgetRepo.GetElementByExternal(h.ctx, h.budgetID, vo.MustParseId(externalID))
	if err != nil {
		t.Fatalf("element %s: %v", externalID, err)
	}
	return el
}

func (h *debtHarness) hasElement(t *testing.T, externalID string) bool {
	t.Helper()
	_, err := h.budgetRepo.GetElementByExternal(h.ctx, h.budgetID, vo.MustParseId(externalID))
	if err == nil {
		return true
	}
	if _, ok := errs.AsNotFound(err); !ok {
		t.Fatal(err)
	}
	return false
}

func (h *debtHarness) setLimit(userID vo.Id, externalID, period, amount string) error {
	a := vo.NewFlexString(amount)
	_, err := h.svc.SetLimit(h.ctx, userID, model.SetLimitRequest{
		BudgetId: h.budgetID.String(), ElementId: externalID, Period: period, Amount: &a,
	})
	return err
}

func (h *debtHarness) mustSetLimit(t *testing.T, externalID, period, amount string) {
	t.Helper()
	if err := h.setLimit(h.owner, externalID, period, amount); err != nil {
		t.Fatalf("SetLimit %s %s: %v", externalID, period, err)
	}
}

func (h *debtHarness) comment(externalID, period string) error {
	_, err := h.svc.CreateComment(h.ctx, h.owner, model.CreateCommentRequest{
		Id: vo.NewId().String(), BudgetId: h.budgetID.String(), ElementId: externalID, Period: period, Comment: "note",
	})
	return err
}

// limits returns the element's limits as "YYYY-MM-01" -> normalized amount.
func (h *debtHarness) limits(t *testing.T, elementID vo.Id) map[string]string {
	t.Helper()
	ls, err := h.budgetRepo.ListLimitsByElement(h.ctx, elementID)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, l := range ls {
		out[l.Period.Format("2006-01-02")] = vo.NewDecimal(l.Amount.String()).String()
	}
	return out
}

func (h *debtHarness) commentCount(t *testing.T, elementID vo.Id) int {
	t.Helper()
	cs, err := h.budgetRepo.ListCommentsFrom(h.ctx, h.budgetID, time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, c := range cs {
		if c.ElementID.Equal(elementID) {
			n++
		}
	}
	return n
}

func wantFieldCode(t *testing.T, err error, field, code string) {
	t.Helper()
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want validation error %s, got %v", code, err)
	}
	for _, f := range ve.Fields {
		if f.Key == field && f.Code == code {
			return
		}
	}
	t.Fatalf("want field %s code %s, got %+v", field, code, ve.Fields)
}

func amount(s string) string { return vo.NewDecimal(s).String() }

func TestSyncElements_CreatesDebtRows(t *testing.T) {
	h := newDebtHarness(t)
	own := h.addDebt(h.owner, h.eur)
	member := h.addDebt(h.member, fixture.USD)
	guest := h.addDebt(h.guest, fixture.USD)
	pending := h.addDebt(h.pending, fixture.USD)
	stranger := h.addDebt(h.stranger, fixture.USD)
	cat := h.f.Category(fixture.Category{UserID: h.owner.String(), Name: "Food"})

	h.sync(t)

	for id, cur := range map[string]string{own: h.eur, member: fixture.USD} {
		el := h.element(t, id)
		if el.Type != model.ElementDebt {
			t.Errorf("debt %s: type %d, want %d", id, el.Type, model.ElementDebt)
		}
		if el.CurrencyID == nil || el.CurrencyID.String() != cur {
			t.Errorf("debt %s: currency %v, want %s", id, el.CurrencyID, cur)
		}
		if el.FolderID != nil {
			t.Errorf("debt %s: folder %v, want none", id, el.FolderID)
		}
		if el.IsSortKeyUnset() {
			t.Errorf("debt %s: no sort key", id)
		}
	}
	for _, id := range []string{guest, pending, stranger} {
		if h.hasElement(t, id) {
			t.Errorf("debt %s of a non-participant got a row", id)
		}
	}
	// Debt rows are their own ordering group: a new category is appended to
	// the categories, not after the debts.
	catEl := h.element(t, cat)
	if catEl.IsSortKeyUnset() {
		t.Fatal("category row has no key")
	}

	// A second sync is a no-op for the rows (no duplicate, same key).
	before := h.element(t, own).SortKey
	h.sync(t)
	if got := h.element(t, own).SortKey; got != before {
		t.Errorf("resync moved the debt row: %q -> %q", before, got)
	}
}

func TestSyncElements_DropsDebtRowWhenDebtDeleted(t *testing.T) {
	h := newDebtHarness(t)
	debt := h.addDebt(h.owner, fixture.USD)
	h.sync(t)
	h.mustSetLimit(t, debt, "2026-05-01", "100")
	if err := h.comment(debt, "2026-05-01"); err != nil {
		t.Fatal(err)
	}
	el := h.element(t, debt)

	h.debts.remove(debt)
	h.sync(t)

	if h.hasElement(t, debt) {
		t.Fatal("the deleted debt's row is still there")
	}
	if got := h.limits(t, el.ID); len(got) != 0 {
		t.Errorf("limits of the dropped row survived: %v", got)
	}
	if n := h.commentCount(t, el.ID); n != 0 {
		t.Errorf("%d comments of the dropped row survived", n)
	}
}

func TestMoveElement_DebtRowRefusesFolder(t *testing.T) {
	h := newDebtHarness(t)
	debt := h.addDebt(h.owner, fixture.USD)
	h.sync(t)
	folder := h.f.BudgetFolder(fixture.BudgetFolder{BudgetID: h.budgetID.String()})

	_, err := h.svc.MoveElement(h.ctx, h.owner, model.MoveElementRequest{
		BudgetId: h.budgetID.String(), Id: debt, FolderId: &folder,
	})
	wantFieldCode(t, err, "folderId", errs.CodeBudgetDebtFolderNotAllowed)

	// Reordering within the no-folder area is allowed.
	if _, err := h.svc.MoveElement(h.ctx, h.owner, model.MoveElementRequest{BudgetId: h.budgetID.String(), Id: debt}); err != nil {
		t.Fatalf("move without folder: %v", err)
	}
	if h.element(t, debt).FolderID != nil {
		t.Fatal("debt row ended up in a folder")
	}
}

func TestSetLimit_DebtOutsideBoundsRefused(t *testing.T) {
	h := newDebtHarness(t)
	debt := h.addDebt(h.owner, fixture.USD)
	h.sync(t)

	for _, period := range []string{"2026-04-01", "2026-09-01"} {
		wantFieldCode(t, h.setLimit(h.owner, debt, period, "10"), "period", errs.CodeBudgetDebtPeriodOutOfRange)
		wantFieldCode(t, h.comment(debt, period), "period", errs.CodeBudgetDebtPeriodOutOfRange)
	}
	if got := h.limits(t, h.element(t, debt).ID); len(got) != 0 {
		t.Errorf("limits stored out of range: %v", got)
	}
}

func TestSetLimit_DebtInsideBounds(t *testing.T) {
	h := newDebtHarness(t)
	debt := h.addDebt(h.owner, fixture.USD)
	// No sync first: the row is self-healed by SetLimit.
	h.mustSetLimit(t, debt, "2026-05-01", "10")
	h.mustSetLimit(t, debt, "2026-08-01", "20")
	if err := h.comment(debt, "2026-08-01"); err != nil {
		t.Fatalf("comment inside bounds: %v", err)
	}
	got := h.limits(t, h.element(t, debt).ID)
	if got["2026-05-01"] != amount("10") || got["2026-08-01"] != amount("20") || len(got) != 2 {
		t.Errorf("limits = %v", got)
	}
}

func TestTransferElementData_MovesLimitsAndComments(t *testing.T) {
	h := newDebtHarness(t)
	cat := h.f.Category(fixture.Category{UserID: h.owner.String(), Name: "Loan"})
	h.mustSetLimit(t, cat, "2026-05-01", "100")
	h.mustSetLimit(t, cat, "2026-06-01", "70")
	if err := h.comment(cat, "2026-05-01"); err != nil {
		t.Fatal(err)
	}
	debt := h.addDebt(h.owner, fixture.USD)

	has, err := h.svc.ElementHasData(h.ctx, vo.MustParseId(debt), model.ElementDebt)
	if err != nil || has {
		t.Fatalf("debt has data before the transfer: %v %v", has, err)
	}
	if err := h.svc.TransferElementData(h.ctx, vo.MustParseId(cat), model.ElementCategory,
		vo.MustParseId(debt), model.ElementDebt, vo.MustParseId(fixture.USD)); err != nil {
		t.Fatalf("TransferElementData: %v", err)
	}

	catEl, debtEl := h.element(t, cat), h.element(t, debt)
	if got := h.limits(t, catEl.ID); len(got) != 0 {
		t.Errorf("category kept limits: %v", got)
	}
	got := h.limits(t, debtEl.ID)
	if got["2026-05-01"] != amount("100") || got["2026-06-01"] != amount("70") || len(got) != 2 {
		t.Errorf("debt limits = %v", got)
	}
	if n := h.commentCount(t, catEl.ID); n != 0 {
		t.Errorf("category kept %d comments", n)
	}
	if n := h.commentCount(t, debtEl.ID); n != 1 {
		t.Errorf("debt has %d comments, want 1", n)
	}
	has, err = h.svc.ElementHasData(h.ctx, vo.MustParseId(debt), model.ElementDebt)
	if err != nil || !has {
		t.Fatalf("debt has no data after the transfer: %v %v", has, err)
	}
	// The type is part of the key: the category id names no debt row.
	if has, _ := h.svc.ElementHasData(h.ctx, vo.MustParseId(debt), model.ElementCategory); has {
		t.Fatal("ElementHasData ignored the type")
	}
}

func TestTransferElementData_SkipsOtherCurrency(t *testing.T) {
	h := newDebtHarness(t)
	cat := h.f.Category(fixture.Category{UserID: h.owner.String(), Name: "Loan"})
	h.mustSetLimit(t, cat, "2026-05-01", "100")
	debt := h.addDebt(h.owner, h.eur)

	// The category row is in the budget's currency (USD), the debt in EUR.
	if err := h.svc.TransferElementData(h.ctx, vo.MustParseId(cat), model.ElementCategory,
		vo.MustParseId(debt), model.ElementDebt, vo.MustParseId(h.eur)); err != nil {
		t.Fatalf("TransferElementData: %v", err)
	}
	if got := h.limits(t, h.element(t, cat).ID); got["2026-05-01"] != amount("100") {
		t.Errorf("category limits = %v, want them untouched", got)
	}
	if h.hasElement(t, debt) {
		if got := h.limits(t, h.element(t, debt).ID); len(got) != 0 {
			t.Errorf("debt got limits in another currency: %v", got)
		}
	}
}

func TestTransferElementData_KeepsTargetOnPeriodClash(t *testing.T) {
	h := newDebtHarness(t)
	cat := h.f.Category(fixture.Category{UserID: h.owner.String(), Name: "Loan"})
	debt := h.addDebt(h.owner, fixture.USD)
	h.mustSetLimit(t, cat, "2026-05-01", "100")
	h.mustSetLimit(t, cat, "2026-06-01", "70")
	h.mustSetLimit(t, debt, "2026-05-01", "50")

	if err := h.svc.TransferElementData(h.ctx, vo.MustParseId(cat), model.ElementCategory,
		vo.MustParseId(debt), model.ElementDebt, vo.MustParseId(fixture.USD)); err != nil {
		t.Fatalf("TransferElementData: %v", err)
	}
	got := h.limits(t, h.element(t, debt).ID)
	if got["2026-05-01"] != amount("50") || got["2026-06-01"] != amount("70") || len(got) != 2 {
		t.Errorf("debt limits = %v, want 05=50 (target wins), 06=70", got)
	}
	if got := h.limits(t, h.element(t, cat).ID); len(got) != 0 {
		t.Errorf("category kept limits: %v", got)
	}
}

func TestRemoveMember_DropsTheirDebtRows(t *testing.T) {
	h := newDebtHarness(t)
	own := h.addDebt(h.owner, fixture.USD)
	member := h.addDebt(h.member, fixture.USD)
	h.sync(t)

	if err := h.svc.RemoveMember(h.ctx, h.budgetID, h.member); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
	if h.hasElement(t, member) {
		t.Error("the departed member's debt row is still there")
	}
	if !h.hasElement(t, own) {
		t.Error("the owner's debt row went too")
	}
}
