package api_test

// recurring_id records which recurring template a transaction was posted from,
// so clients can tell a materialized instance apart from a hand-entered one.
// Two properties matter beyond the happy path (which the apiparity golden
// covers): the field is server-owned, so a client cannot claim a provenance it
// does not have, and the key is always emitted so the SPA can branch on it.

import (
	"bytes"
	"net/http"
	"testing"
)

// createTxBody builds a minimal non-transfer create-transaction body, merging
// any extra keys (used here to smuggle in a forged recurringId).
func createTxBody(opID string, extra map[string]any) map[string]any {
	body := map[string]any{
		"id": opID, "type": "expense", "amount": "10.00",
		"accountId": accountID, "categoryId": catID,
		"date": "2026-07-01 10:00:00", "description": "lunch",
	}
	for k, v := range extra {
		body[k] = v
	}
	return body
}

type recurringIDItem struct {
	Item struct {
		ID          string  `json:"id"`
		RecurringId *string `json:"recurringId"`
	} `json:"item"`
}

func TestCreateTransactionIgnoresClientSuppliedRecurringId(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	// A forged recurringId in the create body must not reach the row: provenance
	// is written only by the post-recurring-transaction path and the explicit
	// create-from-transaction link.
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok,
		createTxBody("e0000000-0000-0000-0000-0000000000f1", map[string]any{
			"recurringId": "bbbbbbbb-0000-0000-0000-00000000bbbb",
		}))
	if status != http.StatusOK {
		t.Fatalf("create status = %d, want 200 (body: %s)", status, env.raw)
	}

	got := mustUnmarshal[recurringIDItem](t, env.Data)
	if got.Item.RecurringId != nil {
		t.Errorf("recurringId = %q, want null (a client must not be able to forge provenance)", *got.Item.RecurringId)
	}
}

func TestTransactionResultAlwaysCarriesRecurringIdKey(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok,
		createTxBody("e0000000-0000-0000-0000-0000000000f2", nil))
	if status != http.StatusOK {
		t.Fatalf("create status = %d, want 200 (body: %s)", status, env.raw)
	}
	// The SPA branches on this key, so it must be emitted as null for a
	// hand-entered transaction rather than omitted.
	if !bytes.Contains(env.raw, []byte(`"recurringId":null`)) {
		t.Errorf("response omits the recurringId key; body: %s", env.raw)
	}
}
