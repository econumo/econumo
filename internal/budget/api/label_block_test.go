package api_test

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// labelBudgetView is the slice of get-budget tests in this file care about.
type labelBudgetView struct {
	Item struct {
		Structure struct {
			Elements []struct {
				Id    string `json:"id"`
				Spent string `json:"spent"`
			} `json:"elements"`
			Labels []struct {
				Id          string `json:"id"`
				Name        string `json:"name"`
				Icon        string `json:"icon"`
				IsArchived  int    `json:"isArchived"`
				Spent       string `json:"spent"`
				OwnerUserId string `json:"ownerUserId"`
			} `json:"labels"`
		} `json:"structure"`
	} `json:"item"`
}

func (v labelBudgetView) findLabel(id string) (spent string, ok bool) {
	for _, l := range v.Item.Structure.Labels {
		if l.Id == id {
			return l.Spent, true
		}
	}
	return "", false
}

func (v labelBudgetView) findElement(id string) (spent string, ok bool) {
	for _, e := range v.Item.Structure.Elements {
		if e.Id == id {
			return e.Spent, true
		}
	}
	return "", false
}

const (
	labelBudgetID = "bbbb5555-0000-7000-8000-000000000005"
	labelAID      = "eeee5555-0000-7000-8000-000000000001"
	labelBID      = "eeee5555-0000-7000-8000-000000000002"
)

// TestLabelBlockOverlapsWithoutInflatingElements is the headline invariant
// test: a single 50.00 expense carrying two labels shows the FULL 50.00 under
// EACH label (labels deliberately overlap), while the category element that
// same expense belongs to still reports 50.00 spent — not 100.00. If the label
// accumulation ever leaked into the element's toConvert keys, the category
// would double-count.
func TestLabelBlockOverlapsWithoutInflatingElements(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": labelBudgetID, "name": "Label Budget", "currencyId": usdID, "startDate": "2024-04-01", "accountIds": []string{accountID}}); st != http.StatusOK {
		t.Fatalf("create-budget=%d body=%s", st, e.raw)
	}

	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Label(fixture.Label{ID: labelAID, UserID: seedUserID, Name: "Kid A", Icon: "face"})
	f.Label(fixture.Label{ID: labelBID, UserID: seedUserID, Name: "Kid B", Icon: "face"})

	txID := f.Transaction(fixture.Transaction{
		ID: "ffff5555-0000-7000-8000-000000000001", UserID: seedUserID, AccountID: accountID,
		CategoryID: catID, Type: 0, Amount: "50.00", SpentAt: "2024-04-10 00:00:00",
	})
	if _, err := h.db.Exec(`INSERT INTO transactions_labels (transaction_id, label_id) VALUES (?, ?), (?, ?)`,
		txID, labelAID, txID, labelBID); err != nil {
		t.Fatalf("attach labels: %v", err)
	}

	_, b := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+labelBudgetID+"&date=2024-04-15", tok, nil)
	res := mustUnmarshal[labelBudgetView](t, b.Data)

	spentA, ok := res.findLabel(labelAID)
	if !ok {
		t.Fatalf("label A missing from labels block; body: %s", b.Data)
	}
	if spentA != "50" {
		t.Errorf("label A spent=%q want 50 (full overlap amount)", spentA)
	}
	spentB, ok := res.findLabel(labelBID)
	if !ok {
		t.Fatalf("label B missing from labels block; body: %s", b.Data)
	}
	if spentB != "50" {
		t.Errorf("label B spent=%q want 50 (full overlap amount)", spentB)
	}

	catSpent, ok := res.findElement(catID)
	if !ok {
		t.Fatalf("category element missing; body: %s", b.Data)
	}
	if catSpent != "50" {
		t.Errorf("category spent=%q want 50 (unchanged by the two labels attached to the same expense)", catSpent)
	}
}

// TestLabelBlockHidesLabelsWithoutSpend guards the visibility rule: labels
// have no limit, so unlike a tag with a carried limit, a label with zero
// period spend has nothing to keep it on screen and must stay absent.
func TestLabelBlockHidesLabelsWithoutSpend(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": labelBudgetID, "name": "Label Budget", "currencyId": usdID, "startDate": "2024-04-01", "accountIds": []string{accountID}}); st != http.StatusOK {
		t.Fatalf("create-budget=%d body=%s", st, e.raw)
	}

	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Label(fixture.Label{ID: labelAID, UserID: seedUserID, Name: "Kid A"})
	// Deliberately no transaction referencing labelAID.

	_, b := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+labelBudgetID+"&date=2024-04-15", tok, nil)
	res := mustUnmarshal[labelBudgetView](t, b.Data)

	if _, ok := res.findLabel(labelAID); ok {
		t.Fatalf("a label with no spend in the period must stay hidden; body: %s", b.Data)
	}
}

// TestLabelBlockUsesAccountOwnerLabels: a second user is an accepted budget
// collaborator and owns the account the expense is posted from. Per the
// design, a shared account's spend aggregates under the ACCOUNT OWNER's
// labels, so the collaborator's own label must appear in the viewing owner's
// budget block, carrying the collaborator's user id.
func TestLabelBlockUsesAccountOwnerLabels(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": labelBudgetID, "name": "Label Budget", "currencyId": usdID, "startDate": "2024-04-01", "accountIds": []string{accountID}}); st != http.StatusOK {
		t.Fatalf("create-budget=%d body=%s", st, e.raw)
	}

	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.User(fixture.User{ID: otherUserID, Name: "Other"})
	f.BudgetAccess(labelBudgetID, otherUserID, 1, true) // role=user, accepted
	collabAcctID := "aaaa5555-0000-7000-8000-000000000005"
	f.Account(fixture.Account{ID: collabAcctID, UserID: otherUserID, CurrencyID: usdID, Name: "Collab"})
	f.BudgetAccount(labelBudgetID, collabAcctID) // the collaborator's account is a budget member
	f.Label(fixture.Label{ID: labelAID, UserID: otherUserID, Name: "Their Label"})

	txID := f.Transaction(fixture.Transaction{
		ID: "ffff5555-0000-7000-8000-000000000002", UserID: otherUserID, AccountID: collabAcctID,
		Type: 0, Amount: "30.00", SpentAt: "2024-04-12 00:00:00",
	})
	if _, err := h.db.Exec(`INSERT INTO transactions_labels (transaction_id, label_id) VALUES (?, ?)`, txID, labelAID); err != nil {
		t.Fatalf("attach label: %v", err)
	}

	// Viewed by the budget owner (seed user), not the collaborator.
	_, b := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+labelBudgetID+"&date=2024-04-15", tok, nil)
	res := mustUnmarshal[labelBudgetView](t, b.Data)

	var found bool
	for _, l := range res.Item.Structure.Labels {
		if l.Id == labelAID {
			found = true
			if l.Spent != "30" {
				t.Errorf("collaborator label spent=%q want 30", l.Spent)
			}
			if l.OwnerUserId != otherUserID {
				t.Errorf("collaborator label ownerUserId=%q want %q", l.OwnerUserId, otherUserID)
			}
		}
	}
	if !found {
		t.Fatalf("collaborator's label must appear in the owner's budget view; body: %s", b.Data)
	}
}

// TestLabelBlockIsEmptyListNotNull: the block is always a list, even with no
// labels at all, so the wire carries [] rather than null.
func TestLabelBlockIsEmptyListNotNull(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(labelBudgetID, "Label Budget")); st != http.StatusOK {
		t.Fatalf("create-budget=%d body=%s", st, e.raw)
	}

	_, b := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+labelBudgetID+"&date=2026-08-15", tok, nil)
	// Raw substring check, not just decode-and-len: a nil slice also decodes to
	// a zero-length slice, so only the wire bytes distinguish "[]" from "null".
	if !bytes.Contains(b.raw, []byte(`"labels":[]`)) {
		t.Fatalf(`expected "labels":[] in response, got: %s`, b.raw)
	}
}

// TestLabelBlockNeverCreatesBudgetsElementsRows is the load-bearing invariant
// guard: labels are budget-neutral and must never become budgets_elements rows
// (no limits, no envelope-math participation). Labels have no set-limit
// endpoint of their own, so the realistic regression is set-limit accepting a
// label id as an ordinary element's externalId (elements are looked up and, on
// a miss, self-healed/created by external id — see getElementSelfHeal in
// internal/budget/accounts.go). This test drives exactly that path with a
// label id that has real period spend (so a leak would have a row to create),
// and asserts BOTH that set-limit rejects it and that no budgets_elements row
// for that id exists afterward.
func TestLabelBlockNeverCreatesBudgetsElementsRows(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": labelBudgetID, "name": "Label Budget", "currencyId": usdID, "startDate": "2024-04-01", "accountIds": []string{accountID}}); st != http.StatusOK {
		t.Fatalf("create-budget=%d body=%s", st, e.raw)
	}

	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Label(fixture.Label{ID: labelAID, UserID: seedUserID, Name: "Kid A"})
	txID := f.Transaction(fixture.Transaction{
		ID: "ffff5555-0000-7000-8000-000000000003", UserID: seedUserID, AccountID: accountID,
		CategoryID: catID, Type: 0, Amount: "20.00", SpentAt: "2024-04-11 00:00:00",
	})
	if _, err := h.db.Exec(`INSERT INTO transactions_labels (transaction_id, label_id) VALUES (?, ?)`, txID, labelAID); err != nil {
		t.Fatalf("attach label: %v", err)
	}

	// Precondition: the label genuinely has period spend and shows in the
	// labels block (a leak-free budgets_elements table would otherwise prove
	// nothing — the label might just be invisible altogether).
	_, b := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+labelBudgetID+"&date=2024-04-15", tok, nil)
	res := mustUnmarshal[labelBudgetView](t, b.Data)
	if _, ok := res.findLabel(labelAID); !ok {
		t.Fatalf("precondition: label should appear with its period spend; body: %s", b.Data)
	}

	// The write attempt: pass the label id as an ordinary element's elementId.
	// getElementSelfHeal only ever creates rows for envelope/category/tag
	// participant entities (internal/budget/move.go's restoreElementsOrder), so
	// a label id must resolve to "BudgetElement not found", not silently seed a
	// budgets_elements row.
	st, env := h.do(t, http.MethodPost, "/api/v1/budget/set-limit", tok, map[string]any{
		"budgetId": labelBudgetID, "elementId": labelAID, "period": "2024-04-01", "amount": "10",
	})
	if st != http.StatusBadRequest {
		t.Fatalf("set-limit with a label id as elementId = %d, want 400 (rejection); body=%s", st, env.raw)
	}
	if env.Message != "BudgetElement not found" {
		t.Errorf("set-limit with a label id message=%q want %q", env.Message, "BudgetElement not found")
	}

	var count int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM budgets_elements WHERE external_id = ?`, labelAID).Scan(&count); err != nil {
		t.Fatalf("query budgets_elements: %v", err)
	}
	if count != 0 {
		t.Fatalf("a label must never create a budgets_elements row; found %d row(s) for label id %s", count, labelAID)
	}
}

// TestLabelBlockOrderedByPosition guards the spec's "position order"
// requirement (internal/budget/builder_structure_build.go's sortByPositionThenID
// call for labels). f.labels is a map, so an unsorted emission would come out in
// Go's randomized map-iteration order. The lower position is deliberately
// assigned to the LEXICALLY HIGHER-sorting id (and vice versa) so a sort that
// silently fell back to id order could not pass this by coincidence.
func TestLabelBlockOrderedByPosition(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": labelBudgetID, "name": "Label Budget", "currencyId": usdID, "startDate": "2024-04-01", "accountIds": []string{accountID}}); st != http.StatusOK {
		t.Fatalf("create-budget=%d body=%s", st, e.raw)
	}

	const (
		idHigh = "eeeeaaaa-0000-7000-8000-000000000003" // lexically highest id, position 0
		idMid  = "eeeeaaaa-0000-7000-8000-000000000002" // position 1
		idLow  = "eeeeaaaa-0000-7000-8000-000000000001" // lexically lowest id, position 2
	)
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Label(fixture.Label{ID: idHigh, UserID: seedUserID, Name: "High", Position: 0})
	f.Label(fixture.Label{ID: idMid, UserID: seedUserID, Name: "Mid", Position: 1})
	f.Label(fixture.Label{ID: idLow, UserID: seedUserID, Name: "Low", Position: 2})

	// Every label needs period spend to be visible at all.
	txHigh := f.Transaction(fixture.Transaction{ID: "ffffaaaa-0000-7000-8000-000000000001", UserID: seedUserID, AccountID: accountID, CategoryID: catID, Type: 0, Amount: "1.00", SpentAt: "2024-04-10 00:00:00"})
	txMid := f.Transaction(fixture.Transaction{ID: "ffffaaaa-0000-7000-8000-000000000002", UserID: seedUserID, AccountID: accountID, CategoryID: catID, Type: 0, Amount: "2.00", SpentAt: "2024-04-11 00:00:00"})
	txLow := f.Transaction(fixture.Transaction{ID: "ffffaaaa-0000-7000-8000-000000000003", UserID: seedUserID, AccountID: accountID, CategoryID: catID, Type: 0, Amount: "3.00", SpentAt: "2024-04-12 00:00:00"})
	for txID, labelID := range map[string]string{txHigh: idHigh, txMid: idMid, txLow: idLow} {
		if _, err := h.db.Exec(`INSERT INTO transactions_labels (transaction_id, label_id) VALUES (?, ?)`, txID, labelID); err != nil {
			t.Fatalf("attach label: %v", err)
		}
	}

	_, b := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+labelBudgetID+"&date=2024-04-15", tok, nil)
	res := mustUnmarshal[labelBudgetView](t, b.Data)

	if len(res.Item.Structure.Labels) != 3 {
		t.Fatalf("want 3 labels, got %d; body: %s", len(res.Item.Structure.Labels), b.Data)
	}
	got := []string{res.Item.Structure.Labels[0].Id, res.Item.Structure.Labels[1].Id, res.Item.Structure.Labels[2].Id}
	want := []string{idHigh, idMid, idLow}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("labels emitted in order %v, want %v (position order, id descending here on purpose)", got, want)
		}
	}
}

// TestLabelBlockIsArchivedReflectsArchivedFlag pins the isArchived int mapping
// on LabelSpendResult: boolToInt(meta.IsArchived) could be silently inverted
// and every other test in this file (which use non-archived labels) would stay
// green.
func TestLabelBlockIsArchivedReflectsArchivedFlag(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": labelBudgetID, "name": "Label Budget", "currencyId": usdID, "startDate": "2024-04-01", "accountIds": []string{accountID}}); st != http.StatusOK {
		t.Fatalf("create-budget=%d body=%s", st, e.raw)
	}

	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Label(fixture.Label{ID: labelAID, UserID: seedUserID, Name: "Archived", Archived: true})
	f.Label(fixture.Label{ID: labelBID, UserID: seedUserID, Name: "Active"})

	txArchived := f.Transaction(fixture.Transaction{
		ID: "ffff7777-0000-7000-8000-000000000001", UserID: seedUserID, AccountID: accountID,
		CategoryID: catID, Type: 0, Amount: "5.00", SpentAt: "2024-04-10 00:00:00",
	})
	txActive := f.Transaction(fixture.Transaction{
		ID: "ffff7777-0000-7000-8000-000000000002", UserID: seedUserID, AccountID: accountID,
		CategoryID: catID, Type: 0, Amount: "7.00", SpentAt: "2024-04-11 00:00:00",
	})
	if _, err := h.db.Exec(`INSERT INTO transactions_labels (transaction_id, label_id) VALUES (?, ?)`, txArchived, labelAID); err != nil {
		t.Fatalf("attach archived label: %v", err)
	}
	if _, err := h.db.Exec(`INSERT INTO transactions_labels (transaction_id, label_id) VALUES (?, ?)`, txActive, labelBID); err != nil {
		t.Fatalf("attach active label: %v", err)
	}

	_, b := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+labelBudgetID+"&date=2024-04-15", tok, nil)
	res := mustUnmarshal[labelBudgetView](t, b.Data)

	var foundArchived, foundActive bool
	var gotArchived, gotActive int
	for _, l := range res.Item.Structure.Labels {
		switch l.Id {
		case labelAID:
			foundArchived, gotArchived = true, l.IsArchived
		case labelBID:
			foundActive, gotActive = true, l.IsArchived
		}
	}
	if !foundArchived {
		t.Fatalf("an archived label with period spend must still appear; body: %s", b.Data)
	}
	if !foundActive {
		t.Fatalf("the active label must appear; body: %s", b.Data)
	}
	if gotArchived != 1 {
		t.Errorf("archived label isArchived=%d want 1", gotArchived)
	}
	if gotActive != 0 {
		t.Errorf("active label isArchived=%d want 0", gotActive)
	}
}
