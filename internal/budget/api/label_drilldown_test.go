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

// labelId still refuses to pair with another SELECTOR (a budgeting tag or an
// envelope): which of the two narrows would be ambiguous. Only categoryId and
// uncategorized narrow a label, and those are covered separately below.
func TestGetTransactionListRejectsLabelIdWithTagOrEnvelope(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": labelDrilldownBudgetID, "name": "Label Drilldown Budget", "currencyId": usdID, "startDate": "2024-04-01"}); st != http.StatusOK {
		t.Fatalf("create-budget=%d body=%s", st, e.raw)
	}
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Label(fixture.Label{ID: labelDrillWorkID, UserID: seedUserID, Name: "Work"})

	base := "/api/v1/budget/get-transaction-list?budgetId=" + labelDrilldownBudgetID + "&periodStart=2024-04-01"

	st, b := h.do(t, http.MethodGet, base+"&labelId="+labelDrillWorkID+"&tagId="+tagID, tok, nil)
	if st != http.StatusBadRequest {
		t.Fatalf("labelId+tagId=%d, want 400: %s", st, b.raw)
	}

	envID := "bbbb6666-0000-7000-8000-00000000000e"
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", tok, map[string]any{
		"budgetId": labelDrilldownBudgetID, "id": envID, "name": "Env", "icon": "wallet", "currencyId": usdID,
	}); st != http.StatusOK {
		t.Fatalf("create-envelope=%d body=%s", st, e.raw)
	}
	st, b = h.do(t, http.MethodGet, base+"&labelId="+labelDrillWorkID+"&envelopeId="+envID, tok, nil)
	if st != http.StatusBadRequest {
		t.Fatalf("labelId+envelopeId=%d, want 400: %s", st, b.raw)
	}
}

// Clicking a category row inside an expanded reporting-tag folder must list
// exactly the transactions in BOTH that label and that category -- not the
// label's whole set (which would mean the category was silently dropped) and
// not the category's whole set (which would mean the label was).
func TestGetTransactionListByLabelAndCategory(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": labelDrilldownBudgetID, "name": "Label Drilldown Budget", "currencyId": usdID, "startDate": "2024-04-01"}); st != http.StatusOK {
		t.Fatalf("create-budget=%d body=%s", st, e.raw)
	}

	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Label(fixture.Label{ID: labelDrillWorkID, UserID: seedUserID, Name: "Work"})
	otherCat := "cccc6666-0000-7000-8000-000000000001"
	f.Category(fixture.Category{ID: otherCat, UserID: seedUserID, Name: "Other", Icon: "x"})

	attach := func(txID, labelID string) {
		t.Helper()
		if _, err := h.db.Exec(`INSERT INTO transactions_labels (transaction_id, label_id) VALUES (?, ?)`, txID, labelID); err != nil {
			t.Fatalf("attach label: %v", err)
		}
	}

	// In the label AND in catID: the only row the drill-down may return.
	wantID := f.Transaction(fixture.Transaction{
		ID: "ffff6666-0000-7000-8000-000000000011", UserID: seedUserID, AccountID: accountID,
		CategoryID: catID, Type: 0, Amount: "50.00", SpentAt: "2024-04-10 00:00:00",
	})
	attach(wantID, labelDrillWorkID)

	// Same label, different category.
	otherCatTx := f.Transaction(fixture.Transaction{
		ID: "ffff6666-0000-7000-8000-000000000012", UserID: seedUserID, AccountID: accountID,
		CategoryID: otherCat, Type: 0, Amount: "7.00", SpentAt: "2024-04-11 00:00:00",
	})
	attach(otherCatTx, labelDrillWorkID)

	// Same category, no label at all.
	f.Transaction(fixture.Transaction{
		ID: "ffff6666-0000-7000-8000-000000000013", UserID: seedUserID, AccountID: accountID,
		CategoryID: catID, Type: 0, Amount: "3.00", SpentAt: "2024-04-12 00:00:00",
	})

	// Same label, no category: must not leak into the category-narrowed list.
	uncatTx := f.Transaction(fixture.Transaction{
		ID: "ffff6666-0000-7000-8000-000000000014", UserID: seedUserID, AccountID: accountID,
		Type: 0, Amount: "11.00", SpentAt: "2024-04-13 00:00:00",
	})
	attach(uncatTx, labelDrillWorkID)

	base := "/api/v1/budget/get-transaction-list?budgetId=" + labelDrilldownBudgetID + "&periodStart=2024-04-01"

	st, b := h.do(t, http.MethodGet, base+"&labelId="+labelDrillWorkID+"&categoryId="+catID, tok, nil)
	if st != http.StatusOK {
		t.Fatalf("labelId+categoryId=%d body=%s", st, b.raw)
	}
	got := mustUnmarshal[txListView](t, b.Data)
	if len(got.Items) != 1 || got.Items[0].Id != wantID {
		t.Fatalf("labelId+categoryId must return only %q, got %d items: %s", wantID, len(got.Items), b.Data)
	}

	// The uncategorized child of the same folder: the label's category-less rows.
	st, b = h.do(t, http.MethodGet, base+"&labelId="+labelDrillWorkID+"&uncategorized=1", tok, nil)
	if st != http.StatusOK {
		t.Fatalf("labelId+uncategorized=%d body=%s", st, b.raw)
	}
	got = mustUnmarshal[txListView](t, b.Data)
	if len(got.Items) != 1 || got.Items[0].Id != uncatTx {
		t.Fatalf("labelId+uncategorized must return only %q, got %d items: %s", uncatTx, len(got.Items), b.Data)
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
