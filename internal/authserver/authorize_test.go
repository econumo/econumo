package authserver

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

func register(t *testing.T, s *Service, name string, uris ...string) model.ClientRegistrationResult {
	t.Helper()
	c, err := s.Register(ctx, model.ClientRegistrationRequest{ClientName: name, RedirectURIs: uris})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestDescribeAuthorization(t *testing.T) {
	s, _, _, user := newTestService(t)
	c := register(t, s, "Claude", "https://claude.ai/api/mcp/auth_callback")
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
	// Any requested scope is tolerated; the grant is always mcp.
	for _, sc := range []string{"admin", "mcp", "openid profile", "mcp:tools"} {
		q = authReq(c.ClientID)
		q.Scope = sc
		if res, err := s.DescribeAuthorization(ctx, user, q); err != nil || res.ErrorRedirectURL != "" {
			t.Errorf("scope %q: %+v %v", sc, res, err)
		}
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
	c := register(t, s, "", "http://localhost:4000/callback")
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
	c := register(t, s, "Claude", "https://claude.ai/api/mcp/auth_callback")
	res, err := s.ApproveAuthorization(ctx, user, session, authReq(c.ClientID))
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
	r, err := s.ApproveAuthorization(ctx, user, session, bad)
	if err != nil || !strings.Contains(r.RedirectURL, "error=invalid_request") || strings.Contains(r.RedirectURL, "code=") {
		t.Fatalf("invalid request must redirect with an error, not a code: %+v %v", r, err)
	}
	if _, err := s.DeclineAuthorization(ctx, user, authReq(vo.NewId().String())); !hasCode(err, errs.CodeAuthServerClientNotFound) {
		t.Fatalf("decline unknown client: %v", err)
	}

	off := NewService(s.repo, creds, s.tx, s.clock, nil, "")
	if _, err := off.ApproveAuthorization(ctx, user, session, authReq(c.ClientID)); !hasCode(err, errs.CodeAuthServerDisabled) {
		t.Fatal(err)
	}
}

// A reclaim that commits after the auth middleware accepted the session has
// revoked it; the approval re-checks it under the user row lock and issues no
// code.
func TestApprove_RevokedPresenterGetsNoCode(t *testing.T) {
	s, creds, _, user := newTestService(t)
	c := register(t, s, "Claude", "https://claude.ai/api/mcp/auth_callback")
	creds.deadTokens = map[vo.Id]bool{session: true}
	res, err := s.ApproveAuthorization(ctx, user, session, authReq(c.ClientID))
	if _, ok := errs.AsUnauthorized(err); !ok || res.RedirectURL != "" {
		t.Fatalf("want unauthorized and no redirect, got %+v %v", res, err)
	}
	if len(creds.locked) != 1 || !creds.locked[0].Equal(user) {
		t.Fatalf("the presenter check must run under the user lock: %v", creds.locked)
	}
	got, err := s.repo.GetClient(ctx, vo.MustParseId(c.ClientID))
	if err != nil || got.LastUsedAt != nil {
		t.Fatalf("a refused approval must write nothing: %+v %v", got, err)
	}
}

func TestRedirectKeepsExistingQuery(t *testing.T) {
	s, _, _, user := newTestService(t)
	c := register(t, s, "", "https://a.test/cb?app=1")
	q := authReq(c.ClientID)
	q.RedirectURI = "https://a.test/cb?app=1"
	res, err := s.ApproveAuthorization(ctx, user, session, q)
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

func TestErrorRedirectCarriesHostLabel(t *testing.T) {
	s, _, _, user := newTestService(t)
	c := register(t, s, "Claude", "https://claude.ai/api/mcp/auth_callback")
	q := authReq(c.ClientID)
	q.Resource = "https://other.test/mcp"
	res, err := s.DescribeAuthorization(ctx, user, q)
	if err != nil || res.ErrorRedirectURL == "" || res.ClientName != "Claude" || res.RedirectHost != "claude.ai" || res.IsLoopback {
		t.Fatalf("%+v %v", res, err)
	}
	lb := register(t, s, "Codex", "http://127.0.0.1:1/cb")
	q = authReq(lb.ClientID)
	q.RedirectURI = "http://127.0.0.1:9/cb"
	q.ResponseType = "token"
	res, err = s.DescribeAuthorization(ctx, user, q)
	if err != nil || res.ErrorRedirectURL == "" || !res.IsLoopback || res.RedirectHost != "127.0.0.1" {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestChallengeAndStateShape(t *testing.T) {
	s, _, _, user := newTestService(t)
	c := register(t, s, "Claude", "https://claude.ai/api/mcp/auth_callback")
	for name, mut := range map[string]func(*model.AuthorizationRequest){
		"empty challenge":    func(q *model.AuthorizationRequest) { q.CodeChallenge = "" },
		"short challenge":    func(q *model.AuthorizationRequest) { q.CodeChallenge = "abc" },
		"long challenge":     func(q *model.AuthorizationRequest) { q.CodeChallenge += "A" },
		"padded challenge":   func(q *model.AuthorizationRequest) { q.CodeChallenge = q.CodeChallenge[:42] + "=" },
		"non-base64url char": func(q *model.AuthorizationRequest) { q.CodeChallenge = q.CodeChallenge[:42] + "+" },
		"oversized state":    func(q *model.AuthorizationRequest) { q.State = strings.Repeat("s", 1025) },
	} {
		q := authReq(c.ClientID)
		mut(&q)
		res, err := s.DescribeAuthorization(ctx, user, q)
		if err != nil || !strings.Contains(res.ErrorRedirectURL, "error=invalid_request") {
			t.Errorf("%s: %+v %v", name, res, err)
		}
	}
	q := authReq(c.ClientID)
	q.State = strings.Repeat("s", 1025)
	if res, _ := s.DescribeAuthorization(ctx, user, q); strings.Contains(res.ErrorRedirectURL, "state=") {
		t.Errorf("a rejected state must not be echoed: %s", res.ErrorRedirectURL)
	}
	q = authReq(c.ClientID)
	q.State = strings.Repeat("s", 1024)
	if res, err := s.DescribeAuthorization(ctx, user, q); err != nil || res.ErrorRedirectURL != "" {
		t.Errorf("state at the cap must pass: %+v %v", res, err)
	}
}

const claudeCallback = "https://claude.ai/api/mcp/auth_callback"

func owner(t *testing.T, s *Service, clientID string) *vo.Id {
	t.Helper()
	c, err := s.repo.GetClient(ctx, vo.MustParseId(clientID))
	if err != nil {
		t.Fatal(err)
	}
	return c.OwnerUserID
}

func TestApprove_ClaimsUnownedClient(t *testing.T) {
	s, _, _, db := newTestServiceDB(t)
	a := newUser(t, db, fixture.User{})
	c := register(t, s, "Claude", claudeCallback)
	if o := owner(t, s, c.ClientID); o != nil {
		t.Fatalf("a fresh user app has no owner: %v", o)
	}
	if _, err := s.ApproveAuthorization(ctx, a, session, authReq(c.ClientID)); err != nil {
		t.Fatal(err)
	}
	if o := owner(t, s, c.ClientID); o == nil || !o.Equal(a) {
		t.Fatalf("the first approver must own the app: %v", o)
	}
}

func TestOtherUserIsRefused(t *testing.T) {
	s, _, _, db := newTestServiceDB(t)
	const ownerEmail = "owner-a@example.test"
	a := newUser(t, db, fixture.User{Email: ownerEmail})
	b := newUser(t, db, fixture.User{})
	c := register(t, s, "Claude", claudeCallback)
	if _, err := s.ApproveAuthorization(ctx, a, session, authReq(c.ClientID)); err != nil {
		t.Fatal(err)
	}
	mismatched := authReq(c.ClientID)
	mismatched.RedirectURI = "https://evil.test/cb"
	for name, q := range map[string]model.AuthorizationRequest{"registered uri": authReq(c.ClientID), "unregistered uri": mismatched} {
		checkErr := func(op string, err error) {
			t.Helper()
			if !hasCode(err, errs.CodeAuthServerAppNotYours) {
				t.Fatalf("%s %s: want app_not_yours, got %v", name, op, err)
			}
			if msg := err.Error(); strings.Contains(msg, a.String()) || strings.Contains(msg, ownerEmail) {
				t.Fatalf("%s %s: the error must not name the owner: %q", name, op, msg)
			}
		}
		d, err := s.DescribeAuthorization(ctx, b, q)
		checkErr("describe", err)
		if d.ErrorRedirectURL != "" {
			t.Fatalf("%s describe must not redirect: %+v", name, d)
		}
		r, err := s.ApproveAuthorization(ctx, b, session, q)
		checkErr("approve", err)
		if r.RedirectURL != "" {
			t.Fatalf("%s approve must not redirect: %+v", name, r)
		}
		r, err = s.DeclineAuthorization(ctx, b, q)
		checkErr("decline", err)
		if r.RedirectURL != "" {
			t.Fatalf("%s decline must not redirect: %+v", name, r)
		}
	}
	if n := codeCount(t, db, b); n != 0 {
		t.Fatalf("no code may be issued to a stranger: %d", n)
	}
	if o := owner(t, s, c.ClientID); o == nil || !o.Equal(a) {
		t.Fatalf("owner changed: %v", o)
	}
}

func TestOwnerReconnects(t *testing.T) {
	s, _, _, db := newTestServiceDB(t)
	a := newUser(t, db, fixture.User{})
	c := register(t, s, "Claude", claudeCallback)
	for i := range 2 {
		if _, err := s.ApproveAuthorization(ctx, a, session, authReq(c.ClientID)); err != nil {
			t.Fatalf("approval %d: %v", i, err)
		}
	}
	if o := owner(t, s, c.ClientID); o == nil || !o.Equal(a) {
		t.Fatalf("owner: %v", o)
	}
}

func TestDecline_DoesNotClaim(t *testing.T) {
	s, _, _, db := newTestServiceDB(t)
	a := newUser(t, db, fixture.User{})
	b := newUser(t, db, fixture.User{})
	c := register(t, s, "Claude", claudeCallback)
	if _, err := s.DeclineAuthorization(ctx, a, authReq(c.ClientID)); err != nil {
		t.Fatal(err)
	}
	if o := owner(t, s, c.ClientID); o != nil {
		t.Fatalf("a decline must not claim the app: %v", o)
	}
	if _, err := s.ApproveAuthorization(ctx, b, session, authReq(c.ClientID)); err != nil {
		t.Fatal(err)
	}
	if o := owner(t, s, c.ClientID); o == nil || !o.Equal(b) {
		t.Fatalf("owner: %v", o)
	}
}

func TestOfficialApp_UsableByEveryone(t *testing.T) {
	s, _, clock, db := newTestServiceDB(t)
	a := newUser(t, db, fixture.User{})
	b := newUser(t, db, fixture.User{})
	id := insertOfficialClient(t, s, clock.Now(), claudeCallback)
	for _, u := range []vo.Id{a, b} {
		d, err := s.DescribeAuthorization(ctx, u, authReq(id.String()))
		if err != nil || !d.IsOfficial {
			t.Fatalf("describe: %+v %v", d, err)
		}
		if _, err := s.ApproveAuthorization(ctx, u, session, authReq(id.String())); err != nil {
			t.Fatal(err)
		}
	}
	if o := owner(t, s, id.String()); o != nil {
		t.Fatalf("an official app never gets an owner: %v", o)
	}
	c := register(t, s, "Claude", claudeCallback)
	if d, err := s.DescribeAuthorization(ctx, a, authReq(c.ClientID)); err != nil || d.IsOfficial {
		t.Fatalf("a user app is not official: %+v %v", d, err)
	}
}

// racingRepo lets another user's claim land between validate and the approver's claim.
type racingRepo struct {
	Repository
	before func(ctx context.Context)
}

func (r racingRepo) ClaimClient(ctx context.Context, id, userID vo.Id) (int64, error) {
	r.before(ctx)
	return r.Repository.ClaimClient(ctx, id, userID)
}

func TestApprove_LosesClaimRace(t *testing.T) {
	s, creds, clock, db := newTestServiceDB(t)
	a := newUser(t, db, fixture.User{})
	b := newUser(t, db, fixture.User{})
	c := register(t, s, "Claude", claudeCallback)
	id := vo.MustParseId(c.ClientID)
	inner := s.repo
	racing := NewService(racingRepo{Repository: inner, before: func(ctx context.Context) {
		if n, err := inner.ClaimClient(ctx, id, b); err != nil || n != 1 {
			t.Errorf("rival claim: %d %v", n, err)
		}
	}}, creds, s.tx, clock, nil, testURL)
	res, err := racing.ApproveAuthorization(ctx, a, session, authReq(c.ClientID))
	if !hasCode(err, errs.CodeAuthServerAppNotYours) || res.RedirectURL != "" {
		t.Fatalf("the loser must get app_not_yours and no redirect: %+v %v", res, err)
	}
	if n := codeCount(t, db, a); n != 0 {
		t.Fatalf("the loser must get no code: %d", n)
	}
	if got, err := s.repo.GetClient(ctx, id); err != nil || got.LastUsedAt != nil {
		t.Fatalf("a lost race must not mark the client used: %+v %v", got, err)
	}
}

func TestApprove_ConcurrentApprovalsOneWins(t *testing.T) {
	s, _, _, db := newTestServiceDB(t)
	users := []vo.Id{newUser(t, db, fixture.User{}), newUser(t, db, fixture.User{})}
	c := register(t, s, "Claude", claudeCallback)
	errsOut := make([]error, len(users))
	var wg sync.WaitGroup
	for i, u := range users {
		wg.Go(func() {
			_, errsOut[i] = s.ApproveAuthorization(ctx, u, session, authReq(c.ClientID))
		})
	}
	wg.Wait()
	winner := -1
	for i, err := range errsOut {
		switch {
		case err == nil:
			if winner != -1 {
				t.Fatal("both approvals succeeded")
			}
			winner = i
		case !hasCode(err, errs.CodeAuthServerAppNotYours):
			t.Fatalf("loser error: %v", err)
		}
	}
	if winner == -1 {
		t.Fatalf("no approval succeeded: %v", errsOut)
	}
	if o := owner(t, s, c.ClientID); o == nil || !o.Equal(users[winner]) {
		t.Fatalf("owner %v, winner %v", o, users[winner])
	}
	if n := codeCount(t, db, users[1-winner]); n != 0 {
		t.Fatalf("the loser must get no code: %d", n)
	}
}

// codeDeletingRepo has app:delete land between validate and the code INSERT of
// an official app, whose claim returns early. As in deletingRepo, the DELETE
// runs inside the approval's transaction on SQLite and GetClient then reports
// what the committed DELETE would show.
type codeDeletingRepo struct {
	Repository
	commitDelete bool
	gone         bool
}

func (r *codeDeletingRepo) InsertCode(ctx context.Context, c *model.OAuthAuthorizationCode) error {
	if _, err := r.Repository.DeleteClient(ctx, c.ClientID); err != nil {
		return err
	}
	r.gone = r.commitDelete
	return r.Repository.InsertCode(ctx, c)
}

func (r *codeDeletingRepo) GetClient(ctx context.Context, id vo.Id) (*model.OAuthClient, error) {
	if r.gone {
		return nil, errs.NewNotFound("OAuth client not found")
	}
	return r.Repository.GetClient(ctx, id)
}

func TestApprove_ClientDeletedMidApproval(t *testing.T) {
	for _, committed := range []bool{true, false} {
		t.Run(fmt.Sprint("committed=", committed), func(t *testing.T) {
			s, creds, clock, db := newTestServiceDB(t)
			user := newUser(t, db, fixture.User{})
			id := insertOfficialClient(t, s, clock.Now(), claudeCallback)
			racing := NewService(&codeDeletingRepo{Repository: s.repo, commitDelete: committed}, creds, s.tx, clock, nil, testURL)
			_, err := racing.ApproveAuthorization(ctx, user, session, authReq(id.String()))
			if committed {
				if !hasCode(err, errs.CodeAuthServerClientNotFound) {
					t.Fatalf("want client_not_found, got %v", err)
				}
				return
			}
			if err == nil || hasCode(err, errs.CodeAuthServerClientNotFound) {
				t.Fatalf("a code INSERT failure with the client still present must surface as is: %v", err)
			}
		})
	}
}
