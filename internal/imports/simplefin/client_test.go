package simplefin_test

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/imports"
	"github.com/econumo/econumo/internal/imports/simplefin"
)

const accountsDoc = `{"errors":["Connection to Big Bank may need attention"],"accounts":[{"org":{"domain":"bigbank.example","name":"Big Bank"},"id":"ACT-1","name":"Checking","currency":"USD","balance":"1000.10","available-balance":"990.00","balance-date":1756000000,"transactions":[
{"id":"TRN-1","posted":1755900000,"amount":"-12.50","description":"COFFEE SHOP","payee":"Blue Bottle","memo":"","transacted_at":1755899000},
{"id":"TRN-2","posted":1755910000,"amount":"2500.00","description":"PAYROLL","payee":"","memo":""},
{"id":"TRN-3","posted":0,"amount":"-4.00","description":"PENDING THING","pending":true}
]}]}`

func newBridge(t *testing.T) (*httptest.Server, *[]*http.Request) {
	t.Helper()
	var seen []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Clone(context.Background()))
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/simplefin/claim/abc":
			w.Write([]byte("http://user:s3cret@" + r.Host + "/simplefin"))
		case r.Method == http.MethodPost && r.URL.Path == "/simplefin/claim/used":
			w.WriteHeader(http.StatusForbidden)
		case r.Method == http.MethodGet && r.URL.Path == "/unauthorized/accounts":
			w.WriteHeader(http.StatusUnauthorized)
		case r.Method == http.MethodGet && r.URL.Path == "/teapot/accounts":
			w.WriteHeader(http.StatusTeapot)
		case r.Method == http.MethodGet && r.URL.Path == "/simplefin/accounts":
			u, p, ok := r.BasicAuth()
			if !ok || u != "user" || p != "s3cret" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(accountsDoc))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func setupToken(srv *httptest.Server, path string) string {
	return base64.StdEncoding.EncodeToString([]byte(srv.URL + path))
}

func TestClaim_ReturnsAccessURL(t *testing.T) {
	srv, seen := newBridge(t)
	c := simplefin.New(simplefin.Options{AllowHTTP: true, AllowPrivateHosts: true})
	got, err := c.ClaimSetupToken(context.Background(), setupToken(srv, "/simplefin/claim/abc"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "http://user:s3cret@") || !strings.HasSuffix(got, "/simplefin") {
		t.Fatalf("access url = %q", got)
	}
	if (*seen)[0].ContentLength != 0 {
		t.Fatal("claim must POST an empty body")
	}
}

func TestClaim_ToleratesUnpaddedToken(t *testing.T) {
	srv, _ := newBridge(t)
	c := simplefin.New(simplefin.Options{AllowHTTP: true, AllowPrivateHosts: true})
	tok := strings.TrimRight(setupToken(srv, "/simplefin/claim/abc"), "=")
	if _, err := c.ClaimSetupToken(context.Background(), tok); err != nil {
		t.Fatal(err)
	}
}

func TestClaim_Rejected(t *testing.T) {
	srv, _ := newBridge(t)
	c := simplefin.New(simplefin.Options{AllowHTTP: true, AllowPrivateHosts: true})
	_, err := c.ClaimSetupToken(context.Background(), setupToken(srv, "/simplefin/claim/used"))
	if !errors.Is(err, imports.ErrSetupTokenRejected) {
		t.Fatalf("err = %v, want ErrSetupTokenRejected", err)
	}
}

func TestClaim_BadTokenIsRejected(t *testing.T) {
	c := simplefin.New(simplefin.Options{AllowHTTP: true, AllowPrivateHosts: true})
	for _, tok := range []string{"not base64!!", base64.StdEncoding.EncodeToString([]byte("ftp://x/y")), base64.StdEncoding.EncodeToString([]byte("http://insecure.example/claim"))} {
		if tok == base64.StdEncoding.EncodeToString([]byte("http://insecure.example/claim")) {
			c = simplefin.New(simplefin.Options{}) // https enforced by default
		}
		if _, err := c.ClaimSetupToken(context.Background(), tok); !errors.Is(err, imports.ErrSetupTokenRejected) {
			t.Fatalf("%q: err = %v", tok, err)
		}
	}
}

func TestFetch_ParsesAccountsAndDropsPending(t *testing.T) {
	srv, seen := newBridge(t)
	c := simplefin.New(simplefin.Options{AllowHTTP: true, AllowPrivateHosts: true})
	cred := imports.Credential{AccessURL: "http://user:s3cret@" + strings.TrimPrefix(srv.URL, "http://") + "/simplefin"}
	res, err := c.FetchTransactions(context.Background(), cred, imports.FetchRequest{
		StartDate: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), EndDate: time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	q := (*seen)[0].URL.Query()
	if q.Get("start-date") != "1785542400" || q.Get("end-date") != "1788134400" {
		t.Fatalf("date params = %v", q)
	}
	if (*seen)[0].URL.User != nil {
		t.Fatal("credentials must travel in the Authorization header, not the URL")
	}
	if len(res.Accounts) != 1 || res.Accounts[0].ID != "ACT-1" || res.Accounts[0].Name != "Checking" || res.Accounts[0].Currency != "USD" || res.Accounts[0].Balance != "1000.10" || res.Accounts[0].OrgName != "Big Bank" {
		t.Fatalf("accounts = %+v", res.Accounts)
	}
	if len(res.Transactions) != 2 {
		t.Fatalf("pending/unposted rows must be dropped: %+v", res.Transactions)
	}
	tx := res.Transactions[0]
	if tx.ExternalAccountID != "ACT-1" || tx.ID != "TRN-1" || tx.Amount != "-12.50" || tx.Posted != 1755900000 || tx.Payee != "Blue Bottle" || tx.Description != "COFFEE SHOP" {
		t.Fatalf("tx = %+v", tx)
	}
	if !strings.Contains(string(tx.Raw), `"id":"TRN-1"`) {
		t.Fatalf("raw must be the provider's JSON: %s", tx.Raw)
	}
	if len(res.Warnings) != 1 || res.Warnings[0] != "Connection to Big Bank may need attention" {
		t.Fatalf("warnings = %v", res.Warnings)
	}
}

func TestListAccounts_UsesBalancesOnly(t *testing.T) {
	srv, seen := newBridge(t)
	c := simplefin.New(simplefin.Options{AllowHTTP: true, AllowPrivateHosts: true})
	cred := imports.Credential{AccessURL: "http://user:s3cret@" + strings.TrimPrefix(srv.URL, "http://") + "/simplefin"}
	accts, err := c.ListAccounts(context.Background(), cred)
	if err != nil || len(accts) != 1 {
		t.Fatalf("accounts = %+v err %v", accts, err)
	}
	if (*seen)[0].URL.Query().Get("balances-only") != "1" {
		t.Fatalf("query = %v", (*seen)[0].URL.RawQuery)
	}
}

func TestFetch_BadCredentialAndUnavailable(t *testing.T) {
	srv, _ := newBridge(t)
	c := simplefin.New(simplefin.Options{AllowHTTP: true, AllowPrivateHosts: true})
	host := strings.TrimPrefix(srv.URL, "http://")
	_, err := c.FetchTransactions(context.Background(), imports.Credential{AccessURL: "http://" + host + "/simplefin"}, imports.FetchRequest{})
	if !errors.Is(err, imports.ErrCredentialInvalid) {
		t.Fatalf("no userinfo: %v", err)
	}
	// A revoked access URL (403) or wrong Basic credentials (401) must read as
	// "reconnect", not as a bridge outage.
	_, err = c.FetchTransactions(context.Background(), imports.Credential{AccessURL: "http://user:wrong@" + host + "/simplefin"}, imports.FetchRequest{})
	if !errors.Is(err, imports.ErrCredentialInvalid) {
		t.Fatalf("403 from accounts: %v", err)
	}
	_, err = c.FetchTransactions(context.Background(), imports.Credential{AccessURL: "http://user:s3cret@" + host + "/unauthorized"}, imports.FetchRequest{})
	if !errors.Is(err, imports.ErrCredentialInvalid) {
		t.Fatalf("401 from accounts: %v", err)
	}
	_, err = c.FetchTransactions(context.Background(), imports.Credential{AccessURL: "http://user:s3cret@" + host + "/teapot"}, imports.FetchRequest{})
	if !errors.Is(err, imports.ErrProviderUnavailable) {
		t.Fatalf("other non-2xx: %v", err)
	}
	srv.Close()
	_, err = c.FetchTransactions(context.Background(), imports.Credential{AccessURL: "http://user:s3cret@" + host + "/simplefin"}, imports.FetchRequest{})
	if !errors.Is(err, imports.ErrProviderUnavailable) {
		t.Fatalf("connection refused: %v", err)
	}
	if strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("error text leaks the credential: %v", err)
	}
}

func TestFetch_ForwardsUndecodableRows(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"errors":[],"accounts":[{"id":"ACT-1","name":"Checking","currency":"USD","transactions":[
{"id":"TRN-A","posted":"yesterday","amount":"-1.00"},
{"id":"","posted":1755900000,"amount":"-2.00"},
{"id":"TRN-C","posted":0,"amount":"-3.00"}
]}]}`))
	}))
	t.Cleanup(srv.Close)
	c := simplefin.New(simplefin.Options{AllowHTTP: true, AllowPrivateHosts: true})
	cred := imports.Credential{AccessURL: "http://user:s3cret@" + strings.TrimPrefix(srv.URL, "http://") + "/simplefin"}
	res, err := c.FetchTransactions(context.Background(), cred, imports.FetchRequest{})
	if err != nil {
		t.Fatal(err)
	}
	// Rows the parser will reject still have to reach it, so the run records
	// them as failed instead of losing them without trace.
	if len(res.Transactions) != 3 {
		t.Fatalf("transactions = %+v", res.Transactions)
	}
	for _, tx := range res.Transactions {
		if tx.ExternalAccountID != "ACT-1" || len(tx.Raw) == 0 {
			t.Fatalf("tx = %+v", tx)
		}
		if _, perr := imports.ParseSimpleFINEvent(imports.EncodeSimpleFINEvent(tx), time.UTC); perr == nil {
			t.Fatalf("%s must fail to parse", tx.Raw)
		}
	}
}

func TestClient_RefusesPrivateAddressesByDefault(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Write([]byte("http://user:s3cret@" + r.Host + "/simplefin"))
	}))
	t.Cleanup(srv.Close)
	c := simplefin.New(simplefin.Options{AllowHTTP: true})
	host := strings.TrimPrefix(srv.URL, "http://")
	if _, err := c.ClaimSetupToken(context.Background(), setupToken(srv, "/simplefin/claim/abc")); !errors.Is(err, imports.ErrProviderUnavailable) {
		t.Fatalf("claim: err = %v, want ErrProviderUnavailable", err)
	}
	_, err := c.FetchTransactions(context.Background(), imports.Credential{AccessURL: "http://user:s3cret@" + host + "/simplefin"}, imports.FetchRequest{})
	if !errors.Is(err, imports.ErrProviderUnavailable) {
		t.Fatalf("fetch: err = %v, want ErrProviderUnavailable", err)
	}
	if hits != 0 {
		t.Fatalf("the guard must refuse before the request reaches the host (%d hits)", hits)
	}
}
