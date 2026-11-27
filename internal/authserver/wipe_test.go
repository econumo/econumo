package authserver

import (
	"context"
	"database/sql"
	"testing"
	"time"

	authrepo "github.com/econumo/econumo/internal/authserver/repo"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	appuser "github.com/econumo/econumo/internal/user"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

// newWipeService wires the real user service as Credentials, so revoking a
// grant's tokens writes access_tokens.revoked_at.
func newWipeService(t *testing.T, wrap func(Repository) Repository) (*Service, *dbtest.DB) {
	t.Helper()
	db := dbtest.New(t)
	clock := &fixedClock{t: time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)}
	creds := appuser.NewService(userrepo.NewRepo(db.Engine, db.TX), db.TX, nil, nil,
		userrepo.NewAccessTokenRepo(db.Engine, db.TX), nil, nil, nil, nil, nil, nil, nil, nil, nil,
		clock, nil, false, 0, false)
	var repo Repository = authrepo.NewRepo(db.Engine, db.TX)
	if wrap != nil {
		repo = wrap(repo)
	}
	return NewService(repo, creds, db.TX, clock, nil, testURL), db
}

type wipeWorld struct {
	f  *fixture.Builder
	db *dbtest.DB
}

func (w *wipeWorld) app(owner string, global bool) string {
	return w.f.OAuthClient(fixture.OAuthClient{Name: "App", RedirectURIs: []string{appCallback}, OwnerUserID: owner, IsGlobal: global})
}

func (w *wipeWorld) grant(t *testing.T, user, client string) string {
	t.Helper()
	return w.f.OAuthGrant(fixture.OAuthGrant{UserID: user, ClientID: client, RefreshHash: "refresh-" + vo.NewId().String()})
}

func (w *wipeWorld) token(t *testing.T, user, grant string) string {
	t.Helper()
	id := w.f.AccessToken(fixture.AccessToken{UserID: user, Kind: "oauth", Scope: "mcp", TokenHash: "oat-" + vo.NewId().String()})
	if _, err := w.db.Raw.ExecContext(ctx, w.db.Rebind("UPDATE access_tokens SET grant_id = ? WHERE id = ?"), grant, id); err != nil {
		t.Fatal(err)
	}
	return id
}

func tokenRevoked(t *testing.T, db *dbtest.DB, id string) bool {
	t.Helper()
	var at sql.NullTime
	if err := db.Raw.QueryRowContext(ctx, db.Rebind("SELECT revoked_at FROM access_tokens WHERE id = ?"), id).Scan(&at); err != nil {
		t.Fatal(err)
	}
	return at.Valid
}

func grantRevoked(t *testing.T, db *dbtest.DB, id string) bool {
	t.Helper()
	var at sql.NullTime
	if err := db.Raw.QueryRowContext(ctx, db.Rebind("SELECT revoked_at FROM oauth_grants WHERE id = ?"), id).Scan(&at); err != nil {
		t.Fatal(err)
	}
	return at.Valid
}

func clientExists(t *testing.T, s *Service, id string) bool {
	t.Helper()
	_, err := s.repo.GetClient(ctx, vo.MustParseId(id))
	if err == nil {
		return true
	}
	if _, ok := errs.AsNotFound(err); !ok {
		t.Fatal(err)
	}
	return false
}

func TestDeleteAppsOwnedBy(t *testing.T) {
	s, db := newWipeService(t, nil)
	w := &wipeWorld{f: fixture.New(t, db), db: db}
	gone := newUser(t, db, fixture.User{Email: "gone@example.test"}).String()
	other := newUser(t, db, fixture.User{Email: "other@example.test"}).String()

	owned := w.app(gone, false)
	w.grant(t, gone, owned)
	otherOnOwned := w.grant(t, other, owned)
	otherTokenOnOwned := w.token(t, other, otherOnOwned)

	global := w.app("", true)
	otherOnGlobal := w.grant(t, other, global)
	otherTokenOnGlobal := w.token(t, other, otherOnGlobal)

	othersApp := w.app(other, false)
	otherOnOwn := w.grant(t, other, othersApp)
	otherTokenOnOwn := w.token(t, other, otherOnOwn)

	if err := s.DeleteAppsOwnedBy(ctx, vo.MustParseId(gone)); err != nil {
		t.Fatalf("DeleteAppsOwnedBy: %v", err)
	}

	if clientExists(t, s, owned) {
		t.Error("the deleted user's app is still there")
	}
	if !tokenRevoked(t, db, otherTokenOnOwned) {
		t.Error("another user's token on the deleted app is still live")
	}
	for _, app := range []string{global, othersApp} {
		if !clientExists(t, s, app) {
			t.Errorf("app %s was deleted", app)
		}
	}
	for _, g := range []string{otherOnGlobal, otherOnOwn} {
		if grantRevoked(t, db, g) {
			t.Errorf("grant %s on an untouched app was revoked", g)
		}
	}
	for _, tk := range []string{otherTokenOnGlobal, otherTokenOnOwn} {
		if tokenRevoked(t, db, tk) {
			t.Errorf("token %s on an untouched app was revoked", tk)
		}
	}
}

// globalizingRepo makes one listed app global right after the owner's apps are
// listed, as an operator's app:make-global landing before the delete's lock.
type globalizingRepo struct {
	Repository
	target vo.Id
}

func (r globalizingRepo) ListClientIDsByOwner(ctx context.Context, userID vo.Id) ([]vo.Id, error) {
	ids, err := r.Repository.ListClientIDsByOwner(ctx, userID)
	if err != nil {
		return nil, err
	}
	if _, err := r.Repository.MakeClientGlobal(ctx, r.target); err != nil {
		return nil, err
	}
	return ids, nil
}

func TestDeleteAppsOwnedBy_SkipsAnAppMadeGlobalMeanwhile(t *testing.T) {
	target := vo.NewId()
	s, db := newWipeService(t, func(r Repository) Repository { return globalizingRepo{Repository: r, target: target} })
	w := &wipeWorld{f: fixture.New(t, db), db: db}
	gone := newUser(t, db, fixture.User{Email: "gone@example.test"}).String()
	other := newUser(t, db, fixture.User{Email: "other@example.test"}).String()

	w.f.OAuthClient(fixture.OAuthClient{ID: target.String(), Name: "App", RedirectURIs: []string{appCallback}, OwnerUserID: gone})
	goneGrant := w.grant(t, gone, target.String())
	otherGrant := w.grant(t, other, target.String())
	otherToken := w.token(t, other, otherGrant)
	stillOwned := w.app(gone, false)

	if err := s.DeleteAppsOwnedBy(ctx, vo.MustParseId(gone)); err != nil {
		t.Fatalf("DeleteAppsOwnedBy: %v", err)
	}

	c, err := s.repo.GetClient(ctx, target)
	if err != nil {
		t.Fatalf("the app made global was deleted: %v", err)
	}
	if !c.IsGlobal || c.OwnerUserID != nil {
		t.Fatalf("app = %+v, want global with no owner", c)
	}
	for _, g := range []string{goneGrant, otherGrant} {
		if grantRevoked(t, db, g) {
			t.Errorf("grant %s on the app made global was revoked", g)
		}
	}
	if tokenRevoked(t, db, otherToken) {
		t.Error("a token on the app made global was revoked")
	}
	if clientExists(t, s, stillOwned) {
		t.Error("the user's other app must still be deleted")
	}
}
