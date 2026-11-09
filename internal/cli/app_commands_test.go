package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/infra/storage/sqlite"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/vo"
	appuser "github.com/econumo/econumo/internal/user"
)

func captureOutput(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	prevOut, prevErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW
	var outBuf, errBuf bytes.Buffer
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(&outBuf, outR); done <- struct{}{} }()
	go func() { _, _ = io.Copy(&errBuf, errR); done <- struct{}{} }()
	defer func() {
		os.Stdout, os.Stderr = prevOut, prevErr
	}()
	fn()
	_ = outW.Close()
	_ = errW.Close()
	<-done
	<-done
	return outBuf.String(), errBuf.String()
}

func runApp(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	stdout, stderr = captureOutput(t, func() { code = Run(args) })
	return code, stdout, stderr
}

var (
	clientIDLine     = regexp.MustCompile(`(?m)^client_id: (\S+)$`)
	clientSecretLine = regexp.MustCompile(`(?m)^client_secret: (\S+)$`)
)

func clientID(t *testing.T, stdout string) string {
	t.Helper()
	m := clientIDLine.FindStringSubmatch(stdout)
	if m == nil {
		t.Fatalf("no client_id line in %q", stdout)
	}
	return m[1]
}

// appDB opens the CLI test database for seeding and inspection, with a busy
// timeout so it never races the container's own connection into SQLITE_BUSY.
func appDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	t.Setenv("SQLITE_BUSY_TIMEOUT", "5000")
	t.Setenv("ECONUMO_URL", "")
	be := sqlite.New()
	be.SetBusyTimeout(5000)
	db, err := be.Open(context.Background(), "sqlite://"+path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func countClients(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM oauth_clients`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func createCLIUser(t *testing.T, db *sql.DB, email string) string {
	t.Helper()
	if code, _, stderr := runApp(t, "user:create", "User", email, "secret-pw-1"); code != 0 {
		t.Fatalf("user:create %s = %d: %s", email, code, stderr)
	}
	var id string
	if err := db.QueryRow(`SELECT id FROM users WHERE email = ?`, email).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

const (
	seedUserAppID = "019a0000-0000-7000-8000-0000000000aa"
	seedGrantA    = "019a0000-0000-7000-8000-0000000000a1"
	seedGrantB    = "019a0000-0000-7000-8000-0000000000b1"
)

// seedUserApp inserts a user app owned by owner with one live grant per user.
func seedUserApp(t *testing.T, db *sql.DB, owner string, secretHash any, users ...string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO oauth_clients (id, name, redirect_uris, secret_hash, created_at, owner_user_id, is_global)
		VALUES (?, 'Codex', '["http://127.0.0.1/cb","https://codex.example.test/cb"]', ?, '2026-11-01 10:00:00', ?, 0)`,
		seedUserAppID, secretHash, owner); err != nil {
		t.Fatal(err)
	}
	grantIDs := []string{seedGrantA, seedGrantB}
	for i, u := range users {
		if _, err := db.Exec(`INSERT INTO oauth_grants (id, user_id, client_id, refresh_token_hash, created_at, last_used_at, expires_at)
			VALUES (?, ?, ?, ?, '2026-11-02 10:00:00', ?, '2099-01-01 00:00:00')`,
			grantIDs[i], u, seedUserAppID, "refresh-"+grantIDs[i], "2026-11-0"+string(rune('3'+i))+" 10:00:00"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestParseAppArgs(t *testing.T) {
	pos, flags, err := parseAppArgs([]string{"Claude", "--public", "https://a/cb"}, []string{"public"}, []string{"redirect-uri"})
	if err != nil || !slices.Equal(pos, []string{"Claude", "https://a/cb"}) || len(flags["public"]) != 1 {
		t.Fatalf("bool flag between positionals: %v %v %v", pos, flags, err)
	}
	pos, flags, err = parseAppArgs([]string{"--redirect-uri=https://a", "--redirect-uri", "https://b", "id"}, nil, []string{"redirect-uri", "name"})
	if err != nil || !slices.Equal(pos, []string{"id"}) || !slices.Equal(flags["redirect-uri"], []string{"https://a", "https://b"}) {
		t.Fatalf("repeated value flag: %v %v %v", pos, flags, err)
	}
	if _, _, err := parseAppArgs([]string{"id", "--name"}, nil, []string{"name"}); err == nil {
		t.Fatal("a value flag without a value must be an error")
	}
	if _, _, err := parseAppArgs([]string{"--bogus"}, []string{"public"}, []string{"name"}); err == nil {
		t.Fatal("an unknown flag must be an error")
	}
	if _, _, err := parseAppArgs([]string{"id", "--name", "--public"}, []string{"public"}, []string{"name"}); err == nil {
		t.Fatal("a value flag must not take the next option as its value")
	}
	if _, _, err := parseAppArgs([]string{"--public=yes"}, []string{"public"}, nil); err == nil {
		t.Fatal("a bool flag with a value must be an error")
	}
}

func TestAppRegister(t *testing.T) {
	path := cliEnvDB(t)
	db := appDB(t, path)

	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	code, stdout, stderr := runApp(t, "app:register", "Claude", "https://claude.ai/api/mcp/auth_callback")
	if code != 0 {
		t.Fatalf("app:register = %d: %s", code, stderr)
	}
	id := clientID(t, stdout)
	m := clientSecretLine.FindStringSubmatch(stdout)
	if m == nil || len(m[1]) != 43 {
		t.Fatalf("no 43-char secret in %q", stdout)
	}
	secret := m[1]
	if !strings.Contains(stdout, `App "Claude" registered (official).`) || !strings.Contains(stdout, "The secret is shown only once.") {
		t.Fatalf("stdout: %q", stdout)
	}
	if !strings.Contains(stderr, "warning: the OAuth server is disabled (ECONUMO_URL is not an https origin (or an http loopback origin)); the app will work once it is enabled") {
		t.Fatalf("stderr lacks the disabled warning: %q", stderr)
	}
	var global bool
	var owner, hash sql.NullString
	if err := db.QueryRow(`SELECT is_global, owner_user_id, secret_hash FROM oauth_clients WHERE id = ?`, id).Scan(&global, &owner, &hash); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(secret))
	if !global || owner.Valid || hash.String != hex.EncodeToString(sum[:]) {
		t.Fatalf("row: global=%v owner=%v hash=%v", global, owner, hash)
	}
	if !strings.Contains(logs.String(), "app-register") || !strings.Contains(logs.String(), id) {
		t.Fatalf("no audit line: %s", logs.String())
	}
	if strings.Contains(logs.String(), secret) {
		t.Fatalf("the secret reached the log: %s", logs.String())
	}

	code, stdout, _ = runApp(t, "app:register", "--public", "Share", "https://share.example.test/cb")
	if code != 0 || !strings.Contains(stdout, "client_secret: none (public client, PKCE only)") {
		t.Fatalf("public: %d %q", code, stdout)
	}
	if err := db.QueryRow(`SELECT secret_hash FROM oauth_clients WHERE id = ?`, clientID(t, stdout)).Scan(&hash); err != nil || hash.Valid {
		t.Fatalf("public secret_hash: %v %v", hash, err)
	}

	before := countClients(t, db)
	for _, args := range [][]string{
		{"app:register", "NoURI"},
		{"app:register", "Plain", "http://example.com/cb"},
		{"app:register", strings.Repeat("a", 101), "https://a.example.test/cb"},
	} {
		if code, _, _ := runApp(t, args...); code != 1 {
			t.Errorf("%v = %d, want 1", args, code)
		}
	}
	if got := countClients(t, db); got != before {
		t.Fatalf("rejected registrations wrote rows: %d -> %d", before, got)
	}
}

func TestAppRegisterUnknownFlagWritesNothing(t *testing.T) {
	db := appDB(t, cliEnvDB(t))
	if code, _, stderr := runApp(t, "app:register", "X", "https://a.example.test/cb", "--bogus"); code != 1 || !strings.Contains(stderr, "--bogus") {
		t.Fatalf("unknown flag = %d %q", code, stderr)
	}
	if n := countClients(t, db); n != 0 {
		t.Fatalf("rows written: %d", n)
	}
}

func TestAppListAndShow(t *testing.T) {
	db := appDB(t, cliEnvDB(t))
	a := createCLIUser(t, db, "a@example.test")
	b := createCLIUser(t, db, "b@example.test")
	code, stdout, _ := runApp(t, "app:register", "Share", "https://share.example.test/cb")
	if code != 0 {
		t.Fatal("register")
	}
	official := clientID(t, stdout)
	seedUserApp(t, db, a, nil, a, b)
	if _, err := db.Exec(`UPDATE oauth_grants SET scope = 'read' WHERE id = ?`, seedGrantB); err != nil {
		t.Fatal(err)
	}

	code, stdout, _ = runApp(t, "app:list")
	if code != 0 {
		t.Fatalf("app:list = %d", code)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "ID") || !strings.Contains(lines[0], "LAST USED") || !strings.Contains(lines[0], "CONNECTIONS") {
		t.Fatalf("app:list: %q", stdout)
	}
	var offLine, userLine string
	for _, l := range lines[1:] {
		if strings.HasPrefix(l, official) {
			offLine = l
		}
		if strings.HasPrefix(l, seedUserAppID) {
			userLine = l
		}
	}
	offFields := strings.Fields(offLine)
	if len(offFields) < 3 || offFields[1] != "official" || !strings.Contains(offLine, "share.example.test") ||
		!strings.Contains(offLine, "never") || offFields[len(offFields)-1] != "0" {
		t.Fatalf("official row: %q", offLine)
	}
	if !strings.Contains(userLine, " user ") || !strings.Contains(userLine, a) || !strings.Contains(userLine, "127.0.0.1") ||
		!strings.HasSuffix(userLine, "2 (1 from other users)") || !strings.Contains(userLine, "2026-11-04 10:00:00") {
		t.Fatalf("user row: %q", userLine)
	}

	_, stdout, _ = runApp(t, "app:list", "--official")
	if !strings.Contains(stdout, official) || strings.Contains(stdout, seedUserAppID) {
		t.Fatalf("--official: %q", stdout)
	}
	_, stdout, _ = runApp(t, "app:list", "--user")
	if strings.Contains(stdout, official) || !strings.Contains(stdout, seedUserAppID) {
		t.Fatalf("--user: %q", stdout)
	}
	_, stdout, _ = runApp(t, "app:list", "--user", "--official")
	if !strings.Contains(stdout, official) || !strings.Contains(stdout, seedUserAppID) {
		t.Fatalf("both flags: %q", stdout)
	}

	code, stdout, _ = runApp(t, "app:show", seedUserAppID)
	if code != 0 {
		t.Fatalf("app:show = %d", code)
	}
	for _, want := range []string{"http://127.0.0.1/cb", "https://codex.example.test/cb", "public", "Connections: 2 (1 from other users)", a, b, "USER", "CONNECTED"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("app:show lacks %q:\n%s", want, stdout)
		}
	}
	scopes := map[string]string{}
	for _, l := range strings.Split(stdout, "\n") {
		if f := strings.Fields(l); len(f) >= 2 && (f[0] == "USER" || f[0] == a || f[0] == b) {
			scopes[f[0]] = f[1]
		}
	}
	if scopes["USER"] != "SCOPE" || scopes[a] != "mcp" || scopes[b] != "read" {
		t.Fatalf("app:show connection scopes = %v:\n%s", scopes, stdout)
	}
	code, stdout, _ = runApp(t, "app:show", " "+official+" ")
	if code != 0 || !strings.Contains(stdout, "confidential") || !strings.Contains(stdout, "official") || !strings.Contains(stdout, "never") {
		t.Fatalf("app:show official: %d %q", code, stdout)
	}
	if code, _, _ := runApp(t, "app:show", "019a0000-0000-7000-8000-00000000ffff"); code != 1 {
		t.Fatalf("unknown id = %d", code)
	}
	if code, _, stderr := runApp(t, "app:show", "nope"); code != 1 || !strings.Contains(stderr, `invalid app id "nope"`) {
		t.Fatalf("bad id = %d %q", code, stderr)
	}
}

func TestAppUpdateRefusals(t *testing.T) {
	db := appDB(t, cliEnvDB(t))
	a := createCLIUser(t, db, "a@example.test")
	seedUserApp(t, db, a, nil, a)
	_, stdout, _ := runApp(t, "app:register", "Share", "https://share.example.test/cb")
	official := clientID(t, stdout)

	row := func(id string) string {
		var name, uris string
		if err := db.QueryRow(`SELECT name, redirect_uris FROM oauth_clients WHERE id = ?`, id).Scan(&name, &uris); err != nil {
			t.Fatal(err)
		}
		return name + "|" + uris
	}
	offBefore, userBefore := row(official), row(seedUserAppID)
	if code, _, _ := runApp(t, "app:update", official); code != 1 {
		t.Fatalf("no options = %d", code)
	}
	if code, _, _ := runApp(t, "app:update", seedUserAppID, "--name", "Renamed"); code != 1 {
		t.Fatalf("user app = %d", code)
	}
	if row(official) != offBefore || row(seedUserAppID) != userBefore {
		t.Fatal("a refused update changed a row")
	}

	code, stdout, stderr := runApp(t, "app:update", "--redirect-uri=https://share.example.test/a", official, "--name", "Econumo Share", "--redirect-uri", "https://share.example.test/b")
	if code != 0 || !strings.Contains(stdout, "App "+official+" updated.") {
		t.Fatalf("update = %d %q %q", code, stdout, stderr)
	}
	if got := row(official); got != `Econumo Share|["https://share.example.test/a","https://share.example.test/b"]` {
		t.Fatalf("stored: %s", got)
	}
}

func TestAppMakeOfficialAndRotate(t *testing.T) {
	db := appDB(t, cliEnvDB(t))
	a := createCLIUser(t, db, "a@example.test")
	seedUserApp(t, db, a, "old-hash", a)

	if code, stdout, _ := runApp(t, "app:make-official", seedUserAppID); code != 0 || !strings.Contains(stdout, "App "+seedUserAppID+" is now official.") {
		t.Fatalf("make-official = %d %q", code, stdout)
	}
	var global bool
	var owner sql.NullString
	if err := db.QueryRow(`SELECT is_global, owner_user_id FROM oauth_clients WHERE id = ?`, seedUserAppID).Scan(&global, &owner); err != nil || !global || owner.Valid {
		t.Fatalf("after make-official: %v %v %v", global, owner, err)
	}
	if code, _, _ := runApp(t, "app:make-official", seedUserAppID); code != 1 {
		t.Fatalf("second make-official = %d", code)
	}

	code, stdout, _ := runApp(t, "app:rotate-secret", seedUserAppID)
	m := clientSecretLine.FindStringSubmatch(stdout)
	if code != 0 || m == nil || !strings.Contains(stdout, "The old secret no longer works.") {
		t.Fatalf("rotate = %d %q", code, stdout)
	}
	var hash string
	if err := db.QueryRow(`SELECT secret_hash FROM oauth_clients WHERE id = ?`, seedUserAppID).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(m[1]))
	if hash == "old-hash" || hash != hex.EncodeToString(sum[:]) {
		t.Fatalf("stored hash %q", hash)
	}

	_, stdout, _ = runApp(t, "app:register", "--public", "Share", "https://share.example.test/cb")
	if code, _, _ := runApp(t, "app:rotate-secret", clientID(t, stdout)); code != 1 {
		t.Fatalf("rotate on a public app = %d", code)
	}
}

func TestAppDisconnect(t *testing.T) {
	db := appDB(t, cliEnvDB(t))
	a := createCLIUser(t, db, "a@example.test")
	b := createCLIUser(t, db, "b@example.test")
	c := createCLIUser(t, db, "c@example.test")
	seedUserApp(t, db, a, nil, a, b)

	revoked := func(grant string) bool {
		var at sql.NullString
		if err := db.QueryRow(`SELECT revoked_at FROM oauth_grants WHERE id = ?`, grant).Scan(&at); err != nil {
			t.Fatal(err)
		}
		return at.Valid
	}
	if code, stdout, _ := runApp(t, "app:disconnect", seedUserAppID, "--user", a); code != 0 || !strings.Contains(stdout, "1 connection(s) revoked.") {
		t.Fatalf("--user a = %d %q", code, stdout)
	}
	if !revoked(seedGrantA) || revoked(seedGrantB) {
		t.Fatal("--user a must revoke only a's grant")
	}
	if code, stdout, _ := runApp(t, "app:disconnect", "--user="+c, seedUserAppID); code != 0 || !strings.Contains(stdout, "0 connection(s) revoked.") {
		t.Fatalf("--user c = %d %q", code, stdout)
	}
	if revoked(seedGrantB) {
		t.Fatal("--user c touched b's grant")
	}
	if code, stdout, _ := runApp(t, "app:disconnect", seedUserAppID); code != 0 || !strings.Contains(stdout, "1 connection(s) revoked.") {
		t.Fatalf("all = %d %q", code, stdout)
	}
	if !revoked(seedGrantB) {
		t.Fatal("b's grant not revoked")
	}
	if code, _, _ := runApp(t, "app:disconnect", "019a0000-0000-7000-8000-00000000ffff"); code != 1 {
		t.Fatalf("unknown app = %d", code)
	}
	if code, _, stderr := runApp(t, "app:disconnect", seedUserAppID, "--user", "x"); code != 1 || !strings.Contains(stderr, `invalid user id "x"`) {
		t.Fatalf("bad user id = %d %q", code, stderr)
	}
}

// TestAppDeleteKillsLiveTokensOnARunningServer deletes an app from the CLI
// while a server built over the same database file holds a live access token
// for it: the server reads tokens from the database per request, so the next
// /mcp call fails without any signal between the processes.
func TestAppDeleteKillsLiveTokensOnARunningServer(t *testing.T) {
	path := cliEnvDB(t)
	db := appDB(t, path)
	h := server.BuildAPI(config.Config{
		DatabaseDriver:    "sqlite",
		CurrencyBase:      "USD",
		AllowRegistration: true,
		RateLimitWindow:   15 * time.Minute,
		AppURL:            "https://econumo.example.test",
	}, db, server.Seams{Avatars: appuser.FixedAvatarPicker(appuser.DefaultAvatar)})

	const callback = "https://share.example.test/cb"
	code, stdout, stderr := runApp(t, "app:register", "Share", callback, "--public")
	if code != 0 {
		t.Fatalf("app:register = %d %s", code, stderr)
	}
	id := clientID(t, stdout)

	do := func(method, target, token, contentType, body string) (int, map[string]any) {
		req := httptest.NewRequest(method, target, strings.NewReader(body))
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		req.Header.Set("Accept", "application/json, text/event-stream")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	jsonBody := func(v any) string { b, _ := json.Marshal(v); return string(b) }

	if status, out := do("POST", "/api/v1/user/register-user", "", "application/json",
		jsonBody(map[string]any{"email": "flow@example.test", "password": "flow-password-1", "name": "Flow"})); status != 200 {
		t.Fatalf("register-user: %d %v", status, out)
	}
	status, out := do("POST", "/api/v1/user/login-user", "", "application/json",
		jsonBody(map[string]any{"username": "flow@example.test", "password": "flow-password-1"}))
	session, _ := out["token"].(string)
	if status != 200 || session == "" {
		t.Fatalf("login-user: %d %v", status, out)
	}
	status, out = do("POST", "/api/v1/authserver/approve-authorization", session, "application/json", jsonBody(map[string]any{
		"clientId": id, "redirectUri": callback, "responseType": "code", "codeChallenge": "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
		"codeChallengeMethod": "S256", "resource": "https://econumo.example.test/mcp", "state": "st"}))
	data, _ := out["data"].(map[string]any)
	redirect, _ := data["redirectUrl"].(string)
	u, err := url.Parse(redirect)
	if status != 200 || err != nil || u.Query().Get("code") == "" {
		t.Fatalf("approve-authorization: %d %v", status, out)
	}
	status, tok := do("POST", "/oauth/token", "", "application/x-www-form-urlencoded", url.Values{
		"grant_type": {"authorization_code"}, "code": {u.Query().Get("code")}, "redirect_uri": {callback},
		"code_verifier": {"dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"}, "client_id": {id}}.Encode())
	access, _ := tok["access_token"].(string)
	if status != 200 || access == "" {
		t.Fatalf("token exchange: %d %v", status, tok)
	}
	const initBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`
	if status, out := do("POST", "/mcp", access, "application/json", initBody); status != http.StatusOK {
		t.Fatalf("mcp before delete: %d %v", status, out)
	}

	code, stdout, stderr = runApp(t, "app:delete", id)
	if code != 0 || !strings.Contains(stdout, "App "+id+" deleted (1 connection(s) revoked).") {
		t.Fatalf("app:delete = %d %q %q", code, stdout, stderr)
	}
	if status, out := do("POST", "/mcp", access, "application/json", initBody); status != http.StatusUnauthorized {
		t.Fatalf("mcp after delete: %d %v", status, out)
	}
	var clients, grants int
	if err := db.QueryRow(`SELECT COUNT(*) FROM oauth_clients WHERE id = ?`, id).Scan(&clients); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM oauth_grants WHERE client_id = ?`, id).Scan(&grants); err != nil {
		t.Fatal(err)
	}
	if clients != 0 || grants != 0 {
		t.Fatalf("rows left: clients=%d grants=%d", clients, grants)
	}
	if code, _, _ := runApp(t, "app:delete", id); code != 1 {
		t.Fatalf("second delete = %d", code)
	}
}

func TestAppConnections(t *testing.T) {
	owner, other := vo.NewId(), vo.NewId()
	grants := []model.OAuthGrant{{UserID: owner}, {UserID: other}}
	for _, tc := range []struct {
		name   string
		client model.OAuthClient
		want   string
	}{
		{"official", model.OAuthClient{IsGlobal: true}, "2"},
		{"unowned user app", model.OAuthClient{}, "2"},
		{"owned user app", model.OAuthClient{OwnerUserID: &owner}, "2 (1 from other users)"},
	} {
		if got := appConnections(model.OAuthApp{Client: tc.client, Grants: grants}); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
	if got := appConnections(model.OAuthApp{Client: model.OAuthClient{OwnerUserID: &owner}, Grants: grants[:1]}); got != "1" {
		t.Errorf("owner only: %q", got)
	}
}
