package authserver

import (
	"net/url"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func vo0() vo.Id { return vo.NewId() }

func TestDescribeAuthorization(t *testing.T) {
	s, _, _, user := newTestService(t)
	c, _ := s.Register(ctx, model.ClientRegistrationRequest{ClientName: "Claude", RedirectURIs: []string{"https://claude.ai/api/mcp/auth_callback"}})
	res, err := s.DescribeAuthorization(ctx, user, authReq(c.ClientID))
	if err != nil || res.ClientName != "Claude" || res.RedirectHost != "claude.ai" || res.ErrorRedirectURL != "" {
		t.Fatalf("%+v %v", res, err)
	}

	for _, r := range []string{"", testURL + "/mcp/"} {
		q := authReq(c.ClientID)
		q.Resource = r
		if res, err := s.DescribeAuthorization(ctx, user, q); err != nil || res.ErrorRedirectURL != "" {
			t.Errorf("resource %q: %+v %v", r, res, err)
		}
	}
	q := authReq(c.ClientID)
	q.Resource = "https://other.test/mcp"
	if res, _ := s.DescribeAuthorization(ctx, user, q); !strings.Contains(res.ErrorRedirectURL, "error=invalid_target") || !strings.Contains(res.ErrorRedirectURL, "state=st%261") {
		t.Errorf("bad resource: %+v", res)
	}
	q = authReq(c.ClientID)
	q.CodeChallengeMethod = "plain"
	if res, _ := s.DescribeAuthorization(ctx, user, q); !strings.Contains(res.ErrorRedirectURL, "error=invalid_request") {
		t.Errorf("plain pkce: %+v", res)
	}
	q = authReq(c.ClientID)
	q.Scope = "admin"
	if res, _ := s.DescribeAuthorization(ctx, user, q); !strings.Contains(res.ErrorRedirectURL, "error=invalid_scope") {
		t.Errorf("scope: %+v", res)
	}
	q = authReq(c.ClientID)
	q.ResponseType = "token"
	if res, _ := s.DescribeAuthorization(ctx, user, q); !strings.Contains(res.ErrorRedirectURL, "error=unsupported_response_type") {
		t.Errorf("rt: %+v", res)
	}

	if _, err := s.DescribeAuthorization(ctx, user, authReq(vo.NewId().String())); !hasCode(err, errs.CodeAuthServerClientNotFound) {
		t.Errorf("unknown client: %v", err)
	}
	if _, err := s.DescribeAuthorization(ctx, user, authReq("not-a-uuid")); !hasCode(err, errs.CodeAuthServerClientNotFound) {
		t.Errorf("malformed client id: %v", err)
	}
	q = authReq(c.ClientID)
	q.RedirectURI = "https://evil.test/cb"
	if _, err := s.DescribeAuthorization(ctx, user, q); !hasCode(err, errs.CodeAuthServerRedirectMismatch) {
		t.Errorf("mismatch: %v", err)
	}
}

func TestLoopbackRedirectRules(t *testing.T) {
	s, _, _, user := newTestService(t)
	c, _ := s.Register(ctx, model.ClientRegistrationRequest{RedirectURIs: []string{"http://localhost:4000/callback"}})
	q := authReq(c.ClientID)
	q.RedirectURI = "http://localhost:5555/callback"
	res, err := s.DescribeAuthorization(ctx, user, q)
	if err != nil || !res.IsLoopback || res.ErrorRedirectURL != "" {
		t.Fatalf("other port must be accepted: %+v %v", res, err)
	}
	q.RedirectURI = "http://127.0.0.1:4000/callback"
	if _, err := s.DescribeAuthorization(ctx, user, q); !hasCode(err, errs.CodeAuthServerRedirectMismatch) {
		t.Fatalf("host spelling must match: %v", err)
	}
}

func TestApproveAndDecline(t *testing.T) {
	s, creds, _, user := newTestService(t)
	creds.gen = 7
	c, _ := s.Register(ctx, model.ClientRegistrationRequest{ClientName: "Claude", RedirectURIs: []string{"https://claude.ai/api/mcp/auth_callback"}})
	res, err := s.ApproveAuthorization(ctx, user, authReq(c.ClientID))
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(res.RedirectURL)
	if u.Host != "claude.ai" || u.Query().Get("code") == "" || u.Query().Get("state") != "st&1" || u.Query().Get("iss") != testURL {
		t.Fatalf("%s", res.RedirectURL)
	}
	code, err := s.repo.ConsumeCode(ctx, hashSecret(u.Query().Get("code")))
	if err != nil || code.CredentialsGeneration != 7 || code.UserID != user || code.Resource != testURL+"/mcp" ||
		code.ExpiresAt.Sub(code.CreatedAt) != CodeTTL {
		t.Fatalf("stored code: %+v %v", code, err)
	}
	got, err := s.repo.GetClient(ctx, vo.MustParseId(c.ClientID))
	if err != nil || got.LastUsedAt == nil {
		t.Fatalf("approve must mark the client used: %+v %v", got, err)
	}

	d, err := s.DeclineAuthorization(ctx, user, authReq(c.ClientID))
	if err != nil || !strings.Contains(d.RedirectURL, "error=access_denied") || !strings.Contains(d.RedirectURL, "state=st%261") {
		t.Fatal(d.RedirectURL, err)
	}

	bad := authReq(c.ClientID)
	bad.CodeChallengeMethod = "plain"
	r, err := s.ApproveAuthorization(ctx, user, bad)
	if err != nil || !strings.Contains(r.RedirectURL, "error=invalid_request") || strings.Contains(r.RedirectURL, "code=") {
		t.Fatalf("invalid request must redirect with an error, not a code: %+v %v", r, err)
	}
	if _, err := s.DeclineAuthorization(ctx, user, authReq(vo.NewId().String())); !hasCode(err, errs.CodeAuthServerClientNotFound) {
		t.Fatalf("decline unknown client: %v", err)
	}

	off := NewService(s.repo, creds, s.tx, s.clock, nil, "")
	if _, err := off.ApproveAuthorization(ctx, user, authReq(c.ClientID)); !hasCode(err, errs.CodeAuthServerDisabled) {
		t.Fatal(err)
	}
}

func TestRedirectKeepsExistingQuery(t *testing.T) {
	s, _, _, user := newTestService(t)
	c, _ := s.Register(ctx, model.ClientRegistrationRequest{RedirectURIs: []string{"https://a.test/cb?app=1"}})
	q := authReq(c.ClientID)
	q.RedirectURI = "https://a.test/cb?app=1"
	res, err := s.ApproveAuthorization(ctx, user, q)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(res.RedirectURL)
	if u.Query().Get("app") != "1" || u.Query().Get("code") == "" {
		t.Fatal(res.RedirectURL)
	}
}

func TestConnectedAppRevokeValidate(t *testing.T) {
	for _, id := range []string{"", " ", "nope"} {
		if err := (model.RevokeConnectedAppRequest{ID: id}).Validate(); err == nil {
			t.Errorf("%q accepted", id)
		}
	}
	if err := (model.RevokeConnectedAppRequest{ID: vo.NewId().String()}).Validate(); err != nil {
		t.Fatal(err)
	}
}
