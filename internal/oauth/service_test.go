package oauth_test

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/oidc"
	"github.com/econumo/econumo/internal/infra/oidc/oidctest"
	"github.com/econumo/econumo/internal/model"
	appoauth "github.com/econumo/econumo/internal/oauth"
	oauthrepo "github.com/econumo/econumo/internal/oauth/repo"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// fixedClock is a pointer so a test can advance it after the service is built.
type fixedClock struct{ t time.Time }

func (c *fixedClock) Now() time.Time { return c.t }

// fakeUsers is the Users port over the seeded users table: rows are real (so
// the FK constraints on identities/handoffs hold) but the aggregate is kept in
// memory, which is all the oauth service reads.
type fakeUsers struct {
	t         *testing.T
	db        *dbtest.DB
	byID      map[string]*model.User
	byEmail   map[string]*model.User
	minted    []string // providers of minted sessions
	replaced  []string // emails mirrored via ReplaceVerifiedEmail
	revoked   []string // user ids whose sessions were revoked on auto-link
	verified  []string // user ids marked email-verified on auto-link
	provision int

	// Fault injection for coverage of the service's error-handling branches:
	// non-nil forces the corresponding method to fail regardless of state.
	failFindByID          error
	failFindByEmail       error
	failProvisionExternal error
	failMintSession       error
	failRevokeAllSessions error
}

func newFakeUsers(t *testing.T, db *dbtest.DB) *fakeUsers {
	return &fakeUsers{t: t, db: db, byID: map[string]*model.User{}, byEmail: map[string]*model.User{}}
}

func (f *fakeUsers) seed(t *testing.T, email string, algorithm string) *model.User {
	id := fixture.New(t, f.db).User(fixture.User{Email: email, Algorithm: algorithm})
	u := &model.User{ID: vo.MustParseId(id), Email: email, Name: "Seed", IsActive: true, Algorithm: algorithm, EmailVerified: true}
	f.byID[id] = u
	f.byEmail[strings.ToLower(email)] = u
	return u
}

func (f *fakeUsers) FindByEmail(_ context.Context, email string) (*model.User, error) {
	if f.failFindByEmail != nil {
		return nil, f.failFindByEmail
	}
	if u, ok := f.byEmail[strings.ToLower(email)]; ok {
		return u, nil
	}
	return nil, errs.NewNotFound("User not found")
}
func (f *fakeUsers) FindByID(_ context.Context, id vo.Id) (*model.User, error) {
	if f.failFindByID != nil {
		return nil, f.failFindByID
	}
	if u, ok := f.byID[id.String()]; ok {
		return u, nil
	}
	return nil, errs.NewNotFound("User not found")
}
func (f *fakeUsers) ProvisionExternal(_ context.Context, name, email string) (*model.User, error) {
	if f.failProvisionExternal != nil {
		return nil, f.failProvisionExternal
	}
	f.provision++
	u := f.seed(f.t, email, model.AlgorithmNone)
	u.Name = name
	return u, nil
}
func (f *fakeUsers) ReplaceVerifiedEmail(_ context.Context, userID vo.Id, email string) error {
	f.replaced = append(f.replaced, email)
	return nil
}
func (f *fakeUsers) RevokeAllSessions(_ context.Context, userID vo.Id) error {
	f.revoked = append(f.revoked, userID.String())
	return f.failRevokeAllSessions
}
func (f *fakeUsers) MarkEmailVerified(_ context.Context, userID vo.Id) error {
	f.verified = append(f.verified, userID.String())
	return nil
}
func (f *fakeUsers) MintSession(_ context.Context, userID vo.Id, _ string, provider string, idToken *string) (*model.LoginResult, error) {
	if f.failMintSession != nil {
		return nil, f.failMintSession
	}
	f.minted = append(f.minted, provider)
	return &model.LoginResult{Token: "eco_ses_test", User: model.CurrentUserResult{Id: userID.String()}}, nil
}

type harness struct {
	t         *testing.T
	db        *dbtest.DB
	fake      *oidctest.Fake
	users     *fakeUsers
	svc       *appoauth.Service
	ids       appoauth.Identities
	states    appoauth.States
	hands     appoauth.Handoffs
	clock     *fixedClock
	providers []appoauth.Provider
	flow      string // the flow secret the last start-* call returned
}

func newHarness(t *testing.T, trust, allowRegistration bool) *harness {
	db := dbtest.New(t)
	f := oidctest.New(t)
	users := newFakeUsers(t, db)
	ids := oauthrepo.NewIdentityRepo(db.Engine, db.TX)
	states := oauthrepo.NewStateRepo(db.Engine, db.TX)
	hands := oauthrepo.NewHandoffRepo(db.Engine, db.TX)
	clk := &fixedClock{t: time.Now().UTC().Truncate(time.Second)}
	providers := []appoauth.Provider{
		{Client: oidc.NewClient(f.Issuer(model.OAuthProviderGoogle, true), nil), Name: "Google"},
		{Client: oidc.NewClient(f.Issuer(model.OAuthProviderOIDC, trust), nil), Name: "Authentik"},
	}
	svc := appoauth.NewService(providers, users, ids, states, hands, db.TX, clk, nil, "https://app.example.test", allowRegistration)
	return &harness{t: t, db: db, fake: f, users: users, svc: svc, ids: ids, states: states, hands: hands, clock: clk, providers: providers}
}

// login drives start-login + the provider's consent + the callback and returns
// the redirect the callback produced.
func (h *harness) login(provider, client string) string {
	h.t.Helper()
	res, err := h.svc.StartLogin(context.Background(), model.StartOAuthRequest{Provider: provider, Client: client})
	if err != nil {
		h.t.Fatal(err)
	}
	h.flow = res.Flow
	u, _ := url.Parse(res.Url)
	q := u.Query()
	code := h.fake.IssueCode(q.Get("nonce"), q.Get("code_challenge"))
	return h.svc.Callback(context.Background(), provider, appoauth.CallbackInput{Code: code, State: q.Get("state")})
}

// exchangeReq pairs the redirect's handoff code with the flow secret the
// matching start-login returned, as a real client does.
func (h *harness) exchangeReq(t *testing.T, redirect string) model.ExchangeHandoffRequest {
	t.Helper()
	return model.ExchangeHandoffRequest{Code: handoffOf(t, redirect), Flow: h.flow}
}

func handoffOf(t *testing.T, redirect string) string {
	t.Helper()
	u, _ := url.Parse(redirect)
	if strings.HasPrefix(redirect, "econumo://") {
		return u.Query().Get("handoff")
	}
	frag, _ := url.ParseQuery(u.Fragment)
	return frag.Get("handoff")
}

func TestListProviders_FixedOrderAndNames(t *testing.T) {
	h := newHarness(t, false, true)
	got := h.svc.ListProviders()
	if len(got) != 2 || got[0].Id != "google" || got[0].Name != "Google" || got[1].Id != "oidc" || got[1].Name != "Authentik" {
		t.Fatalf("%+v", got)
	}
}

func TestStartLogin_UnconfiguredProvider(t *testing.T) {
	h := newHarness(t, false, true)
	_, err := h.svc.StartLogin(context.Background(), model.StartOAuthRequest{Provider: "apple", Client: "web"})
	v, ok := errs.AsValidation(err)
	if !ok || v.MsgCode != errs.CodeOAuthProviderNotConfigured {
		t.Fatalf("want provider_not_configured, got %v", err)
	}
}

func TestCallback_ProvisionsNewUserAndHandoffExchanges(t *testing.T) {
	h := newHarness(t, false, true)
	h.fake.Email, h.fake.EmailVerified, h.fake.Name = "new@example.test", true, "New Person"
	redirect := h.login("oidc", "web")
	if !strings.HasPrefix(redirect, "https://app.example.test/oauth/callback#handoff=") {
		t.Fatalf("redirect %s", redirect)
	}
	if h.users.provision != 1 {
		t.Fatalf("provision calls %d", h.users.provision)
	}
	res, err := h.svc.ExchangeHandoff(context.Background(), h.exchangeReq(t, redirect), "UA/1")
	if err != nil || res.Token == "" || h.users.minted[0] != "oidc" {
		t.Fatalf("%+v %v minted=%v", res, err, h.users.minted)
	}
	// single use
	if _, err := h.svc.ExchangeHandoff(context.Background(), h.exchangeReq(t, redirect), "UA/1"); err == nil {
		t.Fatal("handoff must be single use")
	}
	// identity recorded
	id, err := h.ids.GetByProviderSubject(context.Background(), "oidc", h.fake.Subject)
	if err != nil || id.Email != "new@example.test" {
		t.Fatalf("%+v %v", id, err)
	}
}

func TestCallback_AppClientRedirectsToScheme(t *testing.T) {
	h := newHarness(t, false, true)
	redirect := h.login("google", "app")
	if !strings.HasPrefix(redirect, "econumo://oauth?handoff=") {
		t.Fatalf("redirect %s", redirect)
	}
}

func TestCallback_ExistingIdentityLogsIn(t *testing.T) {
	h := newHarness(t, false, false) // registration off: must not matter
	u := h.users.seed(t, "old@example.test", model.AlgorithmArgon2id)
	_ = h.ids.Save(context.Background(), model.NewIdentity(vo.NewId(), u.ID, "google", h.fake.Subject, "old@example.test", h.clock.Now()))
	h.fake.Email = "changed@example.test"
	redirect := h.login("google", "web")
	if handoffOf(t, redirect) == "" {
		t.Fatalf("redirect %s", redirect)
	}
	id, _ := h.ids.GetByProviderSubject(context.Background(), "google", h.fake.Subject)
	if id.Email != "changed@example.test" {
		t.Fatal("identity email must follow the claim")
	}
	if len(h.users.replaced) != 0 {
		t.Fatal("a password user's primary email must not move")
	}
}

func TestCallback_EmailDriftMirroredForPasswordlessSingleIdentity(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "old@example.test", model.AlgorithmNone)
	_ = h.ids.Save(context.Background(), model.NewIdentity(vo.NewId(), u.ID, "google", h.fake.Subject, "old@example.test", h.clock.Now()))
	h.fake.Email = "renamed@example.test"
	h.login("google", "web")
	if len(h.users.replaced) != 1 || h.users.replaced[0] != "renamed@example.test" {
		t.Fatalf("replaced %v", h.users.replaced)
	}
}

func TestCallback_EmailDriftNotMirroredWhenAddressTaken(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "old@example.test", model.AlgorithmNone)
	h.users.seed(t, "taken@example.test", model.AlgorithmArgon2id)
	_ = h.ids.Save(context.Background(), model.NewIdentity(vo.NewId(), u.ID, "google", h.fake.Subject, "old@example.test", h.clock.Now()))
	h.fake.Email = "taken@example.test"
	redirect := h.login("google", "web")
	if handoffOf(t, redirect) == "" || len(h.users.replaced) != 0 {
		t.Fatalf("sign-in proceeds for the identity's owner, primary untouched: %s %v", redirect, h.users.replaced)
	}
}

func TestCallback_AutoLinksVerifiedEmail(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "match@example.test", model.AlgorithmArgon2id)
	h.fake.Email, h.fake.EmailVerified = "match@example.test", true
	redirect := h.login("oidc", "web")
	if handoffOf(t, redirect) == "" {
		t.Fatalf("redirect %s", redirect)
	}
	id, err := h.ids.GetByUserProvider(context.Background(), u.ID, "oidc")
	if err != nil || id.Subject != h.fake.Subject {
		t.Fatalf("auto-link missing: %+v %v", id, err)
	}
}

func TestCallback_UnverifiedEmailRejectedUnlessTrusted(t *testing.T) {
	h := newHarness(t, false, true)
	h.fake.EmailVerified = false
	if r := h.login("oidc", "web"); r != "https://app.example.test/login?oauthError=email_unverified" {
		t.Fatalf("redirect %s", r)
	}
	h.fake.EmailVerified = nil // claim absent
	if r := h.login("oidc", "app"); r != "econumo://oauth?error=email_unverified" {
		t.Fatalf("redirect %s", r)
	}
	trusted := newHarness(t, true, true)
	trusted.fake.EmailVerified = nil
	if r := trusted.login("oidc", "web"); handoffOf(t, r) == "" {
		t.Fatalf("trusted issuer must sign in: %s", r)
	}
}

func TestCallback_RegistrationDisabled(t *testing.T) {
	h := newHarness(t, false, false)
	if r := h.login("google", "web"); r != "https://app.example.test/login?oauthError=registration_disabled" {
		t.Fatalf("redirect %s", r)
	}
}

func TestCallback_MissingEmailUsesUserinfoThenFails(t *testing.T) {
	h := newHarness(t, false, true)
	h.fake.OmitEmailInIDToken = true
	h.fake.UserInfoEmail = "from-userinfo@example.test"
	if r := h.login("oidc", "web"); handoffOf(t, r) == "" {
		t.Fatalf("userinfo fallback must sign in: %s", r)
	}
	h2 := newHarness(t, false, true)
	h2.fake.OmitEmailInIDToken = true
	h2.fake.Email = ""
	h2.fake.NoUserInfo = true
	if r := h2.login("oidc", "web"); r != "https://app.example.test/login?oauthError=email_required" {
		t.Fatalf("redirect %s", r)
	}
}

func TestCallback_StateErrors(t *testing.T) {
	h := newHarness(t, false, true)
	if r := h.svc.Callback(context.Background(), "google", appoauth.CallbackInput{Code: "x", State: "unknown"}); r != "https://app.example.test/login?oauthError=invalid_state" {
		t.Fatalf("redirect %s", r)
	}
	res, _ := h.svc.StartLogin(context.Background(), model.StartOAuthRequest{Provider: "google", Client: "web"})
	q := mustQuery(t, res.Url)
	if r := h.svc.Callback(context.Background(), "google", appoauth.CallbackInput{Error: "access_denied", State: q.Get("state")}); r != "https://app.example.test/login?oauthError=denied" {
		t.Fatalf("redirect %s", r)
	}
	// consumed: the same state again is invalid
	code := h.fake.IssueCode(q.Get("nonce"), q.Get("code_challenge"))
	if r := h.svc.Callback(context.Background(), "google", appoauth.CallbackInput{Code: code, State: q.Get("state")}); r != "https://app.example.test/login?oauthError=invalid_state" {
		t.Fatalf("redirect %s", r)
	}
	// provider mismatch
	res2, _ := h.svc.StartLogin(context.Background(), model.StartOAuthRequest{Provider: "oidc", Client: "web"})
	q2 := mustQuery(t, res2.Url)
	if r := h.svc.Callback(context.Background(), "google", appoauth.CallbackInput{Code: "c", State: q2.Get("state")}); r != "https://app.example.test/login?oauthError=invalid_state" {
		t.Fatalf("redirect %s", r)
	}
	// expired state
	res3, _ := h.svc.StartLogin(context.Background(), model.StartOAuthRequest{Provider: "google", Client: "web"})
	q3 := mustQuery(t, res3.Url)
	h.clock.t = h.clock.t.Add(model.OAuthStateTTL + time.Second)
	code3 := h.fake.IssueCode(q3.Get("nonce"), q3.Get("code_challenge"))
	if r := h.svc.Callback(context.Background(), "google", appoauth.CallbackInput{Code: code3, State: q3.Get("state")}); r != "https://app.example.test/login?oauthError=invalid_state" {
		t.Fatalf("expired state: %s", r)
	}
}

func mustQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query()
}

func TestStartLinkAndCallback_Link(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "me@example.test", model.AlgorithmArgon2id)
	res, err := h.svc.StartLink(context.Background(), u.ID, model.StartOAuthRequest{Provider: "google", Client: "web"})
	if err != nil {
		t.Fatal(err)
	}
	q := mustQuery(t, res.Url)
	code := h.fake.IssueCode(q.Get("nonce"), q.Get("code_challenge"))
	r := h.svc.Callback(context.Background(), "google", appoauth.CallbackInput{Code: code, State: q.Get("state")})
	if r != "https://app.example.test/settings/profile/linked-accounts?linked=google" {
		t.Fatalf("redirect %s", r)
	}
	if len(h.users.minted) != 0 {
		t.Fatal("link must mint no session")
	}
	// Linking the same subject again is idempotent; a different user gets identity_taken.
	res2, _ := h.svc.StartLink(context.Background(), u.ID, model.StartOAuthRequest{Provider: "google", Client: "app"})
	q2 := mustQuery(t, res2.Url)
	if r := h.svc.Callback(context.Background(), "google", appoauth.CallbackInput{Code: h.fake.IssueCode(q2.Get("nonce"), q2.Get("code_challenge")), State: q2.Get("state")}); r != "econumo://oauth?linked=google" {
		t.Fatalf("redirect %s", r)
	}
	other := h.users.seed(t, "other@example.test", model.AlgorithmArgon2id)
	res3, _ := h.svc.StartLink(context.Background(), other.ID, model.StartOAuthRequest{Provider: "google", Client: "web"})
	q3 := mustQuery(t, res3.Url)
	if r := h.svc.Callback(context.Background(), "google", appoauth.CallbackInput{Code: h.fake.IssueCode(q3.Get("nonce"), q3.Get("code_challenge")), State: q3.Get("state")}); r != "https://app.example.test/settings/profile/linked-accounts?oauthError=identity_taken" {
		t.Fatalf("redirect %s", r)
	}
	// The same user linking google with a DIFFERENT subject: provider_already_linked.
	h.fake.Subject = "another-google-account"
	res4, _ := h.svc.StartLink(context.Background(), u.ID, model.StartOAuthRequest{Provider: "google", Client: "web"})
	q4 := mustQuery(t, res4.Url)
	if r := h.svc.Callback(context.Background(), "google", appoauth.CallbackInput{Code: h.fake.IssueCode(q4.Get("nonce"), q4.Get("code_challenge")), State: q4.Get("state")}); r != "https://app.example.test/settings/profile/linked-accounts?oauthError=provider_already_linked" {
		t.Fatalf("redirect %s", r)
	}
}

func TestListAndUnlinkIdentities(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "me@example.test", model.AlgorithmNone)
	_ = h.ids.Save(context.Background(), model.NewIdentity(vo.NewId(), u.ID, "google", "g1", "me@example.test", h.clock.Now()))
	list, err := h.svc.ListIdentities(context.Background(), u.ID)
	if err != nil || len(list) != 1 || list[0].Provider != "google" || list[0].CreatedAt == "" {
		t.Fatalf("%+v %v", list, err)
	}
	_, err = h.svc.UnlinkIdentity(context.Background(), u.ID, model.UnlinkIdentityRequest{Provider: "google"})
	if v, ok := errs.AsValidation(err); !ok || v.MsgCode != errs.CodeOAuthLastIdentity {
		t.Fatalf("passwordless single identity must refuse: %v", err)
	}
	_ = h.ids.Save(context.Background(), model.NewIdentity(vo.NewId(), u.ID, "oidc", "o1", "me@example.test", h.clock.Now()))
	if _, err := h.svc.UnlinkIdentity(context.Background(), u.ID, model.UnlinkIdentityRequest{Provider: "google"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.UnlinkIdentity(context.Background(), u.ID, model.UnlinkIdentityRequest{Provider: "google"}); err == nil {
		t.Fatal("unlinking a missing identity is an error")
	}
	pw := h.users.seed(t, "pw@example.test", model.AlgorithmArgon2id)
	_ = h.ids.Save(context.Background(), model.NewIdentity(vo.NewId(), pw.ID, "google", "g2", "pw@example.test", h.clock.Now()))
	if _, err := h.svc.UnlinkIdentity(context.Background(), pw.ID, model.UnlinkIdentityRequest{Provider: "google"}); err != nil {
		t.Fatalf("a password user may unlink their only identity: %v", err)
	}
}

func TestInactiveUserRejected(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "gone@example.test", model.AlgorithmArgon2id)
	u.IsActive = false
	_ = h.ids.Save(context.Background(), model.NewIdentity(vo.NewId(), u.ID, "google", h.fake.Subject, "gone@example.test", h.clock.Now()))
	if r := h.login("google", "web"); r != "https://app.example.test/login?oauthError=account_inactive" {
		t.Fatalf("redirect %s", r)
	}
}

func TestExchangeHandoff_ExpiredAndUnknown(t *testing.T) {
	h := newHarness(t, false, true)
	if _, err := h.svc.ExchangeHandoff(context.Background(), model.ExchangeHandoffRequest{Code: "nope", Flow: "x"}, "ua"); err == nil {
		t.Fatal("unknown code must fail")
	} else if u, ok := errs.AsUnauthorized(err); !ok || u.Code != errs.CodeOAuthHandoffInvalid {
		t.Fatalf("want 401 handoff_invalid, got %v", err)
	}
	redirect := h.login("google", "web")
	h.clock.t = h.clock.t.Add(model.OAuthHandoffTTL + time.Second)
	if _, err := h.svc.ExchangeHandoff(context.Background(), h.exchangeReq(t, redirect), "ua"); err == nil {
		t.Fatal("expired handoff must fail")
	}
}

func TestEndSessionURL(t *testing.T) {
	h := newHarness(t, false, true)
	h.fake.EndSession = true
	u, err := h.svc.EndSessionURL(context.Background(), "oidc", "tok")
	if err != nil || !strings.Contains(u, "id_token_hint=tok") || !strings.Contains(u, url.QueryEscape("https://app.example.test/login")) {
		t.Fatalf("%q %v", u, err)
	}
	if u, _ := h.svc.EndSessionURL(context.Background(), "apple", "tok"); u != "" {
		t.Fatal("unconfigured provider yields no url")
	}
}

func TestCallback_AutoLinkEvictsAPreRegisteredPasswordAccount(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "squatted@example.test", model.AlgorithmArgon2id)
	h.fake.Email, h.fake.EmailVerified = "squatted@example.test", true
	if r := h.login("oidc", "web"); handoffOf(t, r) == "" {
		t.Fatalf("redirect %s", r)
	}
	if len(h.users.revoked) != 1 || h.users.revoked[0] != u.ID.String() {
		t.Fatalf("a password account must lose every session on auto-link: %v", h.users.revoked)
	}
	if len(h.users.verified) != 1 || h.users.verified[0] != u.ID.String() {
		t.Fatalf("the provider's assertion must mark the address verified: %v", h.users.verified)
	}
}

func TestCallback_AutoLinkLeavesAPasswordlessAccountAlone(t *testing.T) {
	h := newHarness(t, false, true)
	h.users.seed(t, "external@example.test", model.AlgorithmNone)
	h.fake.Email, h.fake.EmailVerified = "external@example.test", true
	if r := h.login("oidc", "web"); handoffOf(t, r) == "" {
		t.Fatalf("redirect %s", r)
	}
	if len(h.users.revoked) != 0 || len(h.users.verified) != 0 {
		t.Fatalf("a provider-created account already proved the address: revoked=%v verified=%v", h.users.revoked, h.users.verified)
	}
}

func TestCallback_AutoLinkRollsBackWhenTheEvictionFails(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "squatted@example.test", model.AlgorithmArgon2id)
	h.fake.Email, h.fake.EmailVerified = "squatted@example.test", true
	h.users.failRevokeAllSessions = errBoom
	if r := h.login("oidc", "web"); r != "https://app.example.test/login?oauthError=provider_error" {
		t.Fatalf("redirect %s", r)
	}
	// No half-linked identity: the next attempt would otherwise skip the eviction.
	if _, err := h.ids.GetByUserProvider(context.Background(), u.ID, "oidc"); err == nil {
		t.Fatal("the identity insert must roll back with the eviction")
	}
}

func TestExchangeHandoff_RejectsAForeignFlowSecret(t *testing.T) {
	h := newHarness(t, false, true)
	redirect := h.login("google", "web")
	other, _ := oidc.RandomToken()
	_, err := h.svc.ExchangeHandoff(context.Background(),
		model.ExchangeHandoffRequest{Code: handoffOf(t, redirect), Flow: other}, "ua")
	if u, ok := errs.AsUnauthorized(err); !ok || u.Code != errs.CodeOAuthHandoffInvalid {
		t.Fatalf("want 401 handoff_invalid, got %v", err)
	}
	// Still single use: the row is gone even though the secret did not match.
	if _, err := h.svc.ExchangeHandoff(context.Background(), h.exchangeReq(t, redirect), "ua"); err == nil {
		t.Fatal("the rejected attempt must still consume the handoff")
	}
}

func TestStartLink_MintsAFlowSecretToo(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "me@example.test", model.AlgorithmArgon2id)
	res, err := h.svc.StartLink(context.Background(), u.ID, model.StartOAuthRequest{Provider: "google", Client: "web"})
	if err != nil || len(res.Flow) != 43 {
		t.Fatalf("%+v %v", res, err)
	}
}

// stubLimiter reports the endpoint's global per-minute cap as spent.
type stubLimiter struct{ scope, key string }

func (l *stubLimiter) Allow(scope, key string) error {
	l.scope, l.key = scope, key
	return errs.NewTooManyRequests("Too many attempts. Try again later.")
}

func TestStartLogin_SurfacesTheRateLimit(t *testing.T) {
	h := newHarness(t, false, true)
	lim := &stubLimiter{}
	svc := appoauth.NewService(h.providers, h.users, h.ids, h.states, h.hands, h.db.TX, h.clock, lim,
		"https://app.example.test", true)
	_, err := svc.StartLogin(context.Background(), model.StartOAuthRequest{Provider: "google", Client: "web"})
	if _, ok := errs.AsTooManyRequests(err); !ok {
		t.Fatalf("want 429, got %v", err)
	}
	if lim.scope != appoauth.RateScopeOAuthStart || lim.key != "" {
		t.Fatalf("scope %q key %q", lim.scope, lim.key)
	}
	if _, err := svc.StartLink(context.Background(), vo.NewId(), model.StartOAuthRequest{Provider: "google", Client: "web"}); err == nil {
		t.Fatal("start-link is capped too")
	}
}
