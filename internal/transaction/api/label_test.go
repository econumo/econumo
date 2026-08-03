package api_test

// labelIds coverage on the transaction write endpoints: a genuine round trip
// (create -> the response AND a subsequent get-transaction-list both carry the
// same ids in the same, DB-sorted order), the full-replace clear semantics on
// update, transfer's "always empty" rule, dedupe, and the two rejection paths
// (unknown id, foreign-owned id) that keep labelIds from being a way to attach
// or discover another user's data.

import (
	"net/http"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// maxTestLabels mirrors internal/transaction/usecase.go's unexported
// maxTransactionLabels (50) — this test package cannot see it directly, so
// the boundary is duplicated here; keep the two in sync.
const maxTestLabels = 50

func createReqWithLabels(id, typ, amount string, labelIDs []string) map[string]any {
	return map[string]any{
		"id": id, "type": typ, "amount": amount, "accountId": accountID, "categoryId": catID,
		"date": "2024-03-01 10:00:00", "description": "groceries", "labelIds": labelIDs,
	}
}

// TestCreateTransaction_WithLabels_RoundTrip is the genuine round trip R1
// calls for: the create response carries both ids in ascending-string order
// (label1ID < label2ID) even though the request lists them reversed, and a
// subsequent get-transaction-list read of the PERSISTED row shows the exact
// same two ids in the same order — proving the join-table write actually
// happened, not just that the in-memory response looked right.
func TestCreateTransaction_WithLabels_RoundTrip(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok,
		createReqWithLabels(txID1, "expense", "12.00", []string{label2ID, label1ID}))
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body: %s", status, env.raw)
	}
	if !strings.Contains(string(env.raw), `"labelIds":["`+label1ID+`","`+label2ID+`"]`) {
		t.Fatalf("raw body missing sorted labelIds array; body: %s", env.raw)
	}
	res := mustUnmarshal[writeResult](t, env.Data)
	if len(res.Item.LabelIds) != 2 || res.Item.LabelIds[0] != label1ID || res.Item.LabelIds[1] != label2ID {
		t.Fatalf("create labelIds = %v, want [%s %s] (sorted)", res.Item.LabelIds, label1ID, label2ID)
	}

	_, listEnv := h.do(t, http.MethodGet, "/api/v1/transaction/get-transaction-list?accountId="+accountID, tok, nil)
	list := mustUnmarshal[listResult](t, listEnv.Data)
	if len(list.Items) != 1 {
		t.Fatalf("list = %+v, want one item", list.Items)
	}
	if got := list.Items[0].LabelIds; len(got) != 2 || got[0] != label1ID || got[1] != label2ID {
		t.Fatalf("persisted list labelIds = %v, want [%s %s]", got, label1ID, label2ID)
	}
}

// TestCreateTransaction_DuplicateLabelIds_Deduped: the same id repeated in the
// request must land exactly once.
func TestCreateTransaction_DuplicateLabelIds_Deduped(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok,
		createReqWithLabels(txID1, "expense", "12.00", []string{label1ID, label1ID, label2ID}))
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[writeResult](t, env.Data)
	if len(res.Item.LabelIds) != 2 {
		t.Fatalf("labelIds = %v, want exactly 2 (deduped), got %d", res.Item.LabelIds, len(res.Item.LabelIds))
	}
}

// TestUpdateTransaction_LabelIds_Clears: update-transaction is a full-state
// replace (same rule as categoryId), so an explicit "labelIds": [] clears a
// previously-set label list, and the clear persists (checked via the list
// endpoint, not just the update response).
func TestUpdateTransaction_LabelIds_Clears(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	_, cEnv := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok,
		createReqWithLabels(txID1, "expense", "12.00", []string{label1ID, label2ID}))
	created := mustUnmarshal[writeResult](t, cEnv.Data)
	if len(created.Item.LabelIds) != 2 {
		t.Fatalf("labelIds before clearing = %v, want 2", created.Item.LabelIds)
	}

	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/update-transaction", tok, map[string]any{
		"id": created.Item.ID, "type": "expense", "amount": "12.00", "accountId": accountID, "categoryId": catID,
		"date": "2024-03-01 10:00:00", "description": "groceries", "labelIds": []string{},
	})
	if status != http.StatusOK {
		t.Fatalf("update=%d want 200; body: %s", status, env.raw)
	}
	if !strings.Contains(string(env.raw), `"labelIds":[]`) {
		t.Fatalf("raw body labelIds not an empty array; body: %s", env.raw)
	}
	res := mustUnmarshal[writeResult](t, env.Data)
	if res.Item.LabelIds == nil || len(res.Item.LabelIds) != 0 {
		t.Fatalf("labelIds after clearing = %v, want [] (non-nil, empty)", res.Item.LabelIds)
	}

	_, listEnv := h.do(t, http.MethodGet, "/api/v1/transaction/get-transaction-list?accountId="+accountID, tok, nil)
	list := mustUnmarshal[listResult](t, listEnv.Data)
	if len(list.Items) != 1 || len(list.Items[0].LabelIds) != 0 {
		t.Fatalf("persisted list labelIds = %+v, want one item with none", list.Items)
	}
}

// TestCreateTransfer_LabelIds_Ignored: a transfer never carries labels; the
// request's labelIds is silently dropped rather than rejected, and the
// response shows an empty (not null) list.
func TestCreateTransfer_LabelIds_Ignored(t *testing.T) {
	h := newHarness(t)
	h.seedTransferAccounts(t)
	tok := h.token(t)
	body := transferReq(txID1, accountID, usd2AcctID, "10", "10")
	body["labelIds"] = []string{label1ID, label2ID}
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok, body)
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body: %s", status, env.raw)
	}
	if !strings.Contains(string(env.raw), `"labelIds":[]`) {
		t.Fatalf("raw body labelIds not empty for a transfer; body: %s", env.raw)
	}
	res := mustUnmarshal[writeResult](t, env.Data)
	if res.Item.LabelIds == nil || len(res.Item.LabelIds) != 0 {
		t.Fatalf("transfer labelIds = %v, want [] (ignored)", res.Item.LabelIds)
	}
}

// TestCreateTransaction_UnknownLabel_Rejected: a well-formed but non-existent
// label id is rejected exactly like an unknown category/payee/tag.
func TestCreateTransaction_UnknownLabel_Rejected(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok,
		createReqWithLabels(txID1, "expense", "12.00", []string{"dddd3333-0000-0000-0000-0000000000dd"}))
	assertValidationDenied(t, status, env, "This transaction is not available for this operation.")
}

// TestCreateTransaction_ForeignLabel_Rejected: a label owned by a user who
// shares no account with the caller must not be attachable — labelIds is not
// a way to probe or borrow another user's classification.
func TestCreateTransaction_ForeignLabel_Rejected(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok,
		createReqWithLabels(txID1, "expense", "12.00", []string{otherLabelID}))
	assertValidationDenied(t, status, env, "This transaction is not available for this operation.")
}

// TestCreateTransaction_SharedAccount_UsesOwnerLabel_Succeeds mirrors
// TestCreateTransaction_SharedAccount_UsesOwnerCategory_Succeeds: on a shared
// account, the caller classifies with the ACCOUNT OWNER's label, not their
// own — a label must belong to the account owner specifically, matching how
// shared-account category/payee/tag already work.
func TestCreateTransaction_SharedAccount_UsesOwnerLabel_Succeeds(t *testing.T) {
	h := newHarness(t)
	h.shareAccount(t, roleUser, true)
	const ownerLabelID = "cccc2222-0000-0000-0000-00000000c2cc"
	txm := backend.NewTxManager(h.db)
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite", TX: txm}).WithCrypto(testDataSalt)
	f.Label(fixture.Label{ID: ownerLabelID, UserID: ownerTwoID, Name: "Owner Label"})

	tok := h.token(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok, map[string]any{
		"id": txID1, "type": "expense", "amount": "10", "accountId": sharedAcctID, "categoryId": ownerCatID,
		"date": "2024-03-01 10:00:00", "description": "shared", "labelIds": []string{ownerLabelID},
	})
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[writeResult](t, env.Data)
	if len(res.Item.LabelIds) != 1 || res.Item.LabelIds[0] != ownerLabelID {
		t.Fatalf("labelIds = %v, want [%s]", res.Item.LabelIds, ownerLabelID)
	}
}

// TestCreateTransaction_TooManyLabels_Rejected: one over the cap is rejected
// with the dedicated too-many-labels error, BEFORE any per-id ownership
// lookup runs — none of these ids need to exist for the cap to fire, since
// resolveLabels checks the deduped count first.
func TestCreateTransaction_TooManyLabels_Rejected(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	ids := make([]string, maxTestLabels+1)
	for i := range ids {
		ids[i] = fixture.NewID()
	}
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok,
		createReqWithLabels(txID1, "expense", "12.00", ids))
	assertValidationDenied(t, status, env, "A transaction can have at most 50 labels.")
}

// TestCreateTransaction_MaxLabels_Accepted: exactly the cap succeeds, proving
// the check is an off-by-one-safe ">", not ">=".
func TestCreateTransaction_MaxLabels_Accepted(t *testing.T) {
	h := newHarness(t)
	txm := backend.NewTxManager(h.db)
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite", TX: txm}).WithCrypto(testDataSalt)
	ids := make([]string, maxTestLabels)
	for i := range ids {
		ids[i] = f.Label(fixture.Label{UserID: seedUserID, Name: fixture.NewID()})
	}

	tok := h.token(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok,
		createReqWithLabels(txID1, "expense", "12.00", ids))
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[writeResult](t, env.Data)
	if len(res.Item.LabelIds) != maxTestLabels {
		t.Fatalf("labelIds count = %d, want %d", len(res.Item.LabelIds), maxTestLabels)
	}
}

// TestDeleteTransaction_ReturnsLabelsFromBeforeDelete: the delete response
// must reflect the labels the transaction actually had, not an empty list —
// a naive post-delete label lookup would always come back empty because
// transactions_labels rows cascade away with the transaction row.
func TestDeleteTransaction_ReturnsLabelsFromBeforeDelete(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	_, cEnv := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok,
		createReqWithLabels(txID1, "expense", "12.00", []string{label1ID}))
	created := mustUnmarshal[writeResult](t, cEnv.Data)

	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/delete-transaction", tok, map[string]any{"id": created.Item.ID})
	if status != http.StatusOK {
		t.Fatalf("delete=%d want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[writeResult](t, env.Data)
	if len(res.Item.LabelIds) != 1 || res.Item.LabelIds[0] != label1ID {
		t.Fatalf("deleted item labelIds = %v, want [%s] (from before delete)", res.Item.LabelIds, label1ID)
	}
}
