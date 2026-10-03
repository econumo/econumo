package authserver

import (
	"context"
	"fmt"
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
	gen       int64
	issued    int
	failFence bool
	revoked   []vo.Id
	locked    []vo.Id
}

func (f *fakeCreds) LockForOAuth(_ context.Context, u vo.Id) (int64, error) {
	f.locked = append(f.locked, u)
	return f.gen, nil
}
func (f *fakeCreds) CredentialsGeneration(context.Context, vo.Id) (int64, error) { return f.gen, nil }
func (f *fakeCreds) IssueOAuthAccessToken(_ context.Context, _, _ vo.Id, _ string, g int64, _ time.Duration) (string, bool, error) {
	if f.failFence || g != f.gen {
		return "", false, nil
	}
	f.issued++
	return fmt.Sprintf("eco_oat_%d", f.issued), true, nil
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
	db := dbtest.New(t)
	userID := vo.MustParseId(fixture.New(t, db).User(fixture.User{}))
	creds := &fakeCreds{}
	clock := &fixedClock{t: time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)}
	s := NewService(authrepo.NewRepo(db.Engine, db.TX), creds, db.TX, clock, nil, testURL)
	return s, creds, clock, userID
}

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
