package authserver

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	authrepo "github.com/econumo/econumo/internal/authserver/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const testURL = "https://econumo.example.test"

var ctx = context.Background()

type fakeCreds struct {
	mu         sync.Mutex
	gen        int64
	issued     int
	failFence  bool
	revoked    []vo.Id
	locked     []vo.Id
	deadTokens map[vo.Id]bool
	purged     []time.Time
	scopes     map[string]model.TokenScope // issued token -> scope
}

func (f *fakeCreds) LockForOAuth(_ context.Context, u vo.Id) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.locked = append(f.locked, u)
	return f.gen, nil
}
func (f *fakeCreds) IsTokenLive(_ context.Context, _, tokenID vo.Id) (bool, error) {
	return !f.deadTokens[tokenID], nil
}
func (f *fakeCreds) PurgeDeadOAuthTokens(_ context.Context, cutoff time.Time) (int64, error) {
	f.purged = append(f.purged, cutoff)
	return 0, nil
}
func (f *fakeCreds) IssueOAuthAccessToken(_ context.Context, _, _ vo.Id, _ string, scope model.TokenScope, g int64, _ time.Duration) (string, bool, error) {
	if f.failFence || g != f.gen {
		return "", false, nil
	}
	f.issued++
	tok := fmt.Sprintf("eco_oat_%d", f.issued)
	if f.scopes == nil {
		f.scopes = map[string]model.TokenScope{}
	}
	f.scopes[tok] = scope
	return tok, true, nil
}
func (f *fakeCreds) RevokeOAuthGrantTokens(_ context.Context, id vo.Id) error {
	f.revoked = append(f.revoked, id)
	return nil
}

type fixedClock struct{ t time.Time }

func (c *fixedClock) Now() time.Time          { return c.t }
func (c *fixedClock) Advance(d time.Duration) { c.t = c.t.Add(d) }

func newTestService(t *testing.T) (*Service, *fakeCreds, *fixedClock, vo.Id) {
	t.Helper()
	s, creds, clock, db := newTestServiceDB(t)
	return s, creds, clock, newUser(t, db, fixture.User{})
}

func newTestServiceDB(t *testing.T) (*Service, *fakeCreds, *fixedClock, *dbtest.DB) {
	t.Helper()
	db := dbtest.New(t)
	creds := &fakeCreds{}
	clock := &fixedClock{t: time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)}
	s := NewService(authrepo.NewRepo(db.Engine, db.TX), creds, db.TX, clock, nil, testURL)
	return s, creds, clock, db
}

// The fixture's default email takes the id's first 8 characters, which UUIDv7
// users created in the same test share.
func newUser(t *testing.T, db *dbtest.DB, u fixture.User) vo.Id {
	t.Helper()
	if u.ID == "" {
		u.ID = vo.NewId().String()
	}
	if u.Email == "" {
		u.Email = "user-" + u.ID + "@example.test"
	}
	return vo.MustParseId(fixture.New(t, db).User(u))
}

func codeCount(t *testing.T, db *dbtest.DB, user vo.Id) int {
	t.Helper()
	var n int
	if err := db.Raw.QueryRowContext(ctx, db.Rebind("SELECT COUNT(*) FROM oauth_authorization_codes WHERE user_id = ?"), user.String()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func insertGlobalClient(t *testing.T, s *Service, createdAt time.Time, uris ...string) vo.Id {
	t.Helper()
	id := vo.NewId()
	if err := s.repo.InsertClient(ctx, &model.OAuthClient{ID: id, Name: "Global", RedirectURIs: uris, CreatedAt: createdAt, IsGlobal: true}); err != nil {
		t.Fatal(err)
	}
	return id
}

// session is the presenting token id every approval in these tests carries.
var session = vo.NewId()

func authReq(clientID string) model.AuthorizationRequest {
	return model.AuthorizationRequest{
		ClientID: clientID, RedirectURI: "https://claude.ai/api/mcp/auth_callback", ResponseType: "code",
		CodeChallenge: "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM", CodeChallengeMethod: "S256",
		Resource: testURL + "/mcp", State: "st&1",
	}
}

func hasCode(err error, code string) bool {
	v, ok := errs.AsValidation(err)
	return ok && v.MsgCode == code
}
