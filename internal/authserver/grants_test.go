package authserver

import (
	"net/url"
	"testing"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func TestConnectedApps(t *testing.T) {
	s, creds, _, user := newTestService(t)
	cid, code := approve(t, s, user)
	_, _ = exchange(s, cid, code)
	list, err := s.ListConnectedApps(ctx, user)
	if err != nil || len(list) != 1 || list[0].ClientName != "Claude" || list[0].RedirectHost != "claude.ai" {
		t.Fatalf("%+v %v", list, err)
	}
	stranger := vo.NewId() // revoke only compares owner ids, so no persisted user is needed
	if err := s.RevokeConnectedApp(ctx, stranger, vo.MustParseId(list[0].ID)); !hasCode(err, errs.CodeAuthServerGrantNotFound) {
		t.Fatal("foreign grant must be not found")
	}
	creds.locked = nil
	if err := s.RevokeConnectedApp(ctx, user, vo.MustParseId(list[0].ID)); err != nil {
		t.Fatal(err)
	}
	if len(creds.locked) != 1 || !creds.locked[0].Equal(user) {
		t.Fatalf("revoke must take the user row lock first: %v", creds.locked)
	}
	if len(creds.revoked) != 1 {
		t.Fatal("revoke must drop the grant's access tokens")
	}
	if l, _ := s.ListConnectedApps(ctx, user); len(l) != 0 {
		t.Fatal("still listed")
	}
	if err := s.RevokeConnectedApp(ctx, user, vo.MustParseId(list[0].ID)); !hasCode(err, errs.CodeAuthServerGrantNotFound) {
		t.Fatal("an already revoked grant must be not found")
	}
	cid, code = approve(t, s, user)
	_, _ = exchange(s, cid, code)
	if n, _ := s.RevokeAllForUser(ctx, user); n != 1 {
		t.Fatalf("revoke all = %d", n)
	}
}

func TestConnectedApps_IsOfficial(t *testing.T) {
	s, _, clock, user := newTestService(t)
	cid, code := approve(t, s, user)
	if _, err := exchange(s, cid, code); err != nil {
		t.Fatal(err)
	}
	official := insertOfficialClient(t, s, clock.Now(), "https://claude.ai/api/mcp/auth_callback")
	res, err := s.ApproveAuthorization(ctx, user, session, true, authReq(official.String()))
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(res.RedirectURL)
	if _, err := exchange(s, official.String(), u.Query().Get("code")); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListConnectedApps(ctx, user)
	if err != nil || len(list) != 2 {
		t.Fatalf("%+v %v", list, err)
	}
	for _, app := range list {
		if want := app.ClientName == "Official"; app.IsOfficial != want {
			t.Errorf("%s: isOfficial = %v", app.ClientName, app.IsOfficial)
		}
	}
}
