// Command apicompare diffs the JSON responses of the PHP and Go Econumo backends
// running against an identical (synthetic-fixture) database. It logs in to each
// backend, walks the read-only (GET) endpoints, and reports per-endpoint whether
// the FULL response payload matches — body bytes compared after a stable
// re-marshal so object key ORDER is ignored but every value (dates included) is
// compared verbatim.
//
// Prereqs (see deployment/compare/seed.sh):
//   - both backends serve byte-identical databases
//   - both share the same JWT keypair
//
// Usage:
//
//	go run ./cmd/apicompare -php http://localhost:8082 -go http://localhost:8282
//
// Exit code 0 = all compared endpoints match; 1 = at least one diff or error.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

// Login credentials for the seed database. Overridable via -email / -password
// so creds are never hardcoded for a non-fixture seed.
var (
	loginEmail    = "kuznetsov2d@gmail.com"
	loginPassword = "econum0))"
)

func main() {
	phpBase := flag.String("php", "http://localhost:8082", "base URL of the PHP backend")
	goBase := flag.String("go", "http://localhost:8282", "base URL of the Go backend")
	verbose := flag.Bool("v", false, "print the full diffing body on mismatch")
	email := flag.String("email", loginEmail, "login email for the seed DB")
	password := flag.String("password", loginPassword, "login password for the seed DB")
	flag.Parse()
	loginEmail, loginPassword = *email, *password

	php := &client{base: strings.TrimRight(*phpBase, "/"), http: &http.Client{Timeout: 30 * time.Second}}
	go_ := &client{base: strings.TrimRight(*goBase, "/"), http: &http.Client{Timeout: 30 * time.Second}}

	fmt.Printf("PHP backend: %s\n", php.base)
	fmt.Printf("Go  backend: %s\n\n", go_.base)

	// Authenticate against each backend independently (also exercises /login).
	if err := php.login(); err != nil {
		fmt.Fprintf(os.Stderr, "PHP login failed: %v\n", err)
		os.Exit(1)
	}
	if err := go_.login(); err != nil {
		fmt.Fprintf(os.Stderr, "Go login failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Authenticated against both backends ✓")
	fmt.Println()

	// The endpoint walk: static GETs first; parameterized GETs derive their
	// params from a list response fetched from the PHP backend (both backends
	// have identical data, so the same params apply to both).
	endpoints := buildEndpoints(php)

	var pass, fail int
	for _, ep := range endpoints {
		res := compare(php, go_, ep, *verbose)
		if res {
			pass++
		} else {
			fail++
		}
	}

	fmt.Println()
	fmt.Printf("===== %d passed, %d failed, %d total =====\n", pass, fail, pass+fail)
	if fail > 0 {
		os.Exit(1)
	}
}

// endpoint is one GET to compare.
type endpoint struct {
	name  string
	path  string
	query url.Values
}

func (e endpoint) full() string {
	if len(e.query) == 0 {
		return e.path
	}
	return e.path + "?" + e.query.Encode()
}

// buildEndpoints returns the GET endpoints to compare. Parameterized ones are
// expanded by first querying the PHP backend's list endpoints for real IDs.
func buildEndpoints(php *client) []endpoint {
	eps := []endpoint{
		{name: "user/get-user-data", path: "/api/v1/user/get-user-data"},
		{name: "user/get-option-list", path: "/api/v1/user/get-option-list"},
		{name: "account/get-account-list", path: "/api/v1/account/get-account-list"},
		{name: "account/get-folder-list", path: "/api/v1/account/get-folder-list"},
		{name: "category/get-category-list", path: "/api/v1/category/get-category-list"},
		{name: "tag/get-tag-list", path: "/api/v1/tag/get-tag-list"},
		{name: "payee/get-payee-list", path: "/api/v1/payee/get-payee-list"},
		{name: "currency/get-currency-list", path: "/api/v1/currency/get-currency-list"},
		{name: "currency/get-currency-rate-list", path: "/api/v1/currency/get-currency-rate-list"},
		{name: "connection/get-connection-list", path: "/api/v1/connection/get-connection-list"},
		{name: "budget/get-budget-list", path: "/api/v1/budget/get-budget-list"},
	}

	// get-budget — for each budget id from the list.
	if budgets, err := php.getData("/api/v1/budget/get-budget-list", nil); err == nil {
		date := time.Now().Format("2006-01-02")
		for _, id := range extractIDs(budgets) {
			q := url.Values{"id": {id}, "date": {date}}
			eps = append(eps, endpoint{name: "budget/get-budget (" + short(id) + ")", path: "/api/v1/budget/get-budget", query: q})
		}
	} else {
		fmt.Fprintf(os.Stderr, "warn: could not list budgets for get-budget: %v\n", err)
	}

	// transaction/get-transaction-list — for each account id, over a wide period.
	// periodEnd must not be in the future (PHP validates that), so end = today.
	if accounts, err := php.getData("/api/v1/account/get-account-list", nil); err == nil {
		today := time.Now().Format("2006-01-02")
		for _, id := range extractIDs(accounts) {
			q := url.Values{
				"accountId":   {id},
				"periodStart": {"2000-01-01"},
				"periodEnd":   {today},
			}
			eps = append(eps, endpoint{name: "transaction/get-transaction-list (" + short(id) + ")", path: "/api/v1/transaction/get-transaction-list", query: q})
		}
	} else {
		fmt.Fprintf(os.Stderr, "warn: could not list accounts for get-transaction-list: %v\n", err)
	}

	return eps
}

// compare fetches one endpoint from both backends and reports parity. It returns
// true on a full match.
func compare(php, go_ *client, ep endpoint, verbose bool) bool {
	pStatus, pBody, pErr := php.get(ep.full())
	gStatus, gBody, gErr := go_.get(ep.full())

	if pErr != nil || gErr != nil {
		fmt.Printf("[ERR ] %-48s php=%v go=%v\n", ep.name, pErr, gErr)
		return false
	}
	if pStatus != gStatus {
		fmt.Printf("[DIFF] %-48s status php=%d go=%d\n", ep.name, pStatus, gStatus)
		return false
	}

	pCanon, pe := canonical(pBody)
	gCanon, ge := canonical(gBody)
	if pe != nil || ge != nil {
		// One side returned non-JSON (e.g. CSV/text) — fall back to raw bytes.
		if bytes.Equal(pBody, gBody) {
			fmt.Printf("[PASS] %-48s (%d, raw match)\n", ep.name, pStatus)
			return true
		}
		fmt.Printf("[DIFF] %-48s non-JSON body differs (php %dB, go %dB)\n", ep.name, len(pBody), len(gBody))
		if verbose {
			printBodies(pBody, gBody)
		}
		return false
	}

	if bytes.Equal(pCanon, gCanon) {
		fmt.Printf("[PASS] %-48s (%d)\n", ep.name, pStatus)
		return true
	}

	// Classify list payloads: same element multiset but different order is an
	// ORDERING difference, not a content one. This is reported without printing
	// any field values (only the classification + first byte offset).
	if kind := classifyListDiff(pBody, gBody); kind != "" {
		fmt.Printf("[DIFF] %-48s payload differs (%d) — %s\n", ep.name, pStatus, kind)
	} else {
		fmt.Printf("[DIFF] %-48s payload differs (%d)\n", ep.name, pStatus)
	}
	if d := firstDiff(pCanon, gCanon); d != "" {
		fmt.Printf("        %s\n", d)
	}
	if verbose {
		printBodies(pCanon, gCanon)
	}
	return false
}

// ---- HTTP client ----

type client struct {
	base  string
	http  *http.Client
	token string
}

func (c *client) login() error {
	body, _ := json.Marshal(map[string]string{"username": loginEmail, "password": loginPassword})
	req, _ := http.NewRequest(http.MethodPost, c.base+"/api/v1/user/login-user", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	// The login endpoint returns the payload un-enveloped: {token, user}. Accept
	// either a top-level token or a data-wrapped one, to be tolerant of either
	// backend's shape.
	var env struct {
		Token string `json:"token"`
		Data  struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("decode login response: %w", err)
	}
	token := env.Token
	if token == "" {
		token = env.Data.Token
	}
	if token == "" {
		return fmt.Errorf("no token in login response: %s", truncate(string(raw), 200))
	}
	c.token = token
	return nil
}

// get returns (status, body, error) for a GET against this backend.
func (c *client) get(path string) (int, []byte, error) {
	req, _ := http.NewRequest(http.MethodGet, c.base+path, nil)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	return resp.StatusCode, raw, err
}

// getData fetches a GET and returns the decoded `data` field (for ID chaining).
func (c *client) getData(path string, q url.Values) (interface{}, error) {
	full := path
	if len(q) > 0 {
		full += "?" + q.Encode()
	}
	status, raw, err := c.get(full)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("status %d", status)
	}
	var env struct {
		Data interface{} `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	return env.Data, nil
}

// ---- JSON helpers ----

// canonical re-marshals JSON with sorted object keys, so semantically-equal
// payloads with different key order compare equal while every value is preserved.
func canonical(raw []byte) ([]byte, error) {
	var v interface{}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber() // preserve numeric formatting exactly
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return marshalSorted(v)
}

func marshalSorted(v interface{}) ([]byte, error) {
	var b bytes.Buffer
	if err := writeSorted(&b, v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func writeSorted(b *bytes.Buffer, v interface{}) error {
	switch t := v.(type) {
	case map[string]interface{}:
		b.WriteByte('{')
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			kb, _ := json.Marshal(k)
			b.Write(kb)
			b.WriteByte(':')
			if err := writeSorted(b, t[k]); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	case []interface{}:
		b.WriteByte('[')
		for i, e := range t {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := writeSorted(b, e); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	default:
		enc, err := json.Marshal(v)
		if err != nil {
			return err
		}
		b.Write(enc)
	}
	return nil
}

// firstDiff returns a short human description of the first byte divergence.
func firstDiff(a, b []byte) string {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			start := i - 40
			if start < 0 {
				start = 0
			}
			return fmt.Sprintf("first diff at byte %d:\n          php: ...%s\n          go : ...%s",
				i, snippet(a, start, i+40), snippet(b, start, i+40))
		}
	}
	if len(a) != len(b) {
		return fmt.Sprintf("bodies share a prefix but lengths differ (php %dB, go %dB)", len(a), len(b))
	}
	return ""
}

func snippet(b []byte, start, end int) string {
	if start < 0 {
		start = 0
	}
	if end > len(b) {
		end = len(b)
	}
	return string(b[start:end])
}

func printBodies(php, go_ []byte) {
	fmt.Printf("        --- PHP ---\n%s\n        --- GO ---\n%s\n", string(php), string(go_))
}

// classifyListDiff inspects two response bodies whose canonical forms differ and
// reports "ordering only (N items)" when they contain the same multiset of list
// elements in a different order, "content differs (...)" when the element sets
// differ, or "" when the shape isn't a recognizable list. It compares only
// canonicalized element strings (never prints them), so no field values leak.
func classifyListDiff(phpBody, goBody []byte) string {
	pl := listElements(phpBody)
	gl := listElements(goBody)
	if pl == nil || gl == nil {
		return ""
	}
	if len(pl) != len(gl) {
		return fmt.Sprintf("content differs (php %d items, go %d items)", len(pl), len(gl))
	}
	pm := multiset(pl)
	gm := multiset(gl)
	missing := 0
	for k, n := range pm {
		if gm[k] != n {
			missing++
		}
	}
	if missing == 0 {
		return fmt.Sprintf("ordering only (%d items)", len(pl))
	}
	return fmt.Sprintf("content differs (%d of %d elements not matched)", missing, len(pl))
}

// listElements returns the canonicalized elements of the response's primary list
// (data as an array, or data.items), or nil if there isn't one.
func listElements(body []byte) []string {
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil || len(env.Data) == 0 {
		return nil
	}
	// data is either an array, or an object with an "items" array.
	var arr []json.RawMessage
	if json.Unmarshal(env.Data, &arr) != nil {
		var obj struct {
			Items []json.RawMessage `json:"items"`
		}
		if json.Unmarshal(env.Data, &obj) != nil || obj.Items == nil {
			return nil
		}
		arr = obj.Items
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		c, err := canonical(e)
		if err != nil {
			return nil
		}
		out = append(out, string(c))
	}
	return out
}

func multiset(xs []string) map[string]int {
	m := make(map[string]int, len(xs))
	for _, x := range xs {
		m[x]++
	}
	return m
}

// extractIDs pulls top-level "id" string fields from a data value that is either
// a list of objects or an object containing a list.
func extractIDs(data interface{}) []string {
	var ids []string
	switch t := data.(type) {
	case []interface{}:
		for _, e := range t {
			if m, ok := e.(map[string]interface{}); ok {
				if id, ok := m["id"].(string); ok && id != "" {
					ids = append(ids, id)
				}
			}
		}
	case map[string]interface{}:
		for _, v := range t {
			ids = append(ids, extractIDs(v)...)
		}
	}
	return ids
}

func short(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}
