package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	appimports "github.com/econumo/econumo/internal/imports"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/test/fixture"
)

const bankSource = "0c000000-0000-0000-0000-00000000000b"

type stubProvider struct {
	accounts []model.ExternalAccount
	txs      []model.ExternalTransaction
	err      error
}

func (p *stubProvider) ListAccounts(context.Context, appimports.Credential) ([]model.ExternalAccount, error) {
	return p.accounts, p.err
}
func (p *stubProvider) FetchTransactions(context.Context, appimports.Credential, appimports.FetchRequest) (*appimports.FetchResult, error) {
	if p.err != nil {
		return nil, p.err
	}
	return &appimports.FetchResult{Accounts: p.accounts, Transactions: p.txs}, nil
}
func (p *stubProvider) ClaimSetupToken(_ context.Context, tok string) (string, error) {
	if tok == "used" {
		return "", appimports.ErrSetupTokenRejected
	}
	return "https://u:p@bridge.example/simplefin", nil
}

func call(t *testing.T, h *harness, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, h.srv.URL+path, &buf)
	req.Header.Set("Authorization", "Bearer "+userA)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	var env map[string]any
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("body %s: %v", raw, err)
	}
	return res.StatusCode, env
}

func TestSyncSource_EndToEnd(t *testing.T) {
	h := newHarness(t)
	h.f.ImportSource(fixture.ImportSource{ID: bankSource, UserID: userA, Provider: model.ImportProviderSimpleFIN, Name: "Bank", CredentialCiphertext: "v1:iv:ct"})
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: bankSource, ExternalAccountID: "ACT-1", AccountID: acct1})
	h.provider.accounts = []model.ExternalAccount{{ID: "ACT-1", Name: "Checking", Currency: "USD", Balance: "1"}}
	raw, _ := json.Marshal(map[string]any{"id": "T1", "posted": 1755900000, "amount": "-4.20", "payee": "Cafe"})
	h.provider.txs = []model.ExternalTransaction{{ExternalAccountID: "ACT-1", ID: "T1", Amount: "-4.20", Posted: 1755900000, Payee: "Cafe", Raw: raw}}

	status, env := call(t, h, "POST", "/api/v1/import/sync-source", map[string]any{"sourceId": bankSource, "accessUrl": "https://u:p@bridge.example/simplefin", "startDate": "2025-08-01", "endDate": "2025-08-31"})
	if status != 200 {
		t.Fatalf("status %d: %v", status, env)
	}
	data := env["data"].(map[string]any)
	run := data["run"].(map[string]any)
	if run["status"] != "completed" || run["importedCount"] != float64(1) || run["trigger"] != "manual" {
		t.Fatalf("run = %v", run)
	}
	if h.txns.created != 1 {
		t.Fatalf("created %d", h.txns.created)
	}
	accounts := data["accounts"].([]any)
	if len(accounts) != 1 || accounts[0].(map[string]any)["state"] != "mapped" {
		t.Fatalf("accounts = %v", accounts)
	}

	status, env = call(t, h, "GET", "/api/v1/import/get-run-list?sourceId="+bankSource, nil)
	if status != 200 || len(env["data"].(map[string]any)["items"].([]any)) != 1 {
		t.Fatalf("run list %d: %v", status, env)
	}
	status, env = call(t, h, "GET", "/api/v1/import/get-run?id="+run["id"].(string), nil)
	if status != 200 || len(env["data"].(map[string]any)["links"].([]any)) != 1 {
		t.Fatalf("get-run %d: %v", status, env)
	}
	status, env = call(t, h, "GET", "/api/v1/import/get-run?id=nope", nil)
	if status != 400 {
		t.Fatalf("bad id: %d %v", status, env)
	}
}

func TestSyncSource_ValidationAndProviderErrors(t *testing.T) {
	h := newHarness(t)
	h.f.ImportSource(fixture.ImportSource{ID: bankSource, UserID: userA, Provider: model.ImportProviderSimpleFIN, Name: "Bank", CredentialCiphertext: "v1:iv:ct"})
	status, env := call(t, h, "POST", "/api/v1/import/sync-source", map[string]any{"sourceId": bankSource, "accessUrl": "", "startDate": "2025-08-01"})
	if status != 400 || env["errors"].(map[string]any)["accessUrl"] == nil {
		t.Fatalf("blank accessUrl: %d %v", status, env)
	}
	status, env = call(t, h, "POST", "/api/v1/import/sync-source", map[string]any{"sourceId": bankSource, "accessUrl": "https://u:p@b/x", "startDate": "08/01/2025"})
	if status != 400 || env["errors"].(map[string]any)["startDate"] == nil {
		t.Fatalf("bad startDate: %d %v", status, env)
	}
	h.provider.err = appimports.ErrProviderUnavailable
	status, env = call(t, h, "POST", "/api/v1/import/sync-source", map[string]any{"sourceId": bankSource, "accessUrl": "https://u:p@b/x", "startDate": "2025-08-01", "endDate": "2025-08-31"})
	// The 400 carries errs.CodeImportProviderUnavailable, so the envelope
	// message is the server-rendered locales/en.json catalogue text, not the
	// bare provider sentinel.
	if status != 400 || env["message"] != "The bank bridge could not be reached. Try again in a few minutes." {
		t.Fatalf("unavailable: %d %v", status, env)
	}
	// the run row is on record even though the fetch failed
	_, env = call(t, h, "GET", "/api/v1/import/get-run-list", nil)
	items := env["data"].(map[string]any)["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["status"] != "failed" {
		t.Fatalf("items = %v", items)
	}
}
