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
	left, err := h.svc.TransferElementData(h.ctx, model.ElementDataTransfer{FromID: vo.MustParseId(cat), FromType: model.ElementCategory,
		ToID: vo.MustParseId(debt), ToType: model.ElementDebt, CurrencyID: vo.MustParseId(fixture.USD)})
	if err != nil || left {
		t.Fatalf("TransferElementData = %v, %v; want everything moved", left, err)
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
	left, err := h.svc.TransferElementData(h.ctx, model.ElementDataTransfer{FromID: vo.MustParseId(cat), FromType: model.ElementCategory,
		ToID: vo.MustParseId(debt), ToType: model.ElementDebt, CurrencyID: vo.MustParseId(h.eur)})
	if err != nil || !left {
		t.Fatalf("TransferElementData = %v, %v; want the plans reported left behind", left, err)
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

	left, err := h.svc.TransferElementData(h.ctx, model.ElementDataTransfer{FromID: vo.MustParseId(cat), FromType: model.ElementCategory,
		ToID: vo.MustParseId(debt), ToType: model.ElementDebt, CurrencyID: vo.MustParseId(fixture.USD)})
	if err != nil || !left {
		t.Fatalf("TransferElementData = %v, %v; want the dropped plan reported", left, err)
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

func TestTransferElementData_SkipsTargetInOtherCurrency(t *testing.T) {
	h := newDebtHarness(t)
	cat := h.f.Category(fixture.Category{UserID: h.owner.String(), Name: "Loan"})
	debt := h.addDebt(h.owner, fixture.USD)
	h.mustSetLimit(t, debt, "2026-05-01", "50")
	if err := h.comment(debt, "2026-05-01"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.ChangeElementCurrency(h.ctx, h.owner, model.ChangeElementCurrencyRequest{
		BudgetId: h.budgetID.String(), ElementId: cat, CurrencyId: h.eur,
	}); err != nil {
		t.Fatal(err)
	}

	left, err := h.svc.TransferElementData(h.ctx, model.ElementDataTransfer{FromID: vo.MustParseId(debt), FromType: model.ElementDebt,
		ToID: vo.MustParseId(cat), ToType: model.ElementCategory, CurrencyID: vo.MustParseId(fixture.USD)})
	if err != nil || !left {
		t.Fatalf("TransferElementData = %v, %v; want the plans reported left behind", left, err)
	}
	debtEl, catEl := h.element(t, debt), h.element(t, cat)
	if got := h.limits(t, debtEl.ID); got["2026-05-01"] != amount("50") {
		t.Errorf("debt limits = %v, want them untouched", got)
	}
	if got := h.limits(t, catEl.ID); len(got) != 0 {
		t.Errorf("EUR category got USD plans: %v", got)
	}
	if n := h.commentCount(t, debtEl.ID); n != 1 {
		t.Errorf("debt has %d comments, want its 1 kept", n)
	}
}

func TestTransferElementData_NothingLeftWithoutSourceData(t *testing.T) {
	h := newDebtHarness(t)
	cat := h.f.Category(fixture.Category{UserID: h.owner.String(), Name: "Loan"})
	debt := h.addDebt(h.owner, h.eur)
	h.sync(t)

	left, err := h.svc.TransferElementData(h.ctx, model.ElementDataTransfer{FromID: vo.MustParseId(cat), FromType: model.ElementCategory,
		ToID: vo.MustParseId(debt), ToType: model.ElementDebt, CurrencyID: vo.MustParseId(h.eur)})
	if err != nil || left {
		t.Fatalf("TransferElementData = %v, %v; an empty row leaves nothing behind", left, err)
	}
}

func TestChangeElementCurrency_DebtRowRefused(t *testing.T) {
	h := newDebtHarness(t)
	debt := h.addDebt(h.owner, fixture.USD)
	h.sync(t)

	_, err := h.svc.ChangeElementCurrency(h.ctx, h.owner, model.ChangeElementCurrencyRequest{
		BudgetId: h.budgetID.String(), ElementId: debt, CurrencyId: h.eur,
	})
	wantFieldCode(t, err, "currencyId", errs.CodeBudgetDebtCurrencyFixed)
	if el := h.element(t, debt); el.CurrencyID == nil || el.CurrencyID.String() != fixture.USD {
		t.Fatalf("currency = %v, want USD kept", el.CurrencyID)
	}
}

// A plan stored before the debt's months moved stays clearable.
func TestSetLimit_DebtClearOutsideBoundsAllowed(t *testing.T) {
	h := newDebtHarness(t)
	debt := h.addDebt(h.owner, fixture.USD)
	h.mustSetLimit(t, debt, "2026-08-01", "20")
	h.debts.views[0].EndMonth = month(2026, time.July)

	wantFieldCode(t, h.setLimit(h.owner, debt, "2026-08-01", "30"), "period", errs.CodeBudgetDebtPeriodOutOfRange)
	if _, err := h.svc.SetLimit(h.ctx, h.owner, model.SetLimitRequest{
		BudgetId: h.budgetID.String(), ElementId: debt, Period: "2026-08-01",
	}); err != nil {
		t.Fatalf("clearing outside the bounds: %v", err)
	}
	if got := h.limits(t, h.element(t, debt).ID); len(got) != 0 {
		t.Errorf("limits = %v, want the August plan cleared", got)
	}
}

// A merge must not lose the target's plan for a month both rows plan: the
// two add up, like a category merge.
func TestTransferElementData_SumsOnClashWhenAsked(t *testing.T) {
	h := newDebtHarness(t)
	cat := h.f.Category(fixture.Category{UserID: h.owner.String(), Name: "Loan"})
	debt := h.addDebt(h.owner, fixture.USD)
	h.mustSetLimit(t, cat, "2026-05-01", "100")
	h.mustSetLimit(t, cat, "2026-06-01", "70")
	h.mustSetLimit(t, debt, "2026-05-01", "50")

	left, err := h.svc.TransferElementData(h.ctx, model.ElementDataTransfer{FromID: vo.MustParseId(cat), FromType: model.ElementCategory,
		ToID: vo.MustParseId(debt), ToType: model.ElementDebt, CurrencyID: vo.MustParseId(fixture.USD), SumOnClash: true})
	if err != nil || left {
		t.Fatalf("TransferElementData = %v, %v; want everything moved", left, err)
	}
	got := h.limits(t, h.element(t, debt).ID)
	if got["2026-05-01"] != amount("150") || got["2026-06-01"] != amount("70") || len(got) != 2 {
		t.Errorf("debt limits = %v, want 05=150 (summed), 06=70", got)
	}
}

// Plans and comments outside the debt's months stay on the category row,
// where its Available keeps counting them.
func TestTransferElementData_MovesOnlyTheBoundedMonths(t *testing.T) {
	h := newDebtHarness(t)
	cat := h.f.Category(fixture.Category{UserID: h.owner.String(), Name: "Loan"})
	debt := h.addDebt(h.owner, fixture.USD)
	h.mustSetLimit(t, cat, "2026-04-01", "10")
	h.mustSetLimit(t, cat, "2026-05-01", "100")
	h.mustSetLimit(t, cat, "2026-06-01", "70")
	for _, m := range []string{"2026-04-01", "2026-05-01"} {
		if err := h.comment(cat, m); err != nil {
			t.Fatal(err)
		}
	}

	left, err := h.svc.TransferElementData(h.ctx, model.ElementDataTransfer{FromID: vo.MustParseId(cat), FromType: model.ElementCategory,
		ToID: vo.MustParseId(debt), ToType: model.ElementDebt, CurrencyID: vo.MustParseId(fixture.USD),
		FirstDay: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), LastDay: time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)})
	if err != nil || left {
		t.Fatalf("TransferElementData = %v, %v; want nothing reported left behind", left, err)
	}
	catEl, debtEl := h.element(t, cat), h.element(t, debt)
	if got := h.limits(t, debtEl.ID); got["2026-05-01"] != amount("100") || len(got) != 1 {
		t.Errorf("debt limits = %v, want only 05=100", got)
	}
	if got := h.limits(t, catEl.ID); got["2026-04-01"] != amount("10") || got["2026-06-01"] != amount("70") || len(got) != 2 {
		t.Errorf("category limits = %v, want 04 and 06 kept", got)
	}
	if n := h.commentCount(t, debtEl.ID); n != 1 {
		t.Errorf("debt has %d comments, want the May one", n)
	}
	if n := h.commentCount(t, catEl.ID); n != 1 {
		t.Errorf("category has %d comments, want the April one kept", n)
	}
}

// A budget whose periods start on the 15th moves the periods holding the
// transfer's days: 20 May is in period May, 10 Jul in period June.
func TestTransferElementData_BoundsFollowShiftedPeriods(t *testing.T) {
	h := newDebtHarness(t)
	if _, err := h.tdb.Raw.Exec(h.tdb.Rebind(`UPDATE budgets SET period_start_day = 15 WHERE id = ?`), h.budgetID.String()); err != nil {
		t.Fatal(err)
	}
	cat := h.f.Category(fixture.Category{UserID: h.owner.String(), Name: "Loan"})
	debt := h.addDebt(h.owner, fixture.USD)
	for _, m := range []string{"2026-04-01", "2026-05-01", "2026-06-01", "2026-07-01"} {
		h.mustSetLimit(t, cat, m, "10")
	}

	if _, err := h.svc.TransferElementData(h.ctx, model.ElementDataTransfer{FromID: vo.MustParseId(cat), FromType: model.ElementCategory,
		ToID: vo.MustParseId(debt), ToType: model.ElementDebt, CurrencyID: vo.MustParseId(fixture.USD),
		FirstDay: time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC), LastDay: time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	if got := h.limits(t, h.element(t, debt).ID); len(got) != 2 || got["2026-05-01"] != amount("10") || got["2026-06-01"] != amount("10") {
		t.Errorf("debt limits = %v, want May and June", got)
	}
	if got := h.limits(t, h.element(t, cat).ID); len(got) != 2 || got["2026-04-01"] != amount("10") || got["2026-07-01"] != amount("10") {
		t.Errorf("category limits = %v, want April and July kept", got)
	}
}

// A category inside an envelope never shows its own plans: handing a deleted
// debt's plans to its hidden row would make them vanish, so they are reported
// left behind and the delete asks first.
func TestTransferElementData_EnvelopeChildCannotTakeTheData(t *testing.T) {
	h := newDebtHarness(t)
	cat := h.f.Category(fixture.Category{UserID: h.owner.String(), Name: "Loan"})
	debt := h.addDebt(h.owner, fixture.USD)
	if _, err := h.svc.CreateEnvelope(h.ctx, h.owner, model.CreateEnvelopeRequest{BudgetId: h.budgetID.String(), Id: vo.NewId().String(),
		Name: "Bills", Icon: "cart", CurrencyId: fixture.USD, Categories: []string{cat}}); err != nil {
		t.Fatal(err)
	}
	h.mustSetLimit(t, debt, "2026-05-01", "50")

	left, err := h.svc.TransferElementData(h.ctx, model.ElementDataTransfer{FromID: vo.MustParseId(debt), FromType: model.ElementDebt,
		ToID: vo.MustParseId(cat), ToType: model.ElementCategory, CurrencyID: vo.MustParseId(fixture.USD)})
	if err != nil || !left {
		t.Fatalf("TransferElementData = %v, %v; want the plans reported left behind", left, err)
	}
	if got := h.limits(t, h.element(t, debt).ID); got["2026-05-01"] != amount("50") {
		t.Errorf("debt limits = %v, want them kept on the debt row", got)
	}
}

// A deleted debt's rows go with it, plans and comments included, not on the
// budget's next structure sync.
func TestRemoveElements_DropsTheDebtRowsAndTheirData(t *testing.T) {
	h := newDebtHarness(t)
	debt := h.addDebt(h.owner, fixture.USD)
	keep := h.f.Category(fixture.Category{UserID: h.owner.String(), Name: "Food"})
	h.mustSetLimit(t, debt, "2026-05-01", "50")
	h.mustSetLimit(t, keep, "2026-05-01", "20")
	if err := h.comment(debt, "2026-05-01"); err != nil {
		t.Fatal(err)
	}

	if err := h.svc.RemoveElements(h.ctx, vo.MustParseId(debt), model.ElementDebt); err != nil {
		t.Fatalf("RemoveElements: %v", err)
	}
	if h.hasElement(t, debt) {
		t.Fatal("the debt row is still there")
	}
	if has, err := h.svc.ElementHasData(h.ctx, vo.MustParseId(debt), model.ElementDebt); err != nil || has {
		t.Fatalf("debt data left: %v, %v", has, err)
	}
	if got := h.limits(t, h.element(t, keep).ID); got["2026-05-01"] != amount("20") {
		t.Errorf("another row lost its plan: %v", got)
	}
}

// AfterDay moves only the periods after the one holding that day: 10 Jun is
// in period May of a budget whose periods start on the 15th, so June on moves
// back with its comment and May stays.
func TestTransferElementData_AfterDayMovesTheLaterPeriods(t *testing.T) {
	h := newDebtHarness(t)
	if _, err := h.tdb.Raw.Exec(h.tdb.Rebind(`UPDATE budgets SET period_start_day = 15 WHERE id = ?`), h.budgetID.String()); err != nil {
		t.Fatal(err)
	}
	cat := h.f.Category(fixture.Category{UserID: h.owner.String(), Name: "Loan"})
	debt := h.addDebt(h.owner, fixture.USD)
	for _, m := range []string{"2026-05-01", "2026-06-01", "2026-07-01"} {
		h.mustSetLimit(t, debt, m, "10")
	}
	h.mustSetLimit(t, cat, "2026-07-01", "5")
	for _, m := range []string{"2026-05-01", "2026-07-01"} {
		if err := h.comment(debt, m); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := h.svc.TransferElementData(h.ctx, model.ElementDataTransfer{FromID: vo.MustParseId(debt), FromType: model.ElementDebt,
		ToID: vo.MustParseId(cat), ToType: model.ElementCategory, CurrencyID: vo.MustParseId(fixture.USD), SumOnClash: true,
		AfterDay: time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	debtEl, catEl := h.element(t, debt), h.element(t, cat)
	if got := h.limits(t, debtEl.ID); len(got) != 1 || got["2026-05-01"] != amount("10") {
		t.Errorf("debt limits = %v, want only May kept", got)
	}
	if got := h.limits(t, catEl.ID); len(got) != 2 || got["2026-06-01"] != amount("10") || got["2026-07-01"] != amount("15") {
		t.Errorf("category limits = %v, want June 10 and July 15 (summed)", got)
	}
	if n := h.commentCount(t, debtEl.ID); n != 1 {
		t.Errorf("debt has %d comments, want the May one kept", n)
	}
	if n := h.commentCount(t, catEl.ID); n != 1 {
		t.Errorf("category has %d comments, want the July one moved", n)
	}
}

// BeforeDay moves only the periods before the one holding that day: 20 Jun is
// in period June of a budget whose periods start on the 15th, so May moves
// back with its comment and June on stays.
func TestTransferElementData_BeforeDayMovesTheEarlierPeriods(t *testing.T) {
	h := newDebtHarness(t)
	if _, err := h.tdb.Raw.Exec(h.tdb.Rebind(`UPDATE budgets SET period_start_day = 15 WHERE id = ?`), h.budgetID.String()); err != nil {
		t.Fatal(err)
	}
	cat := h.f.Category(fixture.Category{UserID: h.owner.String(), Name: "Loan"})
	debt := h.addDebt(h.owner, fixture.USD)
	for _, m := range []string{"2026-05-01", "2026-06-01", "2026-07-01"} {
		h.mustSetLimit(t, debt, m, "10")
	}
	h.mustSetLimit(t, cat, "2026-05-01", "5")
	for _, m := range []string{"2026-05-01", "2026-07-01"} {
		if err := h.comment(debt, m); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := h.svc.TransferElementData(h.ctx, model.ElementDataTransfer{FromID: vo.MustParseId(debt), FromType: model.ElementDebt,
		ToID: vo.MustParseId(cat), ToType: model.ElementCategory, CurrencyID: vo.MustParseId(fixture.USD), SumOnClash: true,
		BeforeDay: time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	debtEl, catEl := h.element(t, debt), h.element(t, cat)
	if got := h.limits(t, debtEl.ID); len(got) != 2 || got["2026-06-01"] != amount("10") || got["2026-07-01"] != amount("10") {
		t.Errorf("debt limits = %v, want June and July kept", got)
	}
	if got := h.limits(t, catEl.ID); len(got) != 1 || got["2026-05-01"] != amount("15") {
		t.Errorf("category limits = %v, want May 15 (summed)", got)
	}
	if n := h.commentCount(t, debtEl.ID); n != 1 {
		t.Errorf("debt has %d comments, want the July one kept", n)
	}
	if n := h.commentCount(t, catEl.ID); n != 1 {
		t.Errorf("category has %d comments, want the May one moved", n)
	}
}

// A debt whose currency changed moves its rows to that currency; their plans
// keep their numbers.
func TestSetElementsCurrency_DebtRowsFollowAndKeepPlans(t *testing.T) {
	h := newDebtHarness(t)
	debt := h.addDebt(h.owner, fixture.USD)
	h.mustSetLimit(t, debt, "2026-05-01", "100")
	el := h.element(t, debt)

	if err := h.svc.SetElementsCurrency(h.ctx, vo.MustParseId(debt), model.ElementDebt, vo.MustParseId(h.eur)); err != nil {
		t.Fatalf("SetElementsCurrency: %v", err)
	}
	got := h.element(t, debt)
	if got.CurrencyID == nil || got.CurrencyID.String() != h.eur {
		t.Fatalf("currency = %v, want %s", got.CurrencyID, h.eur)
	}
	if l := h.limits(t, el.ID); l["2026-05-01"] != amount("100") {
		t.Errorf("plans = %v, want 100 in May", l)
	}
}

func TestSyncElements_DebtRowFollowsTheDebtCurrency(t *testing.T) {
	h := newDebtHarness(t)
	debt := h.addDebt(h.owner, fixture.USD)
	h.sync(t)
	h.mustSetLimit(t, debt, "2026-05-01", "100")
	el := h.element(t, debt)

	h.debts.views[0].CurrencyID = h.eur
	h.sync(t)

	got := h.element(t, debt)
	if got.CurrencyID == nil || got.CurrencyID.String() != h.eur {
		t.Fatalf("currency = %v, want %s", got.CurrencyID, h.eur)
	}
	if !got.ID.Equal(el.ID) {
		t.Fatal("the row was recreated")
	}
	if l := h.limits(t, el.ID); l["2026-05-01"] != amount("100") {
		t.Errorf("plans = %v, want 100 in May", l)
	}
}

// DebtRowPlans feeds the debt forecast: a reader of the budget gets the
// debt row's plans with the budget's name and period start day; anyone else,
// or a budget without the row, gets nothing.
func TestDebtRowPlans(t *testing.T) {
	h := newDebtHarness(t)
	own := h.addDebt(h.owner, fixture.USD)
	h.sync(t)
	h.mustSetLimit(t, own, "2026-06-01", "250")

	got, ok, err := h.svc.DebtRowPlans(h.ctx, h.owner, h.budgetID, vo.MustParseId(own))
	if err != nil || !ok {
		t.Fatalf("owner: ok=%v err=%v", ok, err)
	}
	if got.Name == "" || got.PeriodStartDay != model.MinPeriodStartDay || len(got.Plans) != 1 || amount(got.Plans["2026-06-01"]) != amount("250") {
		t.Fatalf("plans = %+v", got)
	}
	if _, ok, err := h.svc.DebtRowPlans(h.ctx, h.guest, h.budgetID, vo.MustParseId(own)); err != nil || !ok {
		t.Fatalf("a guest reads the budget: ok=%v err=%v", ok, err)
	}
	for name, user := range map[string]vo.Id{"pending": h.pending, "stranger": h.stranger} {
		if _, ok, err := h.svc.DebtRowPlans(h.ctx, user, h.budgetID, vo.MustParseId(own)); err != nil || ok {
			t.Fatalf("%s: ok=%v err=%v, want nothing", name, ok, err)
		}
	}
	if _, ok, err := h.svc.DebtRowPlans(h.ctx, h.owner, h.budgetID, vo.NewId()); err != nil || ok {
		t.Fatalf("no row: ok=%v err=%v", ok, err)
	}
	if _, ok, err := h.svc.DebtRowPlans(h.ctx, h.owner, vo.NewId(), vo.MustParseId(own)); err != nil || ok {
		t.Fatalf("unknown budget: ok=%v err=%v", ok, err)
	}
}
