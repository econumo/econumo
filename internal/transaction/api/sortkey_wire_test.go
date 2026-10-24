package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestTransactionsCarrySortKeys(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	create := func(id string) map[string]any {
		t.Helper()
		st, e := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok, map[string]any{
			"id": id, "type": "expense", "amount": "1", "accountId": accountID, "date": "2024-03-02 10:00:00",
		})
		if st != http.StatusOK {
			t.Fatalf("create = %d; body=%s", st, e.raw)
		}
		var res struct {
			Item map[string]any `json:"item"`
		}
		if err := json.Unmarshal(e.Data, &res); err != nil {
			t.Fatal(err)
		}
		return res.Item
	}
	first := create("bbbb1111-0000-7000-8000-0000000005e1")
	second := create("bbbb1111-0000-7000-8000-0000000005e2")
	if first["sortKey"] != "a0" || second["sortKey"] != "a1" {
		t.Errorf("sortKey first=%v second=%v, want a0 and a1", first["sortKey"], second["sortKey"])
	}
	if v, ok := first["recipientSortKey"]; !ok || v != nil {
		t.Errorf("recipientSortKey = %v (present %v), want null", v, ok)
	}

	_, e := h.do(t, http.MethodGet, "/api/v1/transaction/get-account-transaction-list?accountIds="+accountID, tok, nil)
	var list struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(e.Data, &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) < 2 || list.Items[0]["id"] != second["id"] || list.Items[0]["sortKey"] != "a1" {
		t.Errorf("account list = %v, want the second row first with sortKey a1", list.Items)
	}
}
