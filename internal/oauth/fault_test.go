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
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/infra/oidc"
	"github.com/econumo/econumo/internal/model"
	appoauth "github.com/econumo/econumo/internal/oauth"
	"github.com/econumo/econumo/internal/shared/errs"
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

func (f faultIdentities) GetByProviderSubject(ctx context.Context, provider, issuer, subject string) (*model.Identity, error) {
	if f.getByProviderSubject != nil {
		return nil, f.getByProviderSubject
	}
	return f.Identities.GetByProviderSubject(ctx, provider, issuer, subject)
}

func (f faultIdentities) GetByUserProvider(ctx context.Context, userID vo.Id, provider string) (*model.Identity, error) {
	if f.getByUserProvider != nil {
		return nil, f.getByUserProvider
	}
	return f.Identities.GetByUserProvider(ctx, userID, provider)
}

func (f faultIdentities) InsertIfCurrent(ctx context.Context, i *model.Identity, generation int64) (int64, error) {
	if f.save != nil {
		return 0, f.save
	}
	return f.Identities.InsertIfCurrent(ctx, i, generation)
}

func (f faultIdentities) UpdateIfCurrent(ctx context.Context, i *model.Identity, generation int64) (int64, error) {
	if f.save != nil {
		return 0, f.save
	}
	return f.Identities.UpdateIfCurrent(ctx, i, generation)
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
	// raced simulates a concurrent presenter who deleted the row first: the
	// delete succeeds but reports zero rows affected.
	raced bool
}

func (f faultStates) Delete(ctx context.Context, stateHash string) (int64, error) {
	if f.delete != nil {
		return 0, f.delete
	}
	if f.raced {
		return 0, nil
	}
	return f.States.Delete(ctx, stateHash)
}

type faultHandoffs struct {
	appoauth.Handoffs
	insert error
	get    error
	delete error
	// raced simulates a concurrent presenter who deleted the row first: the
	// delete succeeds but reports zero rows affected.
	raced bool
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

func (f faultHandoffs) Delete(ctx context.Context, codeHash string) (int64, error) {
	if f.delete != nil {
		return 0, f.delete
	}
	if f.raced {
		return 0, nil
	}
	return f.Handoffs.Delete(ctx, codeHash)
}

// newFaultService builds a second Service over the harness's real fake OIDC
// provider and database, letting a test swap in a wrapped dependency.
func newFaultService(h *harness, users appoauth.Users, ids appoauth.Identities, states appoauth.States, hands appoauth.Handoffs, allowRegistration bool) *appoauth.Service {
	return appoauth.NewService(h.providers, users, ids, states, hands, h.db.TX, h.clock, nil, "https://app.example.test", allowRegistration)
}

func TestCallback_Login_IdentityOwnerLookupFails(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "old@example.test", model.AlgorithmArgon2id)
	saveIdentity(t, h, model.NewIdentity(vo.NewId(), u.ID, "google", h.fake.IssuerURL(), h.fake.Subject, "old@example.test", h.clock.Now()))
	h.users.failFindByID = errBoom
	if r := h.login("google", "web"); r != "https://app.example.test/login?oauthError=provider_error" {
		t.Fatalf("redirect %s", r)
	}
}

func TestCallback_Login_ExistingIdentitySaveFails(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "old@example.test", model.AlgorithmArgon2id)
	saveIdentity(t, h, model.NewIdentity(vo.NewId(), u.ID, "google", h.fake.IssuerURL(), h.fake.Subject, "old@example.test", h.clock.Now()))
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
	u := h.users.seed(t, "match@example.test", model.AlgorithmNone)
	u.IsActive = false
	h.fake.Email, h.fake.EmailVerified = "match@example.test", true
	if r := h.login("google", "web"); r != "https://app.example.test/login?oauthError=account_inactive" {
		t.Fatalf("redirect %s", r)
	}
}

func TestCallback_Login_AutoLinkSaveFails(t *testing.T) {
	h := newHarness(t, false, true)
	h.users.seed(t, "match@example.test", model.AlgorithmNone)
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

// The identity write moved to CompleteLink, so a failing Save surfaces there —
// the callback itself only parks the resolved identity.
func TestCompleteLink_ExistingIdentityUpdateSaveFails(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "me@example.test", model.AlgorithmArgon2id)
	saveIdentity(t, h, model.NewIdentity(vo.NewId(), u.ID, "google", h.fake.IssuerURL(), h.fake.Subject, "me@example.test", h.clock.Now()))
	svc2 := newFaultService(h, h.users, faultIdentities{Identities: h.ids, save: errBoom}, h.states, h.hands, true)
	if err := completeLinkVia(t, h, svc2, u.ID); !errors.Is(err, errBoom) {
		t.Fatalf("want errBoom, got %v", err)
	}
}

// completeLinkVia drives start-link + consent + callback + complete-link
// through svc2 and returns complete-link's error.
func completeLinkVia(t *testing.T, h *harness, svc2 *appoauth.Service, userID vo.Id) error {
	t.Helper()
	res, err := svc2.StartLink(context.Background(), userID, model.StartOAuthRequest{Provider: "google", Client: "web"})
	if err != nil {
		t.Fatal(err)
	}
	q := mustQuery(t, res.Url)
	code := h.fake.IssueCode(q.Get("nonce"), q.Get("code_challenge"))
	redirect := svc2.Callback(context.Background(), "google", appoauth.CallbackInput{Code: code, State: q.Get("state")})
	handoff := linkHandoffOf(t, redirect)
	if handoff == "" {
		t.Fatalf("callback did not park a link handoff: %s", redirect)
	}
	_, cerr := svc2.CompleteLink(context.Background(), userID, model.CompleteLinkRequest{Code: handoff, Flow: res.Flow})
	return cerr
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
	if r := svc2.Callback(context.Background(), "google", appoauth.CallbackInput{Code: code, State: q.Get("state")}); r != "https://app.example.test/settings/profile/linked-accounts?oauthError=provider_error" {
		t.Fatalf("redirect %s", r)
	}
}

func TestCompleteLink_InsertSaveFails(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "me@example.test", model.AlgorithmArgon2id)
	svc2 := newFaultService(h, h.users, faultIdentities{Identities: h.ids, save: errBoom}, h.states, h.hands, true)
	if err := completeLinkVia(t, h, svc2, u.ID); !errors.Is(err, errBoom) {
		t.Fatalf("want errBoom, got %v", err)
	}
}

func TestCallback_Link_OwnerLookupFails(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "me@example.test", model.AlgorithmArgon2id)
	// The link callback reads the owner to capture the fence for the deferred
	// write; without that value the handoff must not be minted at all.
	h.users.failFindByID = errBoom
	if r := h.startLink(u.ID, "google", "web"); !strings.HasSuffix(r, "?oauthError=provider_error") {
		t.Fatalf("redirect %s", r)
	}
}

func TestCompleteLink_UserProviderLookupFails(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "me@example.test", model.AlgorithmArgon2id)
	res, err := h.svc.StartLink(context.Background(), u.ID, model.StartOAuthRequest{Provider: "google", Client: "web"})
	if err != nil {
		t.Fatal(err)
	}
	q := mustQuery(t, res.Url)
	redirect := h.svc.Callback(context.Background(), "google",
		appoauth.CallbackInput{Code: h.fake.IssueCode(q.Get("nonce"), q.Get("code_challenge")), State: q.Get("state")})
	// The eager check ran on the callback; the completion re-runs it, and that
	// second lookup is the one forced to fail here.
	svc2 := newFaultService(h, h.users, faultIdentities{Identities: h.ids, getByUserProvider: errBoom}, h.states, h.hands, true)
	if _, err := svc2.CompleteLink(context.Background(), u.ID,
		model.CompleteLinkRequest{Code: linkHandoffOf(t, redirect), Flow: res.Flow}); !errors.Is(err, errBoom) {
		t.Fatalf("want errBoom, got %v", err)
	}
}

func TestCallback_Link_HandoffInsertFails(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "me@example.test", model.AlgorithmArgon2id)
	svc2 := newFaultService(h, h.users, h.ids, h.states, faultHandoffs{Handoffs: h.hands, insert: errBoom}, true)
	res, err := svc2.StartLink(context.Background(), u.ID, model.StartOAuthRequest{Provider: "google", Client: "web"})
	if err != nil {
		t.Fatal(err)
	}
	q := mustQuery(t, res.Url)
	code := h.fake.IssueCode(q.Get("nonce"), q.Get("code_challenge"))
	if r := svc2.Callback(context.Background(), "google", appoauth.CallbackInput{Code: code, State: q.Get("state")}); r != "https://app.example.test/settings/profile/linked-accounts?oauthError=provider_error" {
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

// TestCallback_ConsumeState_DeleteRaced simulates a concurrent replay of the
// same callback: the delete succeeds but reports zero rows because another
// presenter's delete already removed the row. That must be rejected exactly
// like an unknown state, not treated as success.
func TestCallback_ConsumeState_DeleteRaced(t *testing.T) {
	h := newHarness(t, false, true)
	svc2 := newFaultService(h, h.users, h.ids, faultStates{States: h.states, raced: true}, h.hands, true)
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
	if _, err := svc2.ExchangeHandoff(context.Background(), model.ExchangeHandoffRequest{Code: "x", Flow: "x"}, "ua"); !errors.Is(err, errBoom) {
		t.Fatalf("want errBoom, got %v", err)
	}
}

func TestExchangeHandoff_DeleteFails(t *testing.T) {
	h := newHarness(t, false, true)
	redirect := h.login("google", "web")
	svc2 := newFaultService(h, h.users, h.ids, h.states, faultHandoffs{Handoffs: h.hands, delete: errBoom}, true)
	if _, err := svc2.ExchangeHandoff(context.Background(), h.exchangeReq(t, redirect), "ua"); !errors.Is(err, errBoom) {
		t.Fatalf("want errBoom, got %v", err)
	}
}

// TestExchangeHandoff_DeleteRaced simulates a concurrent exchange of the same
// code: the delete succeeds but reports zero rows because another presenter's
// delete already removed the row. That presenter must be rejected and must
// never mint a session.
func TestExchangeHandoff_DeleteRaced(t *testing.T) {
	h := newHarness(t, false, true)
	redirect := h.login("google", "web")
	svc2 := newFaultService(h, h.users, h.ids, h.states, faultHandoffs{Handoffs: h.hands, raced: true}, true)
	_, err := svc2.ExchangeHandoff(context.Background(), h.exchangeReq(t, redirect), "ua")
	if u, ok := errs.AsUnauthorized(err); !ok || u.Code != errs.CodeOAuthHandoffInvalid {
		t.Fatalf("want 401 handoff_invalid, got %v", err)
	}
	if len(h.users.minted) != 0 {
		t.Fatalf("a raced delete must not mint a session: %v", h.users.minted)
	}
}

func TestExchangeHandoff_MintSessionFails(t *testing.T) {
	h := newHarness(t, false, true)
	redirect := h.login("google", "web")
	h.users.failMintSession = errBoom
	if _, err := h.svc.ExchangeHandoff(context.Background(), h.exchangeReq(t, redirect), "ua"); !errors.Is(err, errBoom) {
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
	saveIdentity(t, h, model.NewIdentity(vo.NewId(), u.ID, "google", h.fake.IssuerURL(), "g1", "me@example.test", h.clock.Now()))
	h.users.failFindByID = errBoom
	if _, err := h.svc.UnlinkIdentity(context.Background(), u.ID, model.UnlinkIdentityRequest{Provider: "google"}); !errors.Is(err, errBoom) {
		t.Fatalf("want errBoom, got %v", err)
	}
}

func TestUnlinkIdentity_CountFails(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "me@example.test", model.AlgorithmNone)
	saveIdentity(t, h, model.NewIdentity(vo.NewId(), u.ID, "google", h.fake.IssuerURL(), "g1", "me@example.test", h.clock.Now()))
	svc2 := newFaultService(h, h.users, faultIdentities{Identities: h.ids, countByUser: errBoom}, h.states, h.hands, true)
	if _, err := svc2.UnlinkIdentity(context.Background(), u.ID, model.UnlinkIdentityRequest{Provider: "google"}); !errors.Is(err, errBoom) {
		t.Fatalf("want errBoom, got %v", err)
	}
}

func TestUnlinkIdentity_DeleteFails(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "me@example.test", model.AlgorithmArgon2id)
	saveIdentity(t, h, model.NewIdentity(vo.NewId(), u.ID, "google", h.fake.IssuerURL(), "g1", "me@example.test", h.clock.Now()))
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
	svc2 := appoauth.NewService(unreachable, h.users, h.ids, h.states, h.hands, h.db.TX, h.clock, nil, "https://app.example.test", true)
	if _, err := svc2.StartLogin(context.Background(), model.StartOAuthRequest{Provider: "google", Client: "web"}); err == nil {
		t.Fatal("unreachable issuer must fail discovery")
	}
}
