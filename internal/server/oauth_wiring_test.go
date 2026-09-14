package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/infra/clock"
	"github.com/econumo/econumo/internal/infra/oidc"
	"github.com/econumo/econumo/internal/infra/ratelimit"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	appuser "github.com/econumo/econumo/internal/user"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

func TestBuildAPI_MountsOAuthRoutesAndProviderList(t *testing.T) {
	db := dbtest.NewSQLite(t)
	cfg := config.Config{DatabaseDriver: db.Engine, CurrencyBase: "USD", AllowRegistration: true,
		AppURL: "https://app.example.test", OAuthGoogleClientID: "g", OAuthGoogleClientSecret: "s",
		RateLimitWindow: 15 * time.Minute}
	srv := httptest.NewServer(BuildAPI(cfg, db.Raw, Seams{}))
	t.Cleanup(srv.Close)
	resp, err := http.Get(srv.URL + "/api/v1/oauth/get-provider-list")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || !strings.Contains(string(body), `{"id":"google","name":"Google"}`) {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
}

func TestBuildAPI_NoProvidersIsEmptyList(t *testing.T) {
	db := dbtest.NewSQLite(t)
	cfg := config.Config{DatabaseDriver: db.Engine, CurrencyBase: "USD", RateLimitWindow: 15 * time.Minute}
	srv := httptest.NewServer(BuildAPI(cfg, db.Raw, Seams{}))
	t.Cleanup(srv.Close)
	resp, _ := http.Get(srv.URL + "/api/v1/oauth/get-provider-list")
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"data":[]`) {
		t.Fatalf("%s", body)
	}
}

// TestOAuthUsers_FullyHydratesFromRealUserService is a deferred finding from
// Task 8: the last-identity-unlink guard depends on OAuthUsers.FindByID/
// FindByEmail returning the SAME hydration as the rest of the user feature
// (in particular users.Algorithm, which HasPassword() reads). A partial read
// path (e.g. swapping in a summary/read-model lookup) would silently make
// every OAuth-provisioned user look password-protected, which would let the
// unlink guard be bypassed for the account's only identity, or would break
// FindByEmail resolution outright. Build a real *appuser.Service the same way
// Build() does and assert both lookups round-trip a seeded passwordless user.
func TestOAuthUsers_FullyHydratesFromRealUserService(t *testing.T) {
	db := dbtest.NewSQLite(t)
	fx := fixture.New(t, db)
	userID := fx.User(fixture.User{Email: "oauth-user@example.test", Algorithm: "none"})

	txm := backend.NewTxManager(db.Raw)
	userRepo := userrepo.NewRepo(db.Engine, txm)
	accessTokens := userrepo.NewAccessTokenRepo(db.Engine, txm)
	authLimiter := ratelimit.New(ratelimit.Config{Window: 15 * time.Minute, Global: 60}, clock.New())
	userSvc := appuser.NewService(
		userRepo, txm, auth.NewEncodeService(""), auth.NewPasswordHasher(), accessTokens,
		nil, nil, nil, nil, nil, nil, nil, nil,
		appuser.FixedAvatarPicker(appuser.DefaultAvatar), clock.New(), authLimiter, true, 0, false,
	)

	oauthUsers := NewOAuthUsers(userSvc)
	ctx := context.Background()

	byID, err := oauthUsers.FindByID(ctx, vo.MustParseId(userID))
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if byID.HasPassword() {
		t.Fatalf("passwordless user hydrated with HasPassword()=true: %+v", byID)
	}

	byEmail, err := oauthUsers.FindByEmail(ctx, "oauth-user@example.test")
	if err != nil {
		t.Fatalf("FindByEmail: %v", err)
	}
	if byEmail.ID.Value() != userID {
		t.Fatalf("FindByEmail resolved id %q, want %q", byEmail.ID.Value(), userID)
	}
}

var wiringResetCodeRe = regexp.MustCompile(`code is: (\d{6})`)

// TestRecovery_ReclaimsAnAccountFromASquatter drives the reported takeover end
// to end against the real handler. Someone registers an address they do not
// own, links their own provider identity to it, and mints a personal token.
// The rightful owner then does the only thing they can: reset the password with
// a code sent to their mailbox. That reset is the account's ownership proof, so
// nothing the squatter left may survive it — not the password, not the token,
// and not the linked sign-in method.
func TestRecovery_ReclaimsAnAccountFromASquatter(t *testing.T) {
	db := dbtest.NewSQLite(t)
	cfg := config.Config{DatabaseDriver: db.Engine, CurrencyBase: "USD", AllowRegistration: true,
		AppURL: "https://app.example.test", OAuthGoogleClientID: "g", OAuthGoogleClientSecret: "s",
		RateLimitWindow: 15 * time.Minute, RateLimitGlobal: 60}
	mail := &captureMailer{}
	srv := httptest.NewServer(BuildAPI(cfg, db.Raw, Seams{Mailer: mail}))
	t.Cleanup(srv.Close)

	post := func(path, token string, body string) (int, string) {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(raw)
	}
	tokenOf := func(body string) string {
		t.Helper()
		// login answers with the raw {token,user} body; create-personal-token
		// wraps its one-time token in the standard envelope.
		var v struct {
			Token string `json:"token"`
			Data  struct {
				Token string `json:"token"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &v); err != nil {
			t.Fatalf("token from %s: %v", body, err)
		}
		if v.Token != "" {
			return v.Token
		}
		return v.Data.Token
	}

	// The squatter registers the victim's address and signs in.
	if code, body := post("/api/v1/user/register-user", "",
		`{"name":"Squatter","email":"victim@example.test","password":"squatter-pass"}`); code != 200 {
		t.Fatalf("register: %d %s", code, body)
	}
	_, loginBody := post("/api/v1/user/login-user", "", `{"username":"victim@example.test","password":"squatter-pass"}`)
	squatterSession := tokenOf(loginBody)
	if squatterSession == "" {
		t.Fatalf("login: %s", loginBody)
	}
	_, patBody := post("/api/v1/user/create-personal-token", squatterSession, `{"name":"squatter-ci"}`)
	squatterPAT := tokenOf(patBody)
	if squatterPAT == "" {
		t.Fatalf("create-personal-token: %s", patBody)
	}
	// ...and links their OWN provider account to it (the identity a credential
	// eviction leaves behind).
	uid := userIDByEmail(t, db, "victim@example.test")
	fixture.New(t, db).Identity(fixture.Identity{UserID: uid, Provider: "google",
		Issuer: "https://accounts.google.com", Subject: "squatter-google-sub", Email: "squatter@example.test"})

	// ...leaves a pending email change to an address they control...
	if code, body := post("/api/v1/user/request-email-change", squatterSession,
		`{"newEmail":"squatter@example.test","password":"squatter-pass"}`); code != 200 {
		t.Fatalf("request-email-change: %d %s", code, body)
	}
	// ...and, moments before the reset, pockets a sign-in handoff for the
	// account: a 60-second code that mints a session with no further proof.
	// (Minted directly, because driving a real provider consent here would add
	// an issuer to a test about what the reset takes away.)
	handoffCode := "eco-pending-handoff-code"
	seedLoginHandoff(t, db, uid, handoffCode, "f10wsecret")

	// The victim reclaims through the mailbox they control.
	if code, body := post("/api/v1/user/remind-password", "", `{"username":"victim@example.test"}`); code != 200 {
		t.Fatalf("remind: %d %s", code, body)
	}
	m := wiringResetCodeRe.FindStringSubmatch(mail.msg.Text)
	if m == nil {
		t.Fatalf("no reset code in %q", mail.msg.Text)
	}
	if code, body := post("/api/v1/user/reset-password", "",
		`{"username":"victim@example.test","code":"`+m[1]+`","password":"victim-pass"}`); code != 200 {
		t.Fatalf("reset: %d %s", code, body)
	}

	// Nothing the squatter left behind still works.
	if code, _ := post("/api/v1/user/logout-user", squatterSession, `{}`); code != 401 {
		t.Errorf("squatter session still authenticates (%d)", code)
	}
	if code, _ := post("/api/v1/user/logout-user", squatterPAT, `{}`); code != 401 {
		t.Errorf("squatter personal token still authenticates (%d)", code)
	}
	if code, body := post("/api/v1/user/login-user", "", `{"username":"victim@example.test","password":"squatter-pass"}`); code == 200 {
		t.Errorf("squatter password still signs in: %d %s", code, body)
	}
	_, victimBody := post("/api/v1/user/login-user", "", `{"username":"victim@example.test","password":"victim-pass"}`)
	victimSession := tokenOf(victimBody)
	if victimSession == "" {
		t.Fatalf("the owner must be able to sign in: %s", victimBody)
	}
	if n := pendingEmailChanges(t, db, uid); n != 0 {
		t.Errorf("a pending email change survived the reclaim (%d rows)", n)
	}
	if code, body := post("/api/v1/oauth/exchange-handoff", "",
		`{"code":"`+handoffCode+`","flow":"f10wsecret"}`); code == 200 {
		t.Errorf("a handoff minted before the reset still bought a session: %d %s", code, body)
	}
	// The race the sweep alone cannot win: a callback that resolved this user
	// BEFORE the reset lands its handoff AFTER it (here, written straight to the
	// table, which is what that in-flight request would do). Sweeping found
	// nothing to delete — the fence is what refuses the redemption.
	seedLoginHandoff(t, db, uid, "late-arriving-code", "lateflow")
	if code, body := post("/api/v1/oauth/exchange-handoff", "",
		`{"code":"late-arriving-code","flow":"lateflow"}`); code == 200 {
		t.Errorf("a handoff from a flow that predates the reclaim still bought a session: %d %s", code, body)
	}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/oauth/get-identity-list", nil)
	req.Header.Set("Authorization", "Bearer "+victimSession)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	listed, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(listed), `"data":[]`) {
		t.Errorf("the squatter's sign-in method survived the reclaim: %s", listed)
	}
}

func userIDByEmail(t *testing.T, db *dbtest.DB, email string) string {
	t.Helper()
	var id string
	if err := db.Raw.QueryRow(db.Rebind(`SELECT id FROM users WHERE lower(email) = lower(?)`), email).Scan(&id); err != nil {
		t.Fatalf("user id for %s: %v", email, err)
	}
	return id
}

// seedLoginHandoff writes the row a resolved oauth callback would have left:
// an unredeemed sign-in code for the user, valid for the next minute.
func seedLoginHandoff(t *testing.T, db *dbtest.DB, userID, code, flow string) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	_, err := db.Raw.Exec(db.Rebind(`INSERT INTO oauth_handoffs
		(code_hash, kind, user_id, provider, issuer, subject, email, flow_hash, id_token, created_at, expires_at)
		VALUES (?, ?, ?, ?, '', '', '', ?, NULL, ?, ?)`),
		oidc.Sha256Hex(code), model.OAuthHandoffKindLogin, userID, "google", oidc.Sha256Hex(flow), now, now.Add(model.OAuthHandoffTTL))
	if err != nil {
		t.Fatalf("seed handoff: %v", err)
	}
}

func pendingEmailChanges(t *testing.T, db *dbtest.DB, userID string) int {
	t.Helper()
	var n int
	if err := db.Raw.QueryRow(db.Rebind(`SELECT COUNT(*) FROM users_email_change_requests WHERE user_id = ?`), userID).Scan(&n); err != nil {
		t.Fatalf("count email change requests: %v", err)
	}
	return n
}

// The login ordering guarantee, pinned at the repo level because the real race
// needs a hook: the generation Login presents must be the one that came out of
// the SAME row read as the password hash. A reset committing after that read
// bumps past it, so the session insert writes nothing. Reintroducing a separate
// generation read (a fresh GetByID here) would make this pass wrongly — and
// that is exactly the bug this pins shut.
func TestLogin_SessionInsertIsFencedByTheGenerationReadWithTheHash(t *testing.T) {
	db := dbtest.New(t)
	users := userrepo.NewRepo(db.Engine, db.TX)
	tokens := userrepo.NewAccessTokenRepo(db.Engine, db.TX)
	ctx := context.Background()
	email := "fenced-login@example.test"
	uid := vo.MustParseId(fixture.New(t, db).User(fixture.User{Email: email}))

	loaded, err := users.GetByEmail(ctx, email) // the evidence read: hash + generation in one row
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	if err := users.BumpCredentialsGeneration(ctx, uid); err != nil { // the reset commits
		t.Fatalf("bump: %v", err)
	}

	exp := time.Now().Add(time.Hour)
	n, err := tokens.InsertIfGeneration(ctx, &model.AccessToken{ID: vo.NewId(), UserID: uid, Kind: model.TokenKindSession,
		TokenHash: "h", CreatedAt: time.Now(), LastUsedAt: time.Now(), ExpiresAt: &exp}, loaded.CredentialsGeneration)
	if err != nil {
		t.Fatalf("InsertIfGeneration: %v", err)
	}
	if n != 0 {
		t.Fatalf("session minted on a pre-reclaim read: n=%d", n)
	}
}
