package api_test

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	labelDrilldownBudgetID = "bbbb6666-0000-7000-8000-000000000006"
	labelDrillWorkID       = "eeee6666-0000-7000-8000-000000000001"
	labelDrillHomeID       = "eeee6666-0000-7000-8000-000000000002"
)

// Clicking a label chip lists exactly the transactions carrying it. The
// headline case: a transaction carrying BOTH labelWork and labelHome must
// appear when drilling into EITHER one (the link is many-to-many, not a
// single tag_id column), while a transaction carrying only a different label
// must not appear (a query that ignored the label id entirely would still
// pass the "appears under both" half of this test).
func TestGetTransactionListByLabel(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": labelDrilldownBudgetID, "name": "Label Drilldown Budget", "currencyId": usdID, "startDate": "2024-04-01"}); st != http.StatusOK {
		t.Fatalf("create-budget=%d body=%s", st, e.raw)
	}

	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Label(fixture.Label{ID: labelDrillWorkID, UserID: seedUserID, Name: "Work"})
	f.Label(fixture.Label{ID: labelDrillHomeID, UserID: seedUserID, Name: "Home"})

	bothID := f.Transaction(fixture.Transaction{
		ID: "ffff6666-0000-7000-8000-000000000001", UserID: seedUserID, AccountID: accountID,
		CategoryID: catID, Type: 0, Amount: "50.00", SpentAt: "2024-04-10 00:00:00",
	})
	if _, err := h.db.Exec(`INSERT INTO transactions_labels (transaction_id, label_id) VALUES (?, ?), (?, ?)`,
		bothID, labelDrillWorkID, bothID, labelDrillHomeID); err != nil {
		t.Fatalf("attach labels: %v", err)
	}

	otherLabelID := "eeee6666-0000-7000-8000-000000000003"
	f.Label(fixture.Label{ID: otherLabelID, UserID: seedUserID, Name: "Other"})
	otherID := f.Transaction(fixture.Transaction{
		ID: "ffff6666-0000-7000-8000-000000000002", UserID: seedUserID, AccountID: accountID,
		CategoryID: catID, Type: 0, Amount: "9.00", SpentAt: "2024-04-11 00:00:00",
	})
	if _, err := h.db.Exec(`INSERT INTO transactions_labels (transaction_id, label_id) VALUES (?, ?)`,
		otherID, otherLabelID); err != nil {
		t.Fatalf("attach other label: %v", err)
	}

	base := "/api/v1/budget/get-transaction-list?budgetId=" + labelDrilldownBudgetID + "&periodStart=2024-04-01"

	st, b := h.do(t, http.MethodGet, base+"&labelId="+labelDrillWorkID, tok, nil)
	if st != http.StatusOK {
		t.Fatalf("get-transaction-list(work)=%d body=%s", st, b.raw)
	}
	got := mustUnmarshal[txListView](t, b.Data)
	if len(got.Items) != 1 || got.Items[0].Id != bothID {
		t.Fatalf("labelId=work must return only %q, got %d items: %s", bothID, len(got.Items), b.Data)
	}

	_, b = h.do(t, http.MethodGet, base+"&labelId="+labelDrillHomeID, tok, nil)
	got = mustUnmarshal[txListView](t, b.Data)
	if len(got.Items) != 1 || got.Items[0].Id != bothID {
		t.Fatalf("labelId=home must return only %q (the shared transaction), got %d items: %s", bothID, len(got.Items), b.Data)
	}

	_, b = h.do(t, http.MethodGet, base+"&labelId="+otherLabelID, tok, nil)
	got = mustUnmarshal[txListView](t, b.Data)
	if len(got.Items) != 1 || got.Items[0].Id != otherID {
		t.Fatalf("labelId=other must return only %q, got %d items: %s", otherID, len(got.Items), b.Data)
	}
}

// labelId is mutually exclusive with the other selectors, like tagId/envelopeId.
func TestGetTransactionListRejectsLabelIdWithCategoryId(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": labelDrilldownBudgetID, "name": "Label Drilldown Budget", "currencyId": usdID, "startDate": "2024-04-01"}); st != http.StatusOK {
		t.Fatalf("create-budget=%d body=%s", st, e.raw)
	}
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Label(fixture.Label{ID: labelDrillWorkID, UserID: seedUserID, Name: "Work"})

	base := "/api/v1/budget/get-transaction-list?budgetId=" + labelDrilldownBudgetID + "&periodStart=2024-04-01"

	st, b := h.do(t, http.MethodGet, base+"&labelId="+labelDrillWorkID+"&categoryId="+catID, tok, nil)
	if st != http.StatusBadRequest {
		t.Fatalf("labelId+categoryId=%d, want 400: %s", st, b.raw)
	}

	st, b = h.do(t, http.MethodGet, base+"&labelId="+labelDrillWorkID+"&tagId="+tagID, tok, nil)
	if st != http.StatusBadRequest {
		t.Fatalf("labelId+tagId=%d, want 400: %s", st, b.raw)
	}

	st, b = h.do(t, http.MethodGet, base+"&labelId="+labelDrillWorkID+"&uncategorized=1", tok, nil)
	if st != http.StatusBadRequest {
		t.Fatalf("labelId+uncategorized=%d, want 400: %s", st, b.raw)
	}
}

// TestGetTransactionListWhitespaceLabelId_TreatedAsAbsent: the mutual-
// exclusion guard trims labelId before deciding whether it's "set", so a
// whitespace-only value must be absent EVERYWHERE downstream too, not just at
// the guard. Before the fix, the guard let a whitespace-only labelId through
// untouched but the selector switch still saw the raw untrimmed value, so
// labelId=" " alone (no other selector) reported the field-specific "labelId
// must not be blank" error, as if a real label filter had been attempted,
// instead of the generic "select a filter" error an omitted labelId
// produces. This asserts both requests now yield the identical shape.
func TestGetTransactionListWhitespaceLabelId_TreatedAsAbsent(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": labelDrilldownBudgetID, "name": "Label Drilldown Budget", "currencyId": usdID, "startDate": "2024-04-01"}); st != http.StatusOK {
		t.Fatalf("create-budget=%d body=%s", st, e.raw)
	}

	base := "/api/v1/budget/get-transaction-list?budgetId=" + labelDrilldownBudgetID + "&periodStart=2024-04-01"

	stAbsent, envAbsent := h.do(t, http.MethodGet, base, tok, nil)
	stWs, envWs := h.do(t, http.MethodGet, base+"&labelId=%20", tok, nil)

	if stWs != stAbsent {
		t.Fatalf("status with whitespace labelId = %d, want the same as no filter (%d)", stWs, stAbsent)
	}
	if envWs.Message != envAbsent.Message {
		t.Fatalf("message with whitespace labelId = %q, want the same as no filter %q", envWs.Message, envAbsent.Message)
	}
	if len(envWs.errorsMap()) != 0 {
		t.Fatalf("errors = %v, want none -- a whitespace-only labelId must not surface as a field-specific labelId error (that would imply a real label filter was attempted)", envWs.errorsMap())
	}
}
