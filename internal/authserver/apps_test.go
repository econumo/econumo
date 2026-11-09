package authserver

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	authrepo "github.com/econumo/econumo/internal/authserver/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const appCallback = "https://claude.ai/api/mcp/auth_callback"

func approveCode(t *testing.T, s *Service, user vo.Id, clientID string) string {
	t.Helper()
	res, err := s.ApproveAuthorization(ctx, user, session, authReq(clientID))
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(res.RedirectURL)
	return u.Query().Get("code")
}

func exchangeWith(s *Service, clientID, secret, code string) (model.TokenResponse, error) {
	return s.Token(ctx, model.TokenRequest{GrantType: "authorization_code", ClientID: clientID, ClientSecret: secret, Code: code,
		RedirectURI: appCallback, CodeVerifier: verifier, Resource: testURL + "/mcp"})
}

func refreshSecret(s *Service, clientID, secret, rt string) (model.TokenResponse, error) {
	return s.Token(ctx, model.TokenRequest{GrantType: "refresh_token", ClientID: clientID, ClientSecret: secret, RefreshToken: rt})
}

func connect(t *testing.T, s *Service, user vo.Id, clientID, secret string) model.TokenResponse {
	t.Helper()
	tr, err := exchangeWith(s, clientID, secret, approveCode(t, s, user, clientID))
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

func TestRegisterOfficialApp(t *testing.T) {
	s, _, _, user := newTestService(t)
	c, secret, err := s.RegisterOfficialApp(ctx, " Claude ", []string{appCallback}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !c.IsGlobal || c.OwnerUserID != nil || c.Name != "Claude" || secret == "" || c.SecretHash == nil || hashSecret(secret) != *c.SecretHash {
		t.Fatalf("%+v %q", c, secret)
	}
	stored, err := s.repo.GetClient(ctx, c.ID)
	if err != nil || !stored.IsGlobal || stored.OwnerUserID != nil || stored.SecretHash == nil || *stored.SecretHash != *c.SecretHash {
		t.Fatalf("%+v %v", stored, err)
	}
	tr := connect(t, s, user, c.ID.String(), secret)
	if _, err := refreshSecret(s, c.ID.String(), secret, tr.RefreshToken); err != nil {
		t.Fatal(err)
	}

	pub, secret, err := s.RegisterOfficialApp(ctx, "Public", []string{appCallback}, true)
	if err != nil || pub.SecretHash != nil || secret != "" || !pub.IsGlobal {
		t.Fatalf("%+v %q %v", pub, secret, err)
	}
	connect(t, s, user, pub.ID.String(), "")
}

func TestRegisterOfficialApp_Validation(t *testing.T) {
	db := dbtest.New(t)
	// Disabled server: the operator commands work without ECONUMO_URL.
	s := NewService(authrepo.NewRepo(db.Engine, db.TX), &fakeCreds{}, db.TX, &fixedClock{t: time.Now().UTC()}, nil, "")
	ok := []string{appCallback}
	eleven := slices.Repeat(ok, 11)
	cases := []struct {
		name string
		uris []string
		want error
	}{
		{strings.Repeat("é", 101), ok, nil},
		{"​\n", ok, ErrAppNameRequired},
		{"", ok, ErrAppNameRequired},
		{"App", nil, nil},
		{"App", eleven, nil},
		{"App", []string{"https://a.test/" + strings.Repeat("a", 2049-len("https://a.test/"))}, nil},
		{"App", []string{"http://example.com/cb"}, nil},
	}
	for i, tc := range cases {
		_, secret, err := s.RegisterOfficialApp(ctx, tc.name, tc.uris, false)
		if err == nil || secret != "" {
			t.Errorf("case %d: err = %v", i, err)
			continue
		}
		if tc.want != nil && !errors.Is(err, tc.want) {
			t.Errorf("case %d: err = %v, want %v", i, err, tc.want)
		}
		var oe *OAuthError
		if errors.As(err, &oe) {
			t.Errorf("case %d: an operator error must be plain, got %v", i, err)
		}
	}
	if apps, err := s.ListApps(ctx); err != nil || len(apps) != 0 {
		t.Fatalf("nothing may be inserted: %+v %v", apps, err)
	}
	c, _, err := s.RegisterOfficialApp(ctx, strings.Repeat("é", 100), slices.Repeat(ok, 10), true)
	if err != nil || !c.IsGlobal {
		t.Fatalf("limits are inclusive: %+v %v", c, err)
	}
}

func TestListAndGetApps(t *testing.T) {
	s, _, clock, db := newTestServiceDB(t)
	a, b := newUser(t, db, fixture.User{}), newUser(t, db, fixture.User{})
	official, _, err := s.RegisterOfficialApp(ctx, "Official", []string{appCallback}, true)
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	cid, code := approve(t, s, a)
	if _, err := exchange(s, cid, code); err != nil {
		t.Fatal(err)
	}
	userApp := vo.MustParseId(cid)
	// A grant of another user predating the owner rule, and one revoked grant.
	insertGrant(t, s, b, userApp, clock.Now(), nil)
	revoked := clock.Now()
	insertGrant(t, s, b, userApp, clock.Now(), &revoked)

	app, err := s.GetApp(ctx, userApp)
	if err != nil {
		t.Fatal(err)
	}
	if app.Client.OwnerUserID == nil || !app.Client.OwnerUserID.Equal(a) || len(app.Grants) != 2 {
		t.Fatalf("%+v", app)
	}
	apps, err := s.ListApps(ctx)
	if err != nil || len(apps) != 2 || !apps[0].Client.ID.Equal(official.ID) || !apps[1].Client.ID.Equal(userApp) {
		t.Fatalf("%+v %v", apps, err)
	}
	if len(apps[0].Grants) != 0 || len(apps[1].Grants) != 2 {
		t.Fatalf("grants: %d %d", len(apps[0].Grants), len(apps[1].Grants))
	}
	clock.Advance(GrantIdleTTL + time.Hour)
	if app, _ := s.GetApp(ctx, userApp); len(app.Grants) != 0 {
		t.Fatal("expired grants are not live")
	}
	if _, err := s.GetApp(ctx, vo.NewId()); err == nil {
		t.Fatal("unknown id")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("want NotFound, got %v", err)
	}
}

func insertGrant(t *testing.T, s *Service, user, client vo.Id, now time.Time, revokedAt *time.Time) vo.Id {
	t.Helper()
	g := &model.OAuthGrant{ID: vo.NewId(), UserID: user, ClientID: client, RefreshTokenHash: hashSecret(vo.NewId().String()),
		CreatedAt: now, LastUsedAt: now, ExpiresAt: now.Add(GrantIdleTTL), RevokedAt: revokedAt}
	if err := s.repo.InsertGrant(ctx, g); err != nil {
		t.Fatal(err)
	}
	if revokedAt != nil {
		if _, err := s.repo.RevokeGrant(ctx, g.ID, *revokedAt); err != nil {
			t.Fatal(err)
		}
	}
	return g.ID
}

func TestUpdateOfficialApp(t *testing.T) {
	s, _, _, user := newTestService(t)
	c, _, err := s.RegisterOfficialApp(ctx, "Old", []string{appCallback}, true)
	if err != nil {
		t.Fatal(err)
	}
	connect(t, s, user, c.ID.String(), "")
	name := "New"
	if err := s.UpdateOfficialApp(ctx, c.ID, &name, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.repo.GetClient(ctx, c.ID); got.Name != "New" || !slices.Equal(got.RedirectURIs, []string{appCallback}) {
		t.Fatalf("%+v", got)
	}
	uris := []string{"https://a.test/cb", "http://127.0.0.1/cb"}
	if err := s.UpdateOfficialApp(ctx, c.ID, nil, uris); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.repo.GetClient(ctx, c.ID); got.Name != "New" || !slices.Equal(got.RedirectURIs, uris) {
		t.Fatalf("%+v", got)
	}
	name = "Both"
	if err := s.UpdateOfficialApp(ctx, c.ID, &name, []string{appCallback}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.repo.GetClient(ctx, c.ID); got.Name != "Both" || !slices.Equal(got.RedirectURIs, []string{appCallback}) {
		t.Fatalf("%+v", got)
	}
	tooLong := "https://a.test/" + strings.Repeat("a", 2049-len("https://a.test/"))
	wantInsecure := ValidateRedirectURI("http://example.com/cb")
	for i, tc := range []struct {
		uris []string
		want string
	}{
		{slices.Repeat([]string{appCallback}, 11), "between 1 and 10 redirect_uris are required"},
		{[]string{tooLong}, "redirect URI is too long"},
		{[]string{"http://example.com/cb"}, wantInsecure.Error()},
	} {
		if err := s.UpdateOfficialApp(ctx, c.ID, nil, tc.uris); err == nil || err.Error() != tc.want {
			t.Errorf("case %d: err = %v, want %q", i, err, tc.want)
		}
	}
	long, blank := strings.Repeat("x", 101), " \n"
	for _, tc := range []struct {
		name *string
		want error
	}{{&long, errors.New("client_name is too long")}, {&blank, ErrAppNameRequired}} {
		if err := s.UpdateOfficialApp(ctx, c.ID, tc.name, nil); err == nil || err.Error() != tc.want.Error() {
			t.Errorf("name %q: err = %v, want %v", *tc.name, err, tc.want)
		}
	}
	if got, _ := s.repo.GetClient(ctx, c.ID); got.Name != "Both" || !slices.Equal(got.RedirectURIs, []string{appCallback}) {
		t.Fatalf("a refused update changed the row: %+v", got)
	}
	if app, _ := s.GetApp(ctx, c.ID); len(app.Grants) != 1 {
		t.Fatal("live grants are kept")
	}

	cid, _ := approve(t, s, user)
	name = "Hijack"
	if err := s.UpdateOfficialApp(ctx, vo.MustParseId(cid), &name, []string{"https://evil.test/cb"}); !errors.Is(err, ErrAppNotOfficial) {
		t.Fatalf("err = %v", err)
	}
	if got, _ := s.repo.GetClient(ctx, vo.MustParseId(cid)); got.Name != "Claude" || !slices.Equal(got.RedirectURIs, []string{appCallback}) {
		t.Fatalf("user app changed: %+v", got)
	}
	if _, ok := errs.AsNotFound(s.UpdateOfficialApp(ctx, vo.NewId(), &name, nil)); !ok {
		t.Fatal("unknown id must be NotFound")
	}
}

func TestMakeAppOfficial_OwnerKeepsGrantOthersCanApprove(t *testing.T) {
	s, _, _, db := newTestServiceDB(t)
	a, b := newUser(t, db, fixture.User{}), newUser(t, db, fixture.User{})
	cid, code := approve(t, s, a)
	tr, err := exchange(s, cid, code)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApproveAuthorization(ctx, b, session, authReq(cid)); !hasCode(err, errs.CodeAuthServerAppNotYours) {
		t.Fatalf("before: %v", err)
	}
	id := vo.MustParseId(cid)
	if err := s.MakeAppOfficial(ctx, id); err != nil {
		t.Fatal(err)
	}
	c, _ := s.repo.GetClient(ctx, id)
	if !c.IsGlobal || c.OwnerUserID != nil {
		t.Fatalf("%+v", c)
	}
	if _, err := refreshWith(s, cid, tr.RefreshToken); err != nil {
		t.Fatalf("owner's grant must keep refreshing: %v", err)
	}
	if _, err := s.ApproveAuthorization(ctx, b, session, authReq(cid)); err != nil {
		t.Fatalf("another user must be able to approve: %v", err)
	}
	if err := s.MakeAppOfficial(ctx, id); !errors.Is(err, ErrAppAlreadyOfficial) {
		t.Fatalf("err = %v", err)
	}
	if _, ok := errs.AsNotFound(s.MakeAppOfficial(ctx, vo.NewId())); !ok {
		t.Fatal("unknown id must be NotFound")
	}
}

func TestRotateAppSecret(t *testing.T) {
	s, _, _, user := newTestService(t)
	c, old, err := s.RegisterOfficialApp(ctx, "App", []string{appCallback}, false)
	if err != nil {
		t.Fatal(err)
	}
	cid := c.ID.String()
	tr := connect(t, s, user, cid, old)
	code := approveCode(t, s, user, cid)
	fresh, err := s.RotateAppSecret(ctx, c.ID)
	if err != nil || fresh == "" || fresh == old {
		t.Fatalf("%q %v", fresh, err)
	}
	_, err = refreshSecret(s, cid, old, tr.RefreshToken)
	wantOAuth(t, err, "invalid_client")
	_, err = exchangeWith(s, cid, old, code)
	wantOAuth(t, err, "invalid_client")
	if _, err := refreshSecret(s, cid, fresh, tr.RefreshToken); err != nil {
		t.Fatalf("a refresh token from before the rotation must work with the new secret: %v", err)
	}
	connect(t, s, user, cid, fresh)

	pub, _, err := s.RegisterOfficialApp(ctx, "Public", []string{appCallback}, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RotateAppSecret(ctx, pub.ID); !errors.Is(err, ErrAppPublic) {
		t.Fatalf("err = %v", err)
	}
	if got, _ := s.repo.GetClient(ctx, pub.ID); got.SecretHash != nil {
		t.Fatal("a public app must stay public")
	}
	if _, err := s.RotateAppSecret(ctx, vo.NewId()); err == nil {
		t.Fatal("unknown id")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("want NotFound, got %v", err)
	}
}

func TestDisconnectApp(t *testing.T) {
	s, creds, _, db := newTestServiceDB(t)
	a, b := newUser(t, db, fixture.User{}), newUser(t, db, fixture.User{})
	c, _, err := s.RegisterOfficialApp(ctx, "App", []string{appCallback}, true)
	if err != nil {
		t.Fatal(err)
	}
	cid := c.ID.String()
	connect(t, s, a, cid, "")
	connect(t, s, b, cid, "")
	app, _ := s.GetApp(ctx, c.ID)
	var grantA, grantB vo.Id
	for _, g := range app.Grants {
		if g.UserID.Equal(a) {
			grantA = g.ID
		} else {
			grantB = g.ID
		}
	}
	creds.locked, creds.revoked = nil, nil
	n, err := s.DisconnectApp(ctx, c.ID, &a)
	if err != nil || n != 1 {
		t.Fatalf("%d %v", n, err)
	}
	if !slices.ContainsFunc(creds.locked, a.Equal) || !slices.ContainsFunc(creds.revoked, grantA.Equal) || slices.ContainsFunc(creds.revoked, grantB.Equal) {
		t.Fatalf("locked %v revoked %v", creds.locked, creds.revoked)
	}
	if app, _ := s.GetApp(ctx, c.ID); len(app.Grants) != 1 || !app.Grants[0].ID.Equal(grantB) {
		t.Fatalf("B's grant must stay live: %+v", app.Grants)
	}
	if n, err := s.DisconnectApp(ctx, c.ID, &a); err != nil || n != 0 {
		t.Fatalf("again: %d %v", n, err)
	}
	if n, err := s.DisconnectApp(ctx, c.ID, nil); err != nil || n != 1 {
		t.Fatalf("all: %d %v", n, err)
	}
	if !slices.ContainsFunc(creds.revoked, grantB.Equal) {
		t.Fatal("B's tokens must be revoked")
	}
	if _, err := s.repo.GetClient(ctx, c.ID); err != nil {
		t.Fatalf("the app stays: %v", err)
	}
	if _, err := s.DisconnectApp(ctx, vo.NewId(), nil); err == nil {
		t.Fatal("unknown id")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("want NotFound, got %v", err)
	}
}

func TestDeleteApp(t *testing.T) {
	s, creds, clock, db := newTestServiceDB(t)
	a, b := newUser(t, db, fixture.User{}), newUser(t, db, fixture.User{})
	c, _, err := s.RegisterOfficialApp(ctx, "App", []string{appCallback}, true)
	if err != nil {
		t.Fatal(err)
	}
	cid := c.ID.String()
	connect(t, s, b, cid, "")
	connect(t, s, a, cid, "")
	app, _ := s.GetApp(ctx, c.ID)
	// Expired but never revoked: revoked with the rest, not counted as a connection.
	expired := insertGrant(t, s, a, c.ID, clock.Now().Add(-GrantIdleTTL-time.Hour), nil)
	creds.locked, creds.revoked = nil, nil
	n, err := s.DeleteApp(ctx, c.ID)
	if err != nil || n != 2 {
		t.Fatalf("%d %v", n, err)
	}
	for _, g := range app.Grants {
		if !slices.ContainsFunc(creds.revoked, g.ID.Equal) {
			t.Errorf("tokens of grant %s not revoked", g.ID)
		}
	}
	if !slices.ContainsFunc(creds.revoked, expired.Equal) {
		t.Error("the expired grant's tokens must be revoked too")
	}
	want := []vo.Id{a, b}
	slices.SortFunc(want, func(x, y vo.Id) int { return strings.Compare(x.String(), y.String()) })
	if len(creds.locked) != 2 || !creds.locked[0].Equal(want[0]) || !creds.locked[1].Equal(want[1]) {
		t.Fatalf("locked %v, want %v", creds.locked, want)
	}
	if _, err := s.repo.GetClient(ctx, c.ID); err == nil {
		t.Fatal("client still there")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Fatal(err)
	}
	var rows int
	if err := db.Raw.QueryRowContext(ctx, db.Rebind("SELECT COUNT(*) FROM oauth_grants WHERE client_id = ?"), cid).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("grant rows %d %v", rows, err)
	}
	if _, err := s.DeleteApp(ctx, c.ID); err == nil {
		t.Fatal("deleting twice")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("want NotFound, got %v", err)
	}
}

func TestDeleteApp_RefreshAndCodeFailAfterwards(t *testing.T) {
	s, _, _, user := newTestService(t)
	cid, code := approve(t, s, user)
	tr, err := exchange(s, cid, code)
	if err != nil {
		t.Fatal(err)
	}
	outstanding := approveCode(t, s, user, cid)
	if _, err := s.DeleteApp(ctx, vo.MustParseId(cid)); err != nil {
		t.Fatal(err)
	}
	_, err = refreshWith(s, cid, tr.RefreshToken)
	wantOAuth(t, err, "invalid_client")
	_, err = exchange(s, cid, outstanding)
	wantOAuth(t, err, "invalid_client")
}

// grantInjectingRepo adds grants for users DeleteApp has not locked: after the
// snapshot read (committed between the snapshot and the client lock), or on
// each read under the client lock (a stream of new connections).
type grantInjectingRepo struct {
	Repository
	afterFirst func()
	underLock  func(ctx context.Context)
	reads      int
}

func (r *grantInjectingRepo) ListClientGrants(ctx context.Context, id vo.Id) ([]model.OAuthGrant, error) {
	r.reads++
	if r.reads == 1 {
		gs, err := r.Repository.ListClientGrants(ctx, id)
		r.afterFirst()
		return gs, err
	}
	r.underLock(ctx)
	return r.Repository.ListClientGrants(ctx, id)
}

func TestDeleteApp_RetriesWhenAGrantForANewUserAppears(t *testing.T) {
	s, creds, clock, db := newTestServiceDB(t)
	a, late := newUser(t, db, fixture.User{}), newUser(t, db, fixture.User{})
	c, _, err := s.RegisterOfficialApp(ctx, "App", []string{appCallback}, true)
	if err != nil {
		t.Fatal(err)
	}
	connect(t, s, a, c.ID.String(), "")
	var lateGrant vo.Id
	repo := &grantInjectingRepo{Repository: s.repo,
		afterFirst: func() { lateGrant = insertGrant(t, s, late, c.ID, clock.Now(), nil) },
		underLock:  func(context.Context) {}}
	racing := NewService(repo, creds, s.tx, clock, nil, testURL)
	creds.locked, creds.revoked = nil, nil
	n, err := racing.DeleteApp(ctx, c.ID)
	if err != nil || n != 2 {
		t.Fatalf("%d %v", n, err)
	}
	both := []vo.Id{a, late}
	slices.SortFunc(both, func(x, y vo.Id) int { return strings.Compare(x.String(), y.String()) })
	want := append([]vo.Id{a}, both...)
	if !slices.EqualFunc(creds.locked, want, vo.Id.Equal) {
		t.Fatalf("locked %v, want %v: the retry must lock the late user too, in id order", creds.locked, want)
	}
	if !slices.ContainsFunc(creds.revoked, lateGrant.Equal) {
		t.Fatal("the late grant's tokens must be revoked")
	}
	if _, err := s.repo.GetClient(ctx, c.ID); err == nil {
		t.Fatal("client still there")
	}
}

func TestDeleteApp_GivesUpWhenNewUsersKeepConnecting(t *testing.T) {
	s, creds, clock, db := newTestServiceDB(t)
	c, _, err := s.RegisterOfficialApp(ctx, "App", []string{appCallback}, true)
	if err != nil {
		t.Fatal(err)
	}
	var fresh []vo.Id
	for range deleteAppAttempts {
		fresh = append(fresh, newUser(t, db, fixture.User{}))
	}
	next := 0
	repo := &grantInjectingRepo{Repository: s.repo, afterFirst: func() {},
		underLock: func(ctx context.Context) {
			g := &model.OAuthGrant{ID: vo.NewId(), UserID: fresh[next], ClientID: c.ID, RefreshTokenHash: hashSecret(vo.NewId().String()),
				CreatedAt: clock.Now(), LastUsedAt: clock.Now(), ExpiresAt: clock.Now().Add(GrantIdleTTL)}
			next++
			if err := s.repo.InsertGrant(ctx, g); err != nil {
				t.Fatal(err)
			}
		}}
	racing := NewService(repo, creds, s.tx, clock, nil, testURL)
	if _, err := racing.DeleteApp(ctx, c.ID); !errors.Is(err, ErrAppBusy) {
		t.Fatalf("err = %v, want ErrAppBusy", err)
	}
	if next != deleteAppAttempts {
		t.Fatalf("attempts = %d", next)
	}
	if _, err := s.repo.GetClient(ctx, c.ID); err != nil {
		t.Fatalf("the app must stay: %v", err)
	}
}

// vanishingRepo deletes the client just before the conditional UPDATE, so it
// matches no row because the app is gone rather than because of its state.
type vanishingRepo struct{ Repository }

func (r vanishingRepo) MakeClientGlobal(ctx context.Context, id vo.Id) (int64, error) {
	if _, err := r.Repository.DeleteClient(ctx, id); err != nil {
		return 0, err
	}
	return r.Repository.MakeClientGlobal(ctx, id)
}

func (r vanishingRepo) SetClientSecret(ctx context.Context, id vo.Id, hash string) (int64, error) {
	if _, err := r.Repository.DeleteClient(ctx, id); err != nil {
		return 0, err
	}
	return r.Repository.SetClientSecret(ctx, id, hash)
}

func TestMakeOfficialAndRotate_AppDeletedMeanwhile(t *testing.T) {
	s, creds, clock, _ := newTestService(t)
	racing := NewService(vanishingRepo{s.repo}, creds, s.tx, clock, nil, testURL)
	user := register(t, s, "Claude", appCallback)
	if _, ok := errs.AsNotFound(racing.MakeAppOfficial(ctx, vo.MustParseId(user.ClientID))); !ok {
		t.Fatal("make-official of a deleted app must be NotFound")
	}
	c, _, err := s.RegisterOfficialApp(ctx, "App", []string{appCallback}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := racing.RotateAppSecret(ctx, c.ID); err == nil {
		t.Fatal("rotating a deleted app must fail")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("want NotFound, got %v", err)
	}
}
