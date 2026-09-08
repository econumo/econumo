package oauth_test

// Fault-injection coverage for the service's error-handling branches: real
// SQLite-backed repositories wrapped so one method can be forced to fail,
// exercising paths a happy-path fixture (fixture.go / oidctest.Fake) never
// hits — a partner-lookup failure, a save failure mid-flow, a corrupt state
// row, and so on. Every case still drives the real Service through
// StartLogin/StartLink/Callback/ExchangeHandoff, so it verifies actual
// behavior, not a mock's expectations.

import (
	"context"
	"errors"
	"testing"

	"github.com/econumo/econumo/internal/infra/oidc"
	"github.com/econumo/econumo/internal/model"
	appoauth "github.com/econumo/econumo/internal/oauth"
	"github.com/econumo/econumo/internal/shared/vo"
)

var errBoom = errors.New("boom")

type faultIdentities struct {
	appoauth.Identities
	getByProviderSubject error
	getByUserProvider    error
	save                 error
	listByUser           error
	countByUser          error
	deleteByUserProvider error
}

func (f faultIdentities) GetByProviderSubject(ctx context.Context, provider, subject string) (*model.Identity, error) {
	if f.getByProviderSubject != nil {
		return nil, f.getByProviderSubject
	}
	return f.Identities.GetByProviderSubject(ctx, provider, subject)
}

func (f faultIdentities) GetByUserProvider(ctx context.Context, userID vo.Id, provider string) (*model.Identity, error) {
	if f.getByUserProvider != nil {
		return nil, f.getByUserProvider
	}
	return f.Identities.GetByUserProvider(ctx, userID, provider)
}

func (f faultIdentities) Save(ctx context.Context, i *model.Identity) error {
	if f.save != nil {
		return f.save
	}
	return f.Identities.Save(ctx, i)
}

func (f faultIdentities) ListByUser(ctx context.Context, userID vo.Id) ([]model.Identity, error) {
	if f.listByUser != nil {
		return nil, f.listByUser
	}
	return f.Identities.ListByUser(ctx, userID)
}

func (f faultIdentities) CountByUser(ctx context.Context, userID vo.Id) (int64, error) {
	if f.countByUser != nil {
		return 0, f.countByUser
	}
	return f.Identities.CountByUser(ctx, userID)
}

func (f faultIdentities) DeleteByUserProvider(ctx context.Context, userID vo.Id, provider string) (int64, error) {
	if f.deleteByUserProvider != nil {
		return 0, f.deleteByUserProvider
	}
	return f.Identities.DeleteByUserProvider(ctx, userID, provider)
}

type faultStates struct {
	appoauth.States
	delete error
}

func (f faultStates) Delete(ctx context.Context, stateHash string) error {
	if f.delete != nil {
		return f.delete
	}
	return f.States.Delete(ctx, stateHash)
}

type faultHandoffs struct {
	appoauth.Handoffs
	insert error
	get    error
	delete error
}

func (f faultHandoffs) Insert(ctx context.Context, h *model.OAuthHandoff) error {
	if f.insert != nil {
		return f.insert
	}
	return f.Handoffs.Insert(ctx, h)
}

func (f faultHandoffs) Get(ctx context.Context, codeHash string) (*model.OAuthHandoff, error) {
	if f.get != nil {
		return nil, f.get
	}
	return f.Handoffs.Get(ctx, codeHash)
}

func (f faultHandoffs) Delete(ctx context.Context, codeHash string) error {
	if f.delete != nil {
		return f.delete
	}
	return f.Handoffs.Delete(ctx, codeHash)
}

// newFaultService builds a second Service over the harness's real fake OIDC
// provider and database, letting a test swap in a wrapped dependency.
func newFaultService(h *harness, users appoauth.Users, ids appoauth.Identities, states appoauth.States, hands appoauth.Handoffs, allowRegistration bool) *appoauth.Service {
	return appoauth.NewService(h.providers, users, ids, states, hands, h.db.TX, h.clock, "https://app.example.test", allowRegistration)
}

func TestCallback_Login_IdentityOwnerLookupFails(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "old@example.test", model.AlgorithmArgon2id)
	_ = h.ids.Save(context.Background(), model.NewIdentity(vo.NewId(), u.ID, "google", h.fake.Subject, "old@example.test", h.clock.Now()))
	h.users.failFindByID = errBoom
	if r := h.login("google", "web"); r != "https://app.example.test/login?oauthError=provider_error" {
		t.Fatalf("redirect %s", r)
	}
}

func TestCallback_Login_ExistingIdentitySaveFails(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "old@example.test", model.AlgorithmArgon2id)
	_ = h.ids.Save(context.Background(), model.NewIdentity(vo.NewId(), u.ID, "google", h.fake.Subject, "old@example.test", h.clock.Now()))
	svc2 := newFaultService(h, h.users, faultIdentities{Identities: h.ids, save: errBoom}, h.states, h.hands, true)
	res, err := svc2.StartLogin(context.Background(), model.StartOAuthRequest{Provider: "google", Client: "web"})
	if err != nil {
		t.Fatal(err)
	}
	q := mustQuery(t, res.Url)
	code := h.fake.IssueCode(q.Get("nonce"), q.Get("code_challenge"))
	if r := svc2.Callback(context.Background(), "google", appoauth.CallbackInput{Code: code, State: q.Get("state")}); r != "https://app.example.test/login?oauthError=provider_error" {
		t.Fatalf("redirect %s", r)
	}
}

func TestCallback_Login_IdentityLookupFails(t *testing.T) {
	h := newHarness(t, false, true)
	svc2 := newFaultService(h, h.users, faultIdentities{Identities: h.ids, getByProviderSubject: errBoom}, h.states, h.hands, true)
	res, err := svc2.StartLogin(context.Background(), model.StartOAuthRequest{Provider: "google", Client: "web"})
	if err != nil {
		t.Fatal(err)
	}
	q := mustQuery(t, res.Url)
	code := h.fake.IssueCode(q.Get("nonce"), q.Get("code_challenge"))
	if r := svc2.Callback(context.Background(), "google", appoauth.CallbackInput{Code: code, State: q.Get("state")}); r != "https://app.example.test/login?oauthError=provider_error" {
		t.Fatalf("redirect %s", r)
	}
}

func TestCallback_Login_AutoLinkInactiveUser(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "match@example.test", model.AlgorithmArgon2id)
	u.IsActive = false
	h.fake.Email, h.fake.EmailVerified = "match@example.test", true
	if r := h.login("google", "web"); r != "https://app.example.test/login?oauthError=account_inactive" {
		t.Fatalf("redirect %s", r)
	}
}

func TestCallback_Login_AutoLinkSaveFails(t *testing.T) {
	h := newHarness(t, false, true)
	h.users.seed(t, "match@example.test", model.AlgorithmArgon2id)
	h.fake.Email, h.fake.EmailVerified = "match@example.test", true
	svc2 := newFaultService(h, h.users, faultIdentities{Identities: h.ids, save: errBoom}, h.states, h.hands, true)
	res, err := svc2.StartLogin(context.Background(), model.StartOAuthRequest{Provider: "google", Client: "web"})
	if err != nil {
		t.Fatal(err)
	}
	q := mustQuery(t, res.Url)
	code := h.fake.IssueCode(q.Get("nonce"), q.Get("code_challenge"))
	if r := svc2.Callback(context.Background(), "google", appoauth.CallbackInput{Code: code, State: q.Get("state")}); r != "https://app.example.test/login?oauthError=provider_error" {
		t.Fatalf("redirect %s", r)
	}
}

func TestCallback_Login_UserLookupFails(t *testing.T) {
	h := newHarness(t, false, true)
	h.users.failFindByEmail = errBoom
	if r := h.login("google", "web"); r != "https://app.example.test/login?oauthError=provider_error" {
		t.Fatalf("redirect %s", r)
	}
}

func TestCallback_Login_ProvisionFails(t *testing.T) {
	h := newHarness(t, false, true)
	h.users.failProvisionExternal = errBoom
	if r := h.login("google", "web"); r != "https://app.example.test/login?oauthError=provider_error" {
		t.Fatalf("redirect %s", r)
	}
}

func TestCallback_Login_ProvisionSaveFails(t *testing.T) {
	h := newHarness(t, false, true)
	svc2 := newFaultService(h, h.users, faultIdentities{Identities: h.ids, save: errBoom}, h.states, h.hands, true)
	res, err := svc2.StartLogin(context.Background(), model.StartOAuthRequest{Provider: "google", Client: "web"})
	if err != nil {
		t.Fatal(err)
	}
	q := mustQuery(t, res.Url)
	code := h.fake.IssueCode(q.Get("nonce"), q.Get("code_challenge"))
	if r := svc2.Callback(context.Background(), "google", appoauth.CallbackInput{Code: code, State: q.Get("state")}); r != "https://app.example.test/login?oauthError=provider_error" {
		t.Fatalf("redirect %s", r)
	}
}

func TestCallback_Login_HandoffInsertFails(t *testing.T) {
	h := newHarness(t, false, true)
	svc2 := newFaultService(h, h.users, h.ids, h.states, faultHandoffs{Handoffs: h.hands, insert: errBoom}, true)
	res, err := svc2.StartLogin(context.Background(), model.StartOAuthRequest{Provider: "google", Client: "web"})
	if err != nil {
		t.Fatal(err)
	}
	q := mustQuery(t, res.Url)
	code := h.fake.IssueCode(q.Get("nonce"), q.Get("code_challenge"))
	if r := svc2.Callback(context.Background(), "google", appoauth.CallbackInput{Code: code, State: q.Get("state")}); r != "https://app.example.test/login?oauthError=provider_error" {
		t.Fatalf("redirect %s", r)
	}
}

func TestCallback_Link_ExistingIdentityUpdateSaveFails(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "me@example.test", model.AlgorithmArgon2id)
	_ = h.ids.Save(context.Background(), model.NewIdentity(vo.NewId(), u.ID, "google", h.fake.Subject, "me@example.test", h.clock.Now()))
	svc2 := newFaultService(h, h.users, faultIdentities{Identities: h.ids, save: errBoom}, h.states, h.hands, true)
	res, err := svc2.StartLink(context.Background(), u.ID, model.StartOAuthRequest{Provider: "google", Client: "web"})
	if err != nil {
		t.Fatal(err)
	}
	q := mustQuery(t, res.Url)
	code := h.fake.IssueCode(q.Get("nonce"), q.Get("code_challenge"))
	if r := svc2.Callback(context.Background(), "google", appoauth.CallbackInput{Code: code, State: q.Get("state")}); r != "https://app.example.test/login?oauthError=provider_error" {
		t.Fatalf("redirect %s", r)
	}
}

func TestCallback_Link_IdentityLookupFails(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "me@example.test", model.AlgorithmArgon2id)
	svc2 := newFaultService(h, h.users, faultIdentities{Identities: h.ids, getByProviderSubject: errBoom}, h.states, h.hands, true)
	res, err := svc2.StartLink(context.Background(), u.ID, model.StartOAuthRequest{Provider: "google", Client: "web"})
	if err != nil {
		t.Fatal(err)
	}
	q := mustQuery(t, res.Url)
	code := h.fake.IssueCode(q.Get("nonce"), q.Get("code_challenge"))
	if r := svc2.Callback(context.Background(), "google", appoauth.CallbackInput{Code: code, State: q.Get("state")}); r != "https://app.example.test/login?oauthError=provider_error" {
		t.Fatalf("redirect %s", r)
	}
}

func TestCallback_Link_InsertSaveFails(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "me@example.test", model.AlgorithmArgon2id)
	svc2 := newFaultService(h, h.users, faultIdentities{Identities: h.ids, save: errBoom}, h.states, h.hands, true)
	res, err := svc2.StartLink(context.Background(), u.ID, model.StartOAuthRequest{Provider: "google", Client: "web"})
	if err != nil {
		t.Fatal(err)
	}
	q := mustQuery(t, res.Url)
	code := h.fake.IssueCode(q.Get("nonce"), q.Get("code_challenge"))
	if r := svc2.Callback(context.Background(), "google", appoauth.CallbackInput{Code: code, State: q.Get("state")}); r != "https://app.example.test/login?oauthError=provider_error" {
		t.Fatalf("redirect %s", r)
	}
}

func TestCallback_ConsumeState_DeleteFails(t *testing.T) {
	h := newHarness(t, false, true)
	svc2 := newFaultService(h, h.users, h.ids, faultStates{States: h.states, delete: errBoom}, h.hands, true)
	res, err := svc2.StartLogin(context.Background(), model.StartOAuthRequest{Provider: "google", Client: "web"})
	if err != nil {
		t.Fatal(err)
	}
	q := mustQuery(t, res.Url)
	if r := svc2.Callback(context.Background(), "google", appoauth.CallbackInput{Code: "x", State: q.Get("state")}); r != "https://app.example.test/login?oauthError=invalid_state" {
		t.Fatalf("redirect %s", r)
	}
}

func TestExchangeHandoff_GetFails(t *testing.T) {
	h := newHarness(t, false, true)
	svc2 := newFaultService(h, h.users, h.ids, h.states, faultHandoffs{Handoffs: h.hands, get: errBoom}, true)
	if _, err := svc2.ExchangeHandoff(context.Background(), model.ExchangeHandoffRequest{Code: "x"}, "ua"); !errors.Is(err, errBoom) {
		t.Fatalf("want errBoom, got %v", err)
	}
}

func TestExchangeHandoff_DeleteFails(t *testing.T) {
	h := newHarness(t, false, true)
	redirect := h.login("google", "web")
	svc2 := newFaultService(h, h.users, h.ids, h.states, faultHandoffs{Handoffs: h.hands, delete: errBoom}, true)
	if _, err := svc2.ExchangeHandoff(context.Background(), model.ExchangeHandoffRequest{Code: handoffOf(t, redirect)}, "ua"); !errors.Is(err, errBoom) {
		t.Fatalf("want errBoom, got %v", err)
	}
}

func TestExchangeHandoff_MintSessionFails(t *testing.T) {
	h := newHarness(t, false, true)
	redirect := h.login("google", "web")
	h.users.failMintSession = errBoom
	if _, err := h.svc.ExchangeHandoff(context.Background(), model.ExchangeHandoffRequest{Code: handoffOf(t, redirect)}, "ua"); !errors.Is(err, errBoom) {
		t.Fatalf("want errBoom, got %v", err)
	}
}

func TestListIdentities_Fails(t *testing.T) {
	h := newHarness(t, false, true)
	svc2 := newFaultService(h, h.users, faultIdentities{Identities: h.ids, listByUser: errBoom}, h.states, h.hands, true)
	if _, err := svc2.ListIdentities(context.Background(), vo.NewId()); !errors.Is(err, errBoom) {
		t.Fatalf("want errBoom, got %v", err)
	}
}

func TestUnlinkIdentity_LookupFails(t *testing.T) {
	h := newHarness(t, false, true)
	svc2 := newFaultService(h, h.users, faultIdentities{Identities: h.ids, getByUserProvider: errBoom}, h.states, h.hands, true)
	if _, err := svc2.UnlinkIdentity(context.Background(), vo.NewId(), model.UnlinkIdentityRequest{Provider: "google"}); !errors.Is(err, errBoom) {
		t.Fatalf("want errBoom, got %v", err)
	}
}

func TestUnlinkIdentity_UserLookupFails(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "me@example.test", model.AlgorithmNone)
	_ = h.ids.Save(context.Background(), model.NewIdentity(vo.NewId(), u.ID, "google", "g1", "me@example.test", h.clock.Now()))
	h.users.failFindByID = errBoom
	if _, err := h.svc.UnlinkIdentity(context.Background(), u.ID, model.UnlinkIdentityRequest{Provider: "google"}); !errors.Is(err, errBoom) {
		t.Fatalf("want errBoom, got %v", err)
	}
}

func TestUnlinkIdentity_CountFails(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "me@example.test", model.AlgorithmNone)
	_ = h.ids.Save(context.Background(), model.NewIdentity(vo.NewId(), u.ID, "google", "g1", "me@example.test", h.clock.Now()))
	svc2 := newFaultService(h, h.users, faultIdentities{Identities: h.ids, countByUser: errBoom}, h.states, h.hands, true)
	if _, err := svc2.UnlinkIdentity(context.Background(), u.ID, model.UnlinkIdentityRequest{Provider: "google"}); !errors.Is(err, errBoom) {
		t.Fatalf("want errBoom, got %v", err)
	}
}

func TestUnlinkIdentity_DeleteFails(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "me@example.test", model.AlgorithmArgon2id)
	_ = h.ids.Save(context.Background(), model.NewIdentity(vo.NewId(), u.ID, "google", "g1", "me@example.test", h.clock.Now()))
	svc2 := newFaultService(h, h.users, faultIdentities{Identities: h.ids, deleteByUserProvider: errBoom}, h.states, h.hands, true)
	if _, err := svc2.UnlinkIdentity(context.Background(), u.ID, model.UnlinkIdentityRequest{Provider: "google"}); !errors.Is(err, errBoom) {
		t.Fatalf("want errBoom, got %v", err)
	}
}

func TestStartLogin_DiscoveryFails(t *testing.T) {
	h := newHarness(t, false, true)
	// An issuer nothing listens on: Discover (and so AuthURL) fails immediately.
	unreachable := []appoauth.Provider{{Name: "Google", Client: oidc.NewClient(oidc.Issuer{
		ID: "google", IssuerURL: "http://127.0.0.1:1", ClientID: "x", ClientSecret: oidc.StaticSecret("s"),
		Scopes: []string{"openid"}, UsePKCE: true, TrustEmail: true,
	}, nil)}}
	svc2 := appoauth.NewService(unreachable, h.users, h.ids, h.states, h.hands, h.db.TX, h.clock, "https://app.example.test", true)
	if _, err := svc2.StartLogin(context.Background(), model.StartOAuthRequest{Provider: "google", Client: "web"}); err == nil {
		t.Fatal("unreachable issuer must fail discovery")
	}
}
