package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/infra/oidc"
	"github.com/econumo/econumo/internal/infra/oidc/oidctest"
	"github.com/econumo/econumo/internal/model"
	appoauth "github.com/econumo/econumo/internal/oauth"
	handleroauth "github.com/econumo/econumo/internal/oauth/api"
	oauthrepo "github.com/econumo/econumo/internal/oauth/repo"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/authstub"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	"github.com/econumo/econumo/internal/web/router"
)

// fixedClock is a pointer so a test can advance it after the service is built.
type fixedClock struct{ t time.Time }

func (c *fixedClock) Now() time.Time { return c.t }

// fakeUsers is the Users port over the seeded users table: rows are real (so
// the FK constraints on identities/handoffs hold) but the aggregate is kept in
// memory, which is all the oauth service reads. Copied in spirit from
// internal/oauth/service_test.go, trimmed of the fault-injection knobs this
// package's tests don't need.
type fakeUsers struct {
	t       *testing.T
	db      *dbtest.DB
	byID    map[string]*model.User
	byEmail map[string]*model.User
}

func newFakeUsers(t *testing.T, db *dbtest.DB) *fakeUsers {
	return &fakeUsers{t: t, db: db, byID: map[string]*model.User{}, byEmail: map[string]*model.User{}}
}

func (f *fakeUsers) seed(t *testing.T, email string, algorithm string) *model.User {
	id := fixture.New(t, f.db).User(fixture.User{Email: email, Algorithm: algorithm})
	u := &model.User{ID: vo.MustParseId(id), Email: email, Name: "Seed", IsActive: true, Algorithm: algorithm, EmailVerified: true}
	f.byID[id] = u
	f.byEmail[strings.ToLower(email)] = u
	return u
}

func (f *fakeUsers) FindByEmail(_ context.Context, email string) (*model.User, error) {
	if u, ok := f.byEmail[strings.ToLower(email)]; ok {
		return u, nil
	}
	return nil, errs.NewNotFound("User not found")
}

func (f *fakeUsers) FindByID(_ context.Context, id vo.Id) (*model.User, error) {
	if u, ok := f.byID[id.String()]; ok {
		return u, nil
	}
	return nil, errs.NewNotFound("User not found")
}

func (f *fakeUsers) ProvisionExternal(_ context.Context, name, email string) (*model.User, error) {
	u := f.seed(f.t, email, model.AlgorithmNone)
	u.Name = name
	return u, nil
}

func (f *fakeUsers) ReplaceVerifiedEmail(_ context.Context, userID vo.Id, email string) error {
	return nil
}
func (f *fakeUsers) RevokeAllSessions(_ context.Context, _ vo.Id) error { return nil }
func (f *fakeUsers) MarkEmailVerified(_ context.Context, _ vo.Id) error { return nil }

// MintSession returns a session-shaped token: eco_ses_ + 43 chars, matching
// the real access-token format so tests can assert on the wire shape.
func (f *fakeUsers) MintSession(_ context.Context, userID vo.Id, _ string, provider string, idToken *string) (*model.LoginResult, error) {
	tok, err := oidc.RandomToken()
	if err != nil {
		return nil, err
	}
	return &model.LoginResult{Token: "eco_ses_" + tok, User: model.CurrentUserResult{Id: userID.String()}}, nil
}

type harness struct {
	t     *testing.T
	srv   *httptest.Server
	fake  *oidctest.Fake
	users *fakeUsers
}

func newHarness(t *testing.T) *harness { return newHarnessWith(t, nil) }

func newHarnessWith(t *testing.T, limiter appoauth.AttemptLimiter) *harness {
	t.Helper()
	db := dbtest.New(t)
	f := oidctest.New(t)
	users := newFakeUsers(t, db)
	ids := oauthrepo.NewIdentityRepo(db.Engine, db.TX)
	states := oauthrepo.NewStateRepo(db.Engine, db.TX)
	hands := oauthrepo.NewHandoffRepo(db.Engine, db.TX)
	clk := &fixedClock{t: time.Now().UTC().Truncate(time.Second)}

	appleIssuer := f.Issuer(model.OAuthProviderApple, true)
	appleIssuer.UsePKCE = false
	appleIssuer.ResponseMode = "form_post"
	appleIssuer.Scopes = []string{"name", "email"}

	providers := []appoauth.Provider{
		{Client: oidc.NewClient(f.Issuer(model.OAuthProviderGoogle, true), nil), Name: "Google"},
		{Client: oidc.NewClient(appleIssuer, nil), Name: "Apple"},
		{Client: oidc.NewClient(f.Issuer(model.OAuthProviderOIDC, false), nil), Name: "Authentik"},
	}
	svc := appoauth.NewService(providers, users, ids, states, hands, db.TX, clk, limiter, "https://app.example.test", true)
	handlers := handleroauth.NewHandlers(svc)

	cfg := config.Config{CORSAllowedOrigins: []string{"*"}}
	h := router.New(router.Deps{
		Cfg:         cfg,
		DB:          nil,
		RegisterAPI: handleroauth.RegisterAPI(handlers, authstub.Authenticator{}),
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	return &harness{t: t, srv: srv, fake: f, users: users}
}

// noRedirectClient never follows a 3xx response, so a test can inspect the
// Location header the callback handlers produce.
func noRedirectClient() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func (h *harness) rawGet(t *testing.T, path string) *http.Response {
	t.Helper()
	resp, err := noRedirectClient().Get(h.srv.URL + path)
	if err != nil {
		t.Fatalf("get %s: %v", path, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func (h *harness) rawPostForm(t *testing.T, path string, form url.Values) *http.Response {
	t.Helper()
	resp, err := noRedirectClient().PostForm(h.srv.URL+path, form)
	if err != nil {
		t.Fatalf("post form %s: %v", path, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func (h *harness) doRaw(t *testing.T, method, path, token string, body any) (int, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, h.srv.URL+path, rdr)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := h.srv.Client().Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

func (h *harness) do(t *testing.T, method, path, token string, body any) (int, envelope) {
	t.Helper()
	status, raw := h.doRaw(t, method, path, token, body)
	var env envelope
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatalf("decode envelope (status %d): %v\nbody: %s", status, err, raw)
		}
	}
	env.raw = raw
	return status, env
}

// issueToken hands back a bearer token authstub accepts: the token IS the
// user id string, so any well-formed UUID authenticates as that user.
func (h *harness) issueToken(t *testing.T) string {
	t.Helper()
	return vo.NewId().String()
}

type envelope struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Code    int             `json:"code"`
	Data    json.RawMessage `json:"data"`
	Errors  json.RawMessage `json:"errors"`
	raw     []byte
}
