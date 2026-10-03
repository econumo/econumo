package oauthhttp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/authserver"
	"github.com/econumo/econumo/internal/authserver/oauthhttp"
	authrepo "github.com/econumo/econumo/internal/authserver/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	testURL  = "https://econumo.example.test"
	verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	callback = "https://claude.ai/api/mcp/auth_callback"
)

var ctx = context.Background()

type fakeCreds struct{ issued int }

func (f *fakeCreds) LockForOAuth(context.Context, vo.Id) (int64, error)      { return 0, nil }
func (f *fakeCreds) IsTokenLive(context.Context, vo.Id, vo.Id) (bool, error) { return true, nil }
func (f *fakeCreds) PurgeDeadOAuthTokens(context.Context, time.Time) (int64, error) {
	return 0, nil
}
func (f *fakeCreds) IssueOAuthAccessToken(context.Context, vo.Id, vo.Id, string, int64, time.Duration) (string, bool, error) {
	f.issued++
	return fmt.Sprintf("eco_oat_%d", f.issued), true, nil
}
func (f *fakeCreds) RevokeOAuthGrantTokens(context.Context, vo.Id) error { return nil }

type clock struct{}

func (clock) Now() time.Time { return time.Now().UTC() }

func newSvcOn(db *dbtest.DB, appURL string) *authserver.Service {
	return authserver.NewService(authrepo.NewRepo(db.Engine, db.TX), &fakeCreds{}, db.TX, clock{}, nil, appURL)
}

func newSvc(t *testing.T, appURL string) *authserver.Service {
	t.Helper()
	return newSvcOn(dbtest.New(t), appURL)
}

func do(h http.Handler, method, path, contentType string, body io.Reader) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func approveCode(t *testing.T, svc *authserver.Service, user vo.Id, clientID string) string {
	t.Helper()
	res, err := svc.ApproveAuthorization(ctx, user, vo.NewId(), model.AuthorizationRequest{
		ClientID: clientID, RedirectURI: callback, ResponseType: "code",
		CodeChallenge: "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM", CodeChallengeMethod: "S256",
		Resource: testURL + "/mcp",
	})
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(res.RedirectURL)
	return u.Query().Get("code")
}

func TestMetadata(t *testing.T) {
	for _, appURL := range []string{testURL, testURL + "/"} {
		svc := newSvc(t, appURL)
		h := oauthhttp.Handler(svc)
		rec := do(h, "GET", oauthhttp.AuthServerMetadataPath, "", nil)
		var m map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &m)
		if rec.Code != 200 || m["issuer"] != testURL || m["token_endpoint"] != testURL+"/oauth/token" ||
			m["authorization_endpoint"] != testURL+"/oauth/authorize" || m["registration_endpoint"] != testURL+"/oauth/register" {
			t.Fatalf("%d %v", rec.Code, m)
		}
		if rec.Header().Get("Access-Control-Allow-Origin") != "*" || rec.Header().Get("Content-Type") != "application/json" {
			t.Fatal("headers", rec.Header())
		}
		for _, p := range []string{oauthhttp.ProtectedResourcePath, oauthhttp.ProtectedResourceMCPPath} {
			rec = do(h, "GET", p, "", nil)
			m = nil
			_ = json.Unmarshal(rec.Body.Bytes(), &m)
			if m["resource"] != testURL+"/mcp" || fmt.Sprint(m["authorization_servers"]) != "["+testURL+"]" {
				t.Fatalf("%s: %v", p, m)
			}
		}
		want := `Bearer resource_metadata="` + testURL + `/.well-known/oauth-protected-resource/mcp", scope="mcp"`
		if c := oauthhttp.Challenge(svc); c != want {
			t.Fatal(c)
		}
	}
}

func TestRegisterEndpoint(t *testing.T) {
	svc := newSvc(t, testURL)
	h := oauthhttp.Handler(svc)
	rec := do(h, "POST", oauthhttp.RegisterPath, "application/json", strings.NewReader(`{"client_name":"Claude","redirect_uris":["`+callback+`"],"token_endpoint_auth_method":"none"}`))
	if rec.Code != 201 || rec.Header().Get("Cache-Control") != "no-store" || !strings.Contains(rec.Body.String(), `"client_id"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	rec = do(h, "POST", oauthhttp.RegisterPath, "application/json", strings.NewReader(`{"redirect_uris":["http://evil.test/cb"]}`))
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), `"error":"invalid_redirect_uri"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	rec = do(h, "POST", oauthhttp.RegisterPath, "application/json", strings.NewReader(`{not json`))
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), `"error":"invalid_client_metadata"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	big := `{"client_name":"` + strings.Repeat("a", 70<<10) + `"}`
	if rec = do(h, "POST", oauthhttp.RegisterPath, "application/json", strings.NewReader(big)); rec.Code != 400 {
		t.Fatalf("oversized body: %d", rec.Code)
	}
	rec = do(h, "OPTIONS", oauthhttp.RegisterPath, "", nil)
	if rec.Code != 204 || rec.Header().Get("Access-Control-Allow-Methods") == "" || rec.Header().Get("Access-Control-Allow-Headers") == "" {
		t.Fatal("preflight", rec.Code, rec.Header())
	}
}

func TestTokenEndpoint_FormAndBasicAuth(t *testing.T) {
	db := dbtest.New(t)
	user := vo.MustParseId(fixture.New(t, db).User(fixture.User{}))
	svc := newSvcOn(db, testURL)
	h := oauthhttp.Handler(svc)
	reg, err := svc.Register(ctx, model.ClientRegistrationRequest{RedirectURIs: []string{callback}, TokenEndpointAuthMethod: "client_secret_basic"})
	if err != nil {
		t.Fatal(err)
	}
	code := approveCode(t, svc, user, reg.ClientID)
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {callback}, "code_verifier": {verifier}}
	req := httptest.NewRequest("POST", oauthhttp.TokenPath, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Percent-encode by hand: UUIDs and base64url need no escaping, so
	// url.QueryEscape alone would send the raw values and prove nothing.
	req.SetBasicAuth(strings.ReplaceAll(reg.ClientID, "-", "%2D"), fmt.Sprintf("%%%02X", reg.ClientSecret[0])+reg.ClientSecret[1:])
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-store" || !strings.Contains(rec.Body.String(), `"token_type":"Bearer"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}

	// client_secret_post
	post, _ := svc.Register(ctx, model.ClientRegistrationRequest{RedirectURIs: []string{callback}, TokenEndpointAuthMethod: "client_secret_post"})
	form = url.Values{"grant_type": {"authorization_code"}, "code": {approveCode(t, svc, user, post.ClientID)}, "redirect_uri": {callback},
		"code_verifier": {verifier}, "client_id": {post.ClientID}, "client_secret": {post.ClientSecret}}
	if rec = do(h, "POST", oauthhttp.TokenPath, "application/x-www-form-urlencoded", strings.NewReader(form.Encode())); rec.Code != 200 {
		t.Fatalf("post auth: %d %s", rec.Code, rec.Body)
	}

	// malformed percent-escape -> 401 invalid_client
	req = httptest.NewRequest("POST", oauthhttp.TokenPath, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(reg.ClientID, "%zz")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 401 || !strings.Contains(rec.Body.String(), `"error":"invalid_client"`) {
		t.Fatalf("malformed escape: %d %s", rec.Code, rec.Body)
	}

	// wrong secret -> 401 invalid_client with a Basic challenge
	req = httptest.NewRequest("POST", oauthhttp.TokenPath, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(reg.ClientID, "wrong")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 401 || !strings.Contains(rec.Body.String(), `"error":"invalid_client"`) || !strings.HasPrefix(rec.Header().Get("WWW-Authenticate"), "Basic") {
		t.Fatalf("%d %s %v", rec.Code, rec.Body, rec.Header())
	}

	rec = do(h, "POST", oauthhttp.TokenPath, "application/x-www-form-urlencoded", strings.NewReader("grant_type=password"))
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), `"error":"unsupported_grant_type"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	// the query string is not a credential channel
	rec = do(h, "POST", oauthhttp.TokenPath+"?grant_type=refresh_token", "application/x-www-form-urlencoded", strings.NewReader(""))
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), `"error":"unsupported_grant_type"`) {
		t.Fatalf("query params honoured: %d %s", rec.Code, rec.Body)
	}
}

func TestMethodsAndDisabled(t *testing.T) {
	svc := newSvc(t, testURL)
	h := oauthhttp.Handler(svc)
	if rec := do(h, "GET", oauthhttp.TokenPath, "", nil); rec.Code != 405 {
		t.Errorf("GET token = %d", rec.Code)
	}

	off := newSvc(t, "")
	h = oauthhttp.Handler(off)
	if c := oauthhttp.Challenge(off); c != "" {
		t.Errorf("challenge = %q", c)
	}
	for _, p := range []string{oauthhttp.AuthServerMetadataPath, oauthhttp.ProtectedResourcePath, oauthhttp.ProtectedResourceMCPPath} {
		if rec := do(h, "GET", p, "", nil); rec.Code != 404 {
			t.Errorf("%s = %d", p, rec.Code)
		}
	}
	for _, p := range []string{oauthhttp.RegisterPath, oauthhttp.TokenPath} {
		if rec := do(h, "POST", p, "application/json", strings.NewReader("{}")); rec.Code != 404 || !strings.Contains(rec.Body.String(), `"error":"invalid_request"`) {
			t.Errorf("%s = %d %s", p, rec.Code, rec.Body)
		}
	}
}
