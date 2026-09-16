package api_test

import (
	"net/http"
	"testing"
)

// The harness must store what production stores: a time.Time bound through
// the sqlc passthrough lands as the frozen 'Y-m-d H:i:s' text, not the
// driver's default time.Time.String() form.
func TestHarness_StoresFrozenDatetimeLayout(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok, createReq(txID1, "expense", "1"))
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body: %s", status, env.raw)
	}
	var spentAt, createdAt string
	if err := h.db.QueryRow(`SELECT CAST(spent_at AS TEXT), CAST(created_at AS TEXT) FROM transactions`).Scan(&spentAt, &createdAt); err != nil {
		t.Fatal(err)
	}
	if spentAt != "2024-03-01 10:00:00" {
		t.Errorf("spent_at stored as %q, want %q", spentAt, "2024-03-01 10:00:00")
	}
	if len(createdAt) != 19 {
		t.Errorf("created_at stored as %q, want 19-character 'Y-m-d H:i:s'", createdAt)
	}
}
