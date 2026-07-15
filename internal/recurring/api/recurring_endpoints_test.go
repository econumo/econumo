package api_test

import (
	"net/http"
	"testing"
)

type recurringItem struct {
	ID            string  `json:"id"`
	Type          string  `json:"type"`
	AccountID     string  `json:"accountId"`
	Amount        string  `json:"amount"`
	Schedule      string  `json:"schedule"`
	NextPaymentAt string  `json:"nextPaymentAt"`
	CategoryID    *string `json:"categoryId"`
	Description   string  `json:"description"`
}

type recurringList struct {
	Items []recurringItem `json:"items"`
}

func TestGetRecurringTransactionList_EmptyByDefault(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	status, env := h.do(t, http.MethodGet, "/api/v1/recurring/get-recurring-transaction-list", tok, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, env.raw)
	}
	res := mustUnmarshal[recurringList](t, env.Data)
	if len(res.Items) != 0 {
		t.Fatalf("expected empty list, got %d", len(res.Items))
	}
}

func TestGetRecurringTransactionList_RequiresAuth(t *testing.T) {
	h := newHarness(t)
	status, _ := h.do(t, http.MethodGet, "/api/v1/recurring/get-recurring-transaction-list", "", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401", status)
	}
}
