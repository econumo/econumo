package api_test

import (
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestClaimSetupToken_Handler(t *testing.T) {
	h := newHarness(t)
	status, env := call(t, h, "POST", "/api/v1/import/claim-setup-token", map[string]any{"setupToken": "abc"})
	if status != 200 || env["data"].(map[string]any)["accessUrl"] != "https://u:p@bridge.example/simplefin" {
		t.Fatalf("%d %v", status, env)
	}
	status, env = call(t, h, "POST", "/api/v1/import/claim-setup-token", map[string]any{"setupToken": "used"})
	// The 403 carries errs.CodeImportSetupTokenRejected, so the envelope
	// message is the server-rendered locales/en.json catalogue text, not the
	// bare provider sentinel.
	if status != 403 || env["message"] != "SimpleFIN rejected this setup token. It may have been claimed already — generate a new one in your SimpleFIN bridge." {
		t.Fatalf("%d %v", status, env)
	}
	status, env = call(t, h, "POST", "/api/v1/import/claim-setup-token", map[string]any{"setupToken": ""})
	if status != 400 || env["errors"].(map[string]any)["setupToken"] == nil {
		t.Fatalf("%d %v", status, env)
	}
}

func TestCredentialKey_Handlers(t *testing.T) {
	h := newHarness(t)
	status, _ := call(t, h, "GET", "/api/v1/import/get-credential-key", nil)
	if status != 400 {
		t.Fatalf("missing key: %d", status)
	}
	status, env := call(t, h, "POST", "/api/v1/import/set-credential-key", map[string]any{"wrappedDataKey": "v1:iv:ct", "kdf": `{"alg":"PBKDF2-SHA256","salt":"c2FsdA==","iterations":600000}`})
	if status != 200 || env["data"].(map[string]any)["wrappedDataKey"] != "v1:iv:ct" {
		t.Fatalf("%d %v", status, env)
	}
	status, env = call(t, h, "POST", "/api/v1/import/set-credential-key", map[string]any{"wrappedDataKey": "", "kdf": "{}"})
	if status != 400 || env["errors"].(map[string]any)["wrappedDataKey"] == nil {
		t.Fatalf("%d %v", status, env)
	}
	status, env = call(t, h, "GET", "/api/v1/import/get-credential-key", nil)
	if status != 200 || env["data"].(map[string]any)["wrappedDataKey"] != "v1:iv:ct" {
		t.Fatalf("%d %v", status, env)
	}
}

func TestListExternalAccounts_Handler(t *testing.T) {
	h := newHarness(t)
	h.f.ImportSource(fixture.ImportSource{ID: bankSource, UserID: userA, Provider: model.ImportProviderSimpleFIN, Name: "Bank", CredentialCiphertext: "v1:iv:ct"})
	h.provider.accounts = []model.ExternalAccount{{ID: "ACT-1", Name: "Checking", Currency: "USD", Balance: "1", OrgName: "Big Bank"}}
	status, env := call(t, h, "POST", "/api/v1/import/list-external-accounts", map[string]any{"sourceId": bankSource, "accessUrl": "https://u:p@b/x"})
	if status != 200 {
		t.Fatalf("%d %v", status, env)
	}
	items := env["data"].(map[string]any)["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["orgName"] != "Big Bank" || items[0].(map[string]any)["state"] != "unmapped" {
		t.Fatalf("items = %v", items)
	}
	status, _ = call(t, h, "POST", "/api/v1/import/list-external-accounts", map[string]any{"sourceId": source, "accessUrl": "https://u:p@b/x"})
	if status != 400 {
		t.Fatalf("push provider: %d", status)
	}
}

func TestCreateSource_SimpleFINHandler(t *testing.T) {
	h := newHarness(t)
	status, env := call(t, h, "POST", "/api/v1/import/create-source", map[string]any{"provider": "simplefin", "name": "Bank", "credentialCiphertext": "v1:a:b"})
	if status != 200 || env["data"].(map[string]any)["item"].(map[string]any)["credentialCiphertext"] != "v1:a:b" {
		t.Fatalf("%d %v", status, env)
	}
	status, env = call(t, h, "POST", "/api/v1/import/create-source", map[string]any{"provider": "simplefin", "name": "Bank"})
	if status != 400 || env["errors"].(map[string]any)["credentialCiphertext"] == nil {
		t.Fatalf("simplefin without ciphertext must fail: %d %v", status, env)
	}
}
