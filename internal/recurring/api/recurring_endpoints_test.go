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

func createRecurringReq(opID, typ, amount string) map[string]any {
	return map[string]any{
		"id": opID, "type": typ, "amount": amount,
		"accountId": accountID, // use the harness's seeded account id variable
		"schedule":  "monthly", "nextPaymentAt": "2026-08-31 00:00:00",
		"description": "rent",
	}
}

func TestCreateRecurringTransaction_Success(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	const opID = "0197c100-0000-7000-8000-000000000001"
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/create-recurring-transaction", tok, createRecurringReq(opID, "expense", "42.50"))
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, env.raw)
	}
	res := mustUnmarshal[struct {
		Item recurringItem `json:"item"`
	}](t, env.Data)
	if res.Item.ID == "" || res.Item.ID == opID {
		t.Fatalf("entity id must be fresh, got %q", res.Item.ID)
	}
	if res.Item.Schedule != "monthly" || res.Item.NextPaymentAt != "2026-08-31 00:00:00" || res.Item.Amount != "42.5" {
		t.Fatalf("unexpected item: %+v", res.Item)
	}

	// list now contains it
	_, listEnv := h.do(t, http.MethodGet, "/api/v1/recurring/get-recurring-transaction-list", tok, nil)
	list := mustUnmarshal[recurringList](t, listEnv.Data)
	if len(list.Items) != 1 {
		t.Fatalf("list has %d items, want 1", len(list.Items))
	}
}

func TestCreateRecurringTransaction_IdempotencyReplayLocked(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	const opID = "0197c100-0000-7000-8000-000000000002"
	h.do(t, http.MethodPost, "/api/v1/recurring/create-recurring-transaction", tok, createRecurringReq(opID, "expense", "10"))
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/create-recurring-transaction", tok, createRecurringReq(opID, "expense", "10"))
	if status != http.StatusBadRequest {
		t.Fatalf("replay status=%d body=%s", status, env.raw)
	}
}

func TestCreateRecurringTransaction_TransferRequiresRecipient(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	req := createRecurringReq("0197c100-0000-7000-8000-000000000003", "transfer", "10")
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/create-recurring-transaction", tok, req)
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", status, env.raw)
	}
	if _, ok := env.errorsMap()["accountRecipientId"]; !ok {
		t.Fatalf("expected accountRecipientId field error, got %s", env.raw)
	}
}

func TestCreateRecurringTransaction_BadSchedule(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	req := createRecurringReq("0197c100-0000-7000-8000-000000000004", "expense", "10")
	req["schedule"] = "daily"
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/create-recurring-transaction", tok, req)
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", status, env.raw)
	}
}

func createTemplate(t *testing.T, h *harness, tok string) recurringItem {
	t.Helper()
	const opID = "0197c100-0000-7000-8000-000000000005"
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/create-recurring-transaction", tok, createRecurringReq(opID, "expense", "42.50"))
	if status != http.StatusOK {
		t.Fatalf("create failed: %s", env.raw)
	}
	return mustUnmarshal[struct {
		Item recurringItem `json:"item"`
	}](t, env.Data).Item
}

func TestUpdateRecurringTransaction_Success(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	item := createTemplate(t, h, tok)
	body := map[string]any{
		"id": item.ID, "type": "expense", "amount": "99",
		"accountId": accountID, "schedule": "weekly",
		"nextPaymentAt": "2026-09-05 00:00:00", "description": "updated",
	}
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/update-recurring-transaction", tok, body)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, env.raw)
	}
	res := mustUnmarshal[struct {
		Item recurringItem `json:"item"`
	}](t, env.Data)
	if res.Item.Schedule != "weekly" || res.Item.Amount != "99" || res.Item.NextPaymentAt != "2026-09-05 00:00:00" {
		t.Fatalf("unexpected item: %+v", res.Item)
	}
}

func TestUpdateRecurringTransaction_NotFound(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	body := map[string]any{
		"id": "0197c1aa-0000-7000-8000-000000000099", "type": "expense", "amount": "1",
		"accountId": accountID, "schedule": "weekly", "nextPaymentAt": "2026-09-05 00:00:00",
	}
	status, _ := h.do(t, http.MethodPost, "/api/v1/recurring/update-recurring-transaction", tok, body)
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", status)
	}
}

func TestDeleteRecurringTransaction_Success(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	item := createTemplate(t, h, tok)
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/delete-recurring-transaction", tok, map[string]any{"id": item.ID})
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, env.raw)
	}
	_, listEnv := h.do(t, http.MethodGet, "/api/v1/recurring/get-recurring-transaction-list", tok, nil)
	if list := mustUnmarshal[recurringList](t, listEnv.Data); len(list.Items) != 0 {
		t.Fatalf("template still listed after delete")
	}
}
