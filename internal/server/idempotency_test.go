package server_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	appuser "github.com/econumo/econumo/internal/user"
)

const (
	createTransactionPath = "/api/v1/transaction/create-transaction"
	operationLocked       = "Operation is locked"
)

type idempotencyHarness struct {
	t  *testing.T
	db *dbtest.DB
	f  *fixture.Builder
	ts *httptest.Server
}

func newIdempotencyHarness(t *testing.T) idempotencyHarness {
	t.Helper()
	db := dbtest.NewSQLite(t)
	cfg := config.Config{
		DatabaseDriver:  db.Engine,
		CurrencyBase:    "USD",
		RateLimitWindow: 15 * time.Minute,
	}
	ts := httptest.NewServer(server.BuildAPI(cfg, db.Raw, server.Seams{Avatars: appuser.FixedAvatarPicker(appuser.DefaultAvatar)}))
	t.Cleanup(ts.Close)
	return idempotencyHarness{t: t, db: db, f: fixture.New(t, db), ts: ts}
}

// seedUser creates a user owning one account and returns its bearer token and
// account id.
func (h idempotencyHarness) seedUser() (token, userID, accountID string) {
	h.t.Helper()
	// Random, not a UUIDv7: the fixture derives emails and option ids from the
	// id's first eight characters, which two v7 ids minted together share.
	userID = h.f.User(fixture.User{ID: uuid.NewString()})
	h.f.DefaultOptions(userID)
	accountID = h.f.Account(fixture.Account{UserID: userID})
	token = "eco_ses_" + uuid.NewString()
	exp := time.Now().UTC().Add(24 * time.Hour)
	h.f.AccessToken(fixture.AccessToken{
		UserID:    userID,
		Kind:      "session",
		TokenHash: appuser.HashAccessToken(token),
		UserAgent: "idempotency-test",
		ExpiresAt: &exp,
	})
	return token, userID, accountID
}

func (h idempotencyHarness) do(method, path, token, key, body string) (int, map[string]any) {
	h.t.Helper()
	req, err := http.NewRequest(method, h.ts.URL+path, strings.NewReader(body))
	if err != nil {
		h.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		h.t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		h.t.Fatalf("%s %s: response is not JSON: %q", method, path, raw)
	}
	return resp.StatusCode, parsed
}

func (h idempotencyHarness) count(query string, args ...any) int {
	h.t.Helper()
	var n int
	if err := h.db.Raw.QueryRow(h.db.Rebind(query), args...).Scan(&n); err != nil {
		h.t.Fatal(err)
	}
	return n
}

func createBody(accountID string) string {
	return fmt.Sprintf(`{"id":%q,"type":"expense","amount":"10","accountId":%q,"date":"2026-01-02 10:00:00"}`, uuid.NewString(), accountID)
}

func newKey(t *testing.T) string {
	t.Helper()
	k, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return k.String()
}

func wantStatus(t *testing.T, got int, body map[string]any, want int) {
	t.Helper()
	if got != want {
		t.Fatalf("status = %d, want %d; body: %v", got, want, body)
	}
}

func wantReplay(t *testing.T, status int, body map[string]any) {
	t.Helper()
	wantStatus(t, status, body, http.StatusConflict)
	if body["success"] != false || body["message"] != operationLocked || body["code"] != float64(http.StatusConflict) {
		t.Fatalf("replay envelope = %v", body)
	}
	if errors, ok := body["errors"].(map[string]any); !ok || len(errors) != 0 {
		t.Fatalf("replay errors = %v, want {}", body["errors"])
	}
}

func TestReplayedCreateIsRefusedAndCreatesOneRow(t *testing.T) {
	h := newIdempotencyHarness(t)
	token, _, accountID := h.seedUser()
	key, body := newKey(t), createBody(accountID)

	status, resp := h.do(http.MethodPost, createTransactionPath, token, key, body)
	wantStatus(t, status, resp, http.StatusOK)

	status, resp = h.do(http.MethodPost, createTransactionPath, token, key, body)
	wantReplay(t, status, resp)

	if n := h.count("SELECT COUNT(*) FROM transactions WHERE account_id = ?", accountID); n != 1 {
		t.Fatalf("transactions = %d, want 1", n)
	}
}

func TestReplayWithANewBodyIdIsStillRefused(t *testing.T) {
	h := newIdempotencyHarness(t)
	token, _, accountID := h.seedUser()
	key := newKey(t)

	status, resp := h.do(http.MethodPost, createTransactionPath, token, key, createBody(accountID))
	wantStatus(t, status, resp, http.StatusOK)

	status, resp = h.do(http.MethodPost, createTransactionPath, token, key, createBody(accountID))
	wantReplay(t, status, resp)

	if n := h.count("SELECT COUNT(*) FROM transactions WHERE account_id = ?", accountID); n != 1 {
		t.Fatalf("transactions = %d, want 1", n)
	}
}

func TestNewKeyWithARepeatedBodyIdKeepsItsLockedError(t *testing.T) {
	h := newIdempotencyHarness(t)
	token, userID, accountID := h.seedUser()
	body := createBody(accountID)

	status, resp := h.do(http.MethodPost, createTransactionPath, token, newKey(t), body)
	wantStatus(t, status, resp, http.StatusOK)

	status, resp = h.do(http.MethodPost, createTransactionPath, token, newKey(t), body)
	wantStatus(t, status, resp, http.StatusBadRequest)
	if resp["message"] != operationLocked {
		t.Fatalf("message = %v, want %q", resp["message"], operationLocked)
	}
	if n := h.count("SELECT COUNT(*) FROM idempotency_keys WHERE user_id = ?", userID); n != 1 {
		t.Fatalf("keys = %d, want only the successful request's", n)
	}
}

func TestFailedWriteReleasesTheKey(t *testing.T) {
	h := newIdempotencyHarness(t)
	token, userID, accountID := h.seedUser()
	key := newKey(t)

	status, resp := h.do(http.MethodPost, createTransactionPath, token, key, createBody("not-an-account-id"))
	wantStatus(t, status, resp, http.StatusBadRequest)
	if n := h.count("SELECT COUNT(*) FROM idempotency_keys WHERE user_id = ?", userID); n != 0 {
		t.Fatalf("keys after a failed write = %d, want 0", n)
	}

	status, resp = h.do(http.MethodPost, createTransactionPath, token, key, createBody(accountID))
	wantStatus(t, status, resp, http.StatusOK)
}

func TestReplayedDeleteIsRefusedNotNotFound(t *testing.T) {
	h := newIdempotencyHarness(t)
	token, userID, accountID := h.seedUser()
	txID := h.f.Transaction(fixture.Transaction{UserID: userID, AccountID: accountID, Amount: "5.00000000"})
	key, body := newKey(t), fmt.Sprintf(`{"id":%q}`, txID)

	status, resp := h.do(http.MethodPost, "/api/v1/transaction/delete-transaction", token, key, body)
	wantStatus(t, status, resp, http.StatusOK)

	status, resp = h.do(http.MethodPost, "/api/v1/transaction/delete-transaction", token, key, body)
	wantReplay(t, status, resp)
}

func TestReplayedSetLimitIsRefused(t *testing.T) {
	h := newIdempotencyHarness(t)
	token, userID, _ := h.seedUser()
	categoryID := h.f.Category(fixture.Category{UserID: userID})
	budgetID := h.f.Budget(fixture.Budget{UserID: userID, StartedAt: "2024-01-01 00:00:00"})
	h.f.BudgetElement(fixture.BudgetElement{BudgetID: budgetID, ExternalID: categoryID, Type: 1})
	key := newKey(t)
	body := fmt.Sprintf(`{"budgetId":%q,"elementId":%q,"period":"2024-02-01","amount":"100"}`, budgetID, categoryID)

	status, resp := h.do(http.MethodPost, "/api/v1/budget/set-limit", token, key, body)
	wantStatus(t, status, resp, http.StatusOK)

	status, resp = h.do(http.MethodPost, "/api/v1/budget/set-limit", token, key, body)
	wantReplay(t, status, resp)
}

func TestKeyIsPerUser(t *testing.T) {
	h := newIdempotencyHarness(t)
	tokenA, _, accountA := h.seedUser()
	tokenB, _, accountB := h.seedUser()
	key := newKey(t)

	status, resp := h.do(http.MethodPost, createTransactionPath, tokenA, key, createBody(accountA))
	wantStatus(t, status, resp, http.StatusOK)

	status, resp = h.do(http.MethodPost, createTransactionPath, tokenB, key, createBody(accountB))
	wantStatus(t, status, resp, http.StatusOK)
}

func TestNoKeyAndMalformedKeyAreUnguarded(t *testing.T) {
	h := newIdempotencyHarness(t)
	token, userID, accountID := h.seedUser()

	for _, key := range []string{"", "not-a-uuid"} {
		for range 2 {
			status, resp := h.do(http.MethodPost, createTransactionPath, token, key, createBody(accountID))
			wantStatus(t, status, resp, http.StatusOK)
		}
	}
	if n := h.count("SELECT COUNT(*) FROM idempotency_keys WHERE user_id = ?", userID); n != 0 {
		t.Fatalf("keys = %d, want 0", n)
	}
}

func TestReadsArePassedThrough(t *testing.T) {
	h := newIdempotencyHarness(t)
	token, userID, _ := h.seedUser()
	key := newKey(t)

	for range 2 {
		status, resp := h.do(http.MethodGet, "/api/v1/user/get-user-data", token, key, "")
		wantStatus(t, status, resp, http.StatusOK)
	}
	if n := h.count("SELECT COUNT(*) FROM idempotency_keys WHERE user_id = ?", userID); n != 0 {
		t.Fatalf("keys = %d, want 0", n)
	}
}

// update-language is a single UPDATE, with no WithTx anywhere on its path.
func TestHandlerWithoutTransactionRecordsAfterSuccess(t *testing.T) {
	h := newIdempotencyHarness(t)
	token, userID, _ := h.seedUser()
	key := newKey(t)

	status, resp := h.do(http.MethodPost, "/api/v1/user/update-language", token, key, `{"language":"en"}`)
	wantStatus(t, status, resp, http.StatusOK)
	if n := h.count("SELECT COUNT(*) FROM idempotency_keys WHERE user_id = ?", userID); n != 1 {
		t.Fatalf("keys = %d, want 1", n)
	}

	status, resp = h.do(http.MethodPost, "/api/v1/user/update-language", token, key, `{"language":"en"}`)
	wantReplay(t, status, resp)
}

func bodyWithoutId(accountID string) string {
	return fmt.Sprintf(`{"type":"expense","amount":"10","accountId":%q,"date":"2026-01-02 10:00:00"}`, accountID)
}

func itemID(t *testing.T, body map[string]any) string {
	t.Helper()
	data, _ := body["data"].(map[string]any)
	item, _ := data["item"].(map[string]any)
	id, _ := item["id"].(string)
	if id == "" {
		t.Fatalf("no item.id in %v", body)
	}
	return id
}

func TestCreateWithoutIdMintsAnId(t *testing.T) {
	h := newIdempotencyHarness(t)
	token, _, accountID := h.seedUser()
	status, resp := h.do(http.MethodPost, createTransactionPath, token, "", bodyWithoutId(accountID))
	wantStatus(t, status, resp, http.StatusOK)
	itemID(t, resp)
}

func TestCreateWithIdKeepsItsLockedReplay(t *testing.T) {
	h := newIdempotencyHarness(t)
	token, _, accountID := h.seedUser()
	body := createBody(accountID)
	status, resp := h.do(http.MethodPost, createTransactionPath, token, "", body)
	wantStatus(t, status, resp, http.StatusOK)
	status, resp = h.do(http.MethodPost, createTransactionPath, token, "", body)
	wantStatus(t, status, resp, http.StatusBadRequest)
	if resp["message"] != operationLocked {
		t.Fatalf("message = %v", resp["message"])
	}
}

func TestReplayReturnsResourceIdForCreateWithoutBodyId(t *testing.T) {
	h := newIdempotencyHarness(t)
	token, _, accountID := h.seedUser()
	key, body := newKey(t), bodyWithoutId(accountID)
	status, first := h.do(http.MethodPost, createTransactionPath, token, key, body)
	wantStatus(t, status, first, http.StatusOK)
	status, resp := h.do(http.MethodPost, createTransactionPath, token, key, body)
	wantReplay(t, status, resp)
	if resp["resourceId"] != itemID(t, first) {
		t.Fatalf("resourceId = %v, want %s", resp["resourceId"], itemID(t, first))
	}
	if n := h.count("SELECT COUNT(*) FROM transactions WHERE account_id = ?", accountID); n != 1 {
		t.Fatalf("transactions = %d, want 1", n)
	}
}
