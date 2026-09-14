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
	provision int

	// beforeFindByID runs once on the next FindByID, then clears itself: the
	// seam for landing a reclaim between two of the flow's reads.
	beforeFindByID func()

	// Fault injection for coverage of the service's error-handling branches:
	// non-nil forces the corresponding method to fail regardless of state.
	failFindByID          error
	failFindByEmail       error
	failProvisionExternal error
	failMintSession       error
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
	if f.beforeFindByID != nil {
		hook := f.beforeFindByID
		f.beforeFindByID = nil
		hook()
	}
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

// generationOf is what a real repository read hands back with the aggregate.
func (f *fakeUsers) generationOf(userID vo.Id) int64 {
	if u, ok := f.byID[userID.String()]; ok {
		return u.CredentialsGeneration
	}
	return 0
}

// reclaim bumps the fence the way a completed password reset does — on the
// in-memory aggregate AND in the users row the guarded writes read, so the two
// agree — and drops the identities the reclaim removes.
func (f *fakeUsers) reclaim(userID vo.Id) {
	if u, ok := f.byID[userID.String()]; ok {
		u.CredentialsGeneration++
	}
	for _, q := range []string{
		`UPDATE users SET credentials_generation = credentials_generation + 1 WHERE id = ?`,
		`DELETE FROM users_identities WHERE user_id = ?`,
	} {
		if _, err := f.db.Raw.Exec(f.db.Rebind(q), userID.String()); err != nil {
			f.t.Fatalf("reclaim: %v", err)
		}
	}
}

func (f *fakeUsers) MintSession(_ context.Context, userID vo.Id, _ string, provider string, idToken *string, generation int64) (*model.LoginResult, error) {
	if f.failMintSession != nil {
		return nil, f.failMintSession
	}
	if generation != f.generationOf(userID) {
		return nil, &errs.UnauthorizedError{Msg: "Invalid credentials.", Code: errs.CodeInvalidCredentials}
	}
	f.minted = append(f.minted, provider)
	return &model.LoginResult{Token: "eco_ses_test", User: model.CurrentUserResult{Id: userID.String()}}, nil
}

// fakeNotifier records every IdentityLinked call the auto-link path makes.
type fakeNotifier struct {
	calls []struct{ userID, providerName string }
	fail  error
}

func (f *fakeNotifier) IdentityLinked(_ context.Context, userID vo.Id, providerName string) error {
	f.calls = append(f.calls, struct{ userID, providerName string }{userID.String(), providerName})
	return f.fail
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
	notifier  *fakeNotifier
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
	svc := appoauth.NewService(providers, users, ids, states, hands, clk, nil, "https://app.example.test", allowRegistration)
	notifier := &fakeNotifier{}
	svc.SetNotifier(notifier)
	return &harness{t: t, db: db, fake: f, users: users, svc: svc, ids: ids, states: states, hands: hands, clock: clk, providers: providers, notifier: notifier}
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
	id, err := h.ids.GetByProviderSubject(context.Background(), "oidc", h.fake.IssuerURL(), h.fake.Subject)
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
	saveIdentity(t, h, model.NewIdentity(vo.NewId(), u.ID, "google", h.fake.IssuerURL(), h.fake.Subject, "old@example.test", h.clock.Now()))
	h.fake.Email = "changed@example.test"
	redirect := h.login("google", "web")
	if handoffOf(t, redirect) == "" {
		t.Fatalf("redirect %s", redirect)
	}
	id, _ := h.ids.GetByProviderSubject(context.Background(), "google", h.fake.IssuerURL(), h.fake.Subject)
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
	saveIdentity(t, h, model.NewIdentity(vo.NewId(), u.ID, "google", h.fake.IssuerURL(), h.fake.Subject, "old@example.test", h.clock.Now()))
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
	saveIdentity(t, h, model.NewIdentity(vo.NewId(), u.ID, "google", h.fake.IssuerURL(), h.fake.Subject, "old@example.test", h.clock.Now()))
	h.fake.Email = "taken@example.test"
	redirect := h.login("google", "web")
	if handoffOf(t, redirect) == "" || len(h.users.replaced) != 0 {
		t.Fatalf("sign-in proceeds for the identity's owner, primary untouched: %s %v", redirect, h.users.replaced)
	}
}

func TestCallback_AutoLinksVerifiedEmail(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "match@example.test", model.AlgorithmNone)
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

// startLink drives start-link + consent + callback and returns the redirect,
// remembering the flow secret so completeLink can present it.
func (h *harness) startLink(userID vo.Id, provider, client string) string {
	h.t.Helper()
	res, err := h.svc.StartLink(context.Background(), userID, model.StartOAuthRequest{Provider: provider, Client: client})
	if err != nil {
		h.t.Fatal(err)
	}
	h.flow = res.Flow
	q := mustQuery(h.t, res.Url)
	code := h.fake.IssueCode(q.Get("nonce"), q.Get("code_challenge"))
	return h.svc.Callback(context.Background(), provider, appoauth.CallbackInput{Code: code, State: q.Get("state")})
}

func linkHandoffOf(t *testing.T, redirect string) string {
	t.Helper()
	u, _ := url.Parse(redirect)
	if strings.HasPrefix(redirect, "econumo://") {
		return u.Query().Get("linkHandoff")
	}
	frag, _ := url.ParseQuery(u.Fragment)
	return frag.Get("linkHandoff")
}

// completeLink redeems the parked identity as the SPA does.
func (h *harness) completeLink(userID vo.Id, redirect string) (*model.CompleteLinkResult, error) {
	h.t.Helper()
	return h.svc.CompleteLink(context.Background(), userID, model.CompleteLinkRequest{Code: linkHandoffOf(h.t, redirect), Flow: h.flow})
}

func TestStartLinkAndCallback_Link(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "me@example.test", model.AlgorithmArgon2id)
	r := h.startLink(u.ID, "google", "web")
	if !strings.HasPrefix(r, "https://app.example.test/settings/profile/linked-accounts#linkHandoff=") {
		t.Fatalf("redirect %s", r)
	}
	if len(h.users.minted) != 0 {
		t.Fatal("link must mint no session")
	}
	res, err := h.completeLink(u.ID, r)
	if err != nil || res.Provider != "google" {
		t.Fatalf("%+v %v", res, err)
	}
	if _, err := h.ids.GetByUserProvider(context.Background(), u.ID, "google"); err != nil {
		t.Fatalf("identity not written: %v", err)
	}
	// Linking the same subject again is idempotent, on the app scheme too.
	r2 := h.startLink(u.ID, "google", "app")
	if !strings.HasPrefix(r2, "econumo://oauth?linkHandoff=") {
		t.Fatalf("redirect %s", r2)
	}
	if _, err := h.completeLink(u.ID, r2); err != nil {
		t.Fatal(err)
	}
	// A different user with the same provider subject: identity_taken.
	other := h.users.seed(t, "other@example.test", model.AlgorithmArgon2id)
	if r := h.startLink(other.ID, "google", "web"); r != "https://app.example.test/settings/profile/linked-accounts?oauthError=identity_taken" {
		t.Fatalf("redirect %s", r)
	}
	// The same user linking google with a DIFFERENT subject: provider_already_linked.
	h.fake.Subject = "another-google-account"
	if r := h.startLink(u.ID, "google", "web"); r != "https://app.example.test/settings/profile/linked-accounts?oauthError=provider_already_linked" {
		t.Fatalf("redirect %s", r)
	}
}

// The callback of a link flow arrives in whatever browser followed the
// authorization URL, with no proof of who started the flow. It must therefore
// write nothing: an attacker who mails their own start-link URL to a victim
// would otherwise bind the victim's provider identity to the attacker account.
func TestLinkCallback_WritesNothingUntilTheInitiatingClientCompletesIt(t *testing.T) {
	h := newHarness(t, false, true)
	attacker := h.users.seed(t, "attacker@example.test", model.AlgorithmArgon2id)
	victim := h.users.seed(t, "victim@example.test", model.AlgorithmArgon2id)
	r := h.startLink(attacker.ID, "google", "web")
	if linkHandoffOf(t, r) == "" {
		t.Fatalf("redirect %s", r)
	}
	if _, err := h.ids.GetByProviderSubject(context.Background(), "google", h.fake.IssuerURL(), h.fake.Subject); err == nil {
		t.Fatal("the callback must not persist the identity")
	}
	// The victim, signed in to their own account, cannot redeem the attacker's code.
	if _, err := h.completeLink(victim.ID, r); err == nil {
		t.Fatal("a handoff minted for another account must be refused")
	}
	if _, err := h.ids.GetByUserProvider(context.Background(), victim.ID, "google"); err == nil {
		t.Fatal("no identity may be written for the victim")
	}
}

func TestCompleteLink_RejectsAForeignFlowSecretAndReplays(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "me@example.test", model.AlgorithmArgon2id)
	r := h.startLink(u.ID, "google", "web")
	code := linkHandoffOf(t, r)
	other, _ := oidc.RandomToken()
	_, err := h.svc.CompleteLink(context.Background(), u.ID, model.CompleteLinkRequest{Code: code, Flow: other})
	if v, ok := errs.AsValidation(err); !ok || v.MsgCode != errs.CodeOAuthLinkInvalid {
		t.Fatalf("want link_invalid, got %v", err)
	}
	// Single use: the rejected attempt consumed the code.
	if _, err := h.completeLink(u.ID, r); err == nil {
		t.Fatal("the rejected attempt must still consume the link handoff")
	}
	if _, err := h.ids.GetByUserProvider(context.Background(), u.ID, "google"); err == nil {
		t.Fatal("nothing may have been written")
	}
}

func TestCompleteLink_RejectsASignInHandoff(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "me@example.test", model.AlgorithmNone)
	saveIdentity(t, h, model.NewIdentity(vo.NewId(), u.ID, "google", h.fake.IssuerURL(), h.fake.Subject, "me@example.test", h.clock.Now()))
	redirect := h.login("google", "web")
	_, err := h.svc.CompleteLink(context.Background(), u.ID, model.CompleteLinkRequest{Code: handoffOf(t, redirect), Flow: h.flow})
	if v, ok := errs.AsValidation(err); !ok || v.MsgCode != errs.CodeOAuthLinkInvalid {
		t.Fatalf("a sign-in handoff is not a link handoff: %v", err)
	}
}

func TestCompleteLink_ExpiredCode(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "me@example.test", model.AlgorithmArgon2id)
	r := h.startLink(u.ID, "google", "web")
	h.clock.t = h.clock.t.Add(model.OAuthHandoffTTL + time.Second)
	if _, err := h.completeLink(u.ID, r); err == nil {
		t.Fatal("an expired link handoff must be refused")
	}
}

func TestListAndUnlinkIdentities(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "me@example.test", model.AlgorithmNone)
	saveIdentity(t, h, model.NewIdentity(vo.NewId(), u.ID, "google", h.fake.IssuerURL(), "g1", "me@example.test", h.clock.Now()))
	list, err := h.svc.ListIdentities(context.Background(), u.ID)
	if err != nil || len(list) != 1 || list[0].Provider != "google" || list[0].CreatedAt == "" {
		t.Fatalf("%+v %v", list, err)
	}
	_, err = h.svc.UnlinkIdentity(context.Background(), u.ID, model.UnlinkIdentityRequest{Provider: "google"})
	if v, ok := errs.AsValidation(err); !ok || v.MsgCode != errs.CodeOAuthLastIdentity {
		t.Fatalf("passwordless single identity must refuse: %v", err)
	}
	saveIdentity(t, h, model.NewIdentity(vo.NewId(), u.ID, "oidc", h.fake.IssuerURL(), "o1", "me@example.test", h.clock.Now()))
	if _, err := h.svc.UnlinkIdentity(context.Background(), u.ID, model.UnlinkIdentityRequest{Provider: "google"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.UnlinkIdentity(context.Background(), u.ID, model.UnlinkIdentityRequest{Provider: "google"}); err == nil {
		t.Fatal("unlinking a missing identity is an error")
	}
	pw := h.users.seed(t, "pw@example.test", model.AlgorithmArgon2id)
	saveIdentity(t, h, model.NewIdentity(vo.NewId(), pw.ID, "google", h.fake.IssuerURL(), "g2", "pw@example.test", h.clock.Now()))
	if _, err := h.svc.UnlinkIdentity(context.Background(), pw.ID, model.UnlinkIdentityRequest{Provider: "google"}); err != nil {
		t.Fatalf("a password user may unlink their only identity: %v", err)
	}
}

func TestInactiveUserRejected(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "gone@example.test", model.AlgorithmArgon2id)
	u.IsActive = false
	saveIdentity(t, h, model.NewIdentity(vo.NewId(), u.ID, "google", h.fake.IssuerURL(), h.fake.Subject, "gone@example.test", h.clock.Now()))
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

// An account with a password is never merged into. Whoever set that password
// never had to prove they own the address, so the account may be a squatter's:
// evicting the password is not enough, because everything else they left on it
// (other linked identities above all) would be inherited by the person the
// provider just vouched for.
func TestCallback_RefusesToMergeIntoAPasswordAccount(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "squatted@example.test", model.AlgorithmArgon2id)
	h.fake.Email, h.fake.EmailVerified = "squatted@example.test", true
	if r := h.login("oidc", "web"); r != "https://app.example.test/login?oauthError=account_exists_password" {
		t.Fatalf("redirect %s", r)
	}
	if _, err := h.ids.GetByUserProvider(context.Background(), u.ID, "oidc"); err == nil {
		t.Fatal("no identity may be attached to an account whose owner has not authenticated")
	}
	if len(h.users.minted) != 0 {
		t.Fatalf("no session may be minted: %v", h.users.minted)
	}
	if len(h.notifier.calls) != 0 {
		t.Fatalf("nothing happened, so nothing to notify about: %+v", h.notifier.calls)
	}
}

// The takeover the refusal closes: a squatter who pre-registered the address
// AND linked their own provider identity to it keeps that identity through any
// credential eviction, so the identity — not just the password — is why the
// merge cannot happen at all.
func TestCallback_RefusesEvenWhenTheSquatterAlreadyLinkedAProvider(t *testing.T) {
	h := newHarness(t, false, true)
	squatted := h.users.seed(t, "victim@example.test", model.AlgorithmArgon2id)
	attacker := model.NewIdentity(vo.NewId(), squatted.ID, "google", h.fake.IssuerURL(), "attacker-google-sub", "attacker@example.test", h.clock.Now())
	saveIdentity(t, h, attacker)
	h.fake.Email, h.fake.EmailVerified = "victim@example.test", true
	if r := h.login("oidc", "web"); r != "https://app.example.test/login?oauthError=account_exists_password" {
		t.Fatalf("redirect %s", r)
	}
	still, err := h.ids.GetByUserProvider(context.Background(), squatted.ID, "google")
	if err != nil || still.Subject != "attacker-google-sub" {
		t.Fatalf("the pre-existing identity is untouched, because nothing was merged: %+v %v", still, err)
	}
	if n, _ := h.ids.CountByUser(context.Background(), squatted.ID); n != 1 {
		t.Fatalf("no second identity may be attached: %d", n)
	}
}

func TestCallback_AutoLinksIntoAPasswordlessAccount(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "external@example.test", model.AlgorithmNone)
	h.fake.Email, h.fake.EmailVerified = "external@example.test", true
	if r := h.login("oidc", "web"); handoffOf(t, r) == "" {
		t.Fatalf("a provider-created account already proved the address: %s", r)
	}
	if _, err := h.ids.GetByUserProvider(context.Background(), u.ID, "oidc"); err != nil {
		t.Fatalf("identity not linked: %v", err)
	}
}

func TestCallback_AutoLinkNotifiesTheAccountOwner(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "external@example.test", model.AlgorithmNone)
	h.fake.Email, h.fake.EmailVerified = "external@example.test", true
	if r := h.login("oidc", "web"); handoffOf(t, r) == "" {
		t.Fatalf("redirect %s", r)
	}
	if len(h.notifier.calls) != 1 {
		t.Fatalf("want exactly one notification, got %+v", h.notifier.calls)
	}
	if got := h.notifier.calls[0]; got.userID != u.ID.String() || got.providerName != "Authentik" {
		t.Fatalf("notification = %+v, want user %s provider Authentik", got, u.ID.String())
	}
}

func TestCallback_AutoLinkNotifiesWithTheProviderDisplayName(t *testing.T) {
	h := newHarness(t, true, true)
	h.users.seed(t, "external-google@example.test", model.AlgorithmNone)
	h.fake.Email, h.fake.EmailVerified = "external-google@example.test", true
	if r := h.login("google", "web"); handoffOf(t, r) == "" {
		t.Fatalf("redirect %s", r)
	}
	if len(h.notifier.calls) != 1 || h.notifier.calls[0].providerName != "Google" {
		t.Fatalf("notification = %+v, want provider Google", h.notifier.calls)
	}
}

func TestCallback_AutoLinkNotifierFailureDoesNotBreakTheRedirect(t *testing.T) {
	h := newHarness(t, false, true)
	h.users.seed(t, "external@example.test", model.AlgorithmNone)
	h.fake.Email, h.fake.EmailVerified = "external@example.test", true
	h.notifier.fail = errBoom
	r := h.login("oidc", "web")
	if handoffOf(t, r) == "" {
		t.Fatalf("a failing notifier must not affect the redirect: %s", r)
	}
	if len(h.notifier.calls) != 1 {
		t.Fatalf("notifier should still have been called once: %+v", h.notifier.calls)
	}
}

func TestCallback_ProvisioningDoesNotNotify(t *testing.T) {
	h := newHarness(t, false, true)
	h.fake.Email, h.fake.EmailVerified = "brandnew@example.test", true
	if r := h.login("oidc", "web"); handoffOf(t, r) == "" {
		t.Fatalf("redirect %s", r)
	}
	if len(h.notifier.calls) != 0 {
		t.Fatalf("provisioning a new user is not an auto-link: %+v", h.notifier.calls)
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
	svc := appoauth.NewService(h.providers, h.users, h.ids, h.states, h.hands, h.clock, lim,
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

// loginVia drives a full sign-in through another service/issuer pair, for the
// operator-repointed-the-issuer cases below.
func loginVia(t *testing.T, svc *appoauth.Service, f *oidctest.Fake, provider, client string) string {
	t.Helper()
	res, err := svc.StartLogin(context.Background(), model.StartOAuthRequest{Provider: provider, Client: client})
	if err != nil {
		t.Fatal(err)
	}
	q := mustQuery(t, res.Url)
	code := f.IssueCode(q.Get("nonce"), q.Get("code_challenge"))
	return svc.Callback(context.Background(), provider, appoauth.CallbackInput{Code: code, State: q.Get("state")})
}

// serviceOver builds a second service on the same storage but a different
// issuer — what an operator does by repointing ECONUMO_OIDC_ISSUER_URL.
func (h *harness) serviceOver(f *oidctest.Fake) *appoauth.Service {
	return appoauth.NewService([]appoauth.Provider{{Client: oidc.NewClient(f.Issuer(model.OAuthProviderOIDC, false), nil), Name: "New IdP"}},
		h.users, h.ids, h.states, h.hands, h.clock, nil, "https://app.example.test", true)
}

// The custom slot's provider id is always "oidc", so a subject is only unique
// within its issuer. A user at a replacement issuer whose subject collides with
// a stored one must NOT be authenticated as that row's owner.
func TestCallback_IdentitiesAreScopedToTheIssuer(t *testing.T) {
	h := newHarness(t, false, true)
	h.fake.Subject, h.fake.Email, h.fake.EmailVerified = "shared-subject", "first@example.test", true
	if r := h.login("oidc", "web"); handoffOf(t, r) == "" {
		t.Fatalf("redirect %s", r)
	}
	first, err := h.ids.GetByProviderSubject(context.Background(), "oidc", h.fake.IssuerURL(), "shared-subject")
	if err != nil {
		t.Fatal(err)
	}

	other := oidctest.New(t)
	other.Subject, other.Email, other.EmailVerified = "shared-subject", "second@example.test", true
	if r := loginVia(t, h.serviceOver(other), other, "oidc", "web"); handoffOf(t, r) == "" {
		t.Fatalf("redirect %s", r)
	}
	got, err := h.ids.GetByProviderSubject(context.Background(), "oidc", other.IssuerURL(), "shared-subject")
	if err != nil {
		t.Fatalf("the new issuer's identity must be its own row: %v", err)
	}
	if got.UserID.Equal(first.UserID) {
		t.Fatal("a subject collision across issuers must not resolve to the first issuer's user")
	}
}

// After an issuer change the returning user still matches by verified email;
// their one row per slot is repointed rather than colliding on (user, provider).
func TestCallback_AutoLinkRepointsAnIdentityAfterAnIssuerChange(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "same@example.test", model.AlgorithmNone)
	saveIdentity(t, h, model.NewIdentity(vo.NewId(), u.ID, "oidc", "https://old.example.test", "old-subject", "same@example.test", h.clock.Now()))

	other := oidctest.New(t)
	other.Subject, other.Email, other.EmailVerified = "new-subject", "same@example.test", true
	if r := loginVia(t, h.serviceOver(other), other, "oidc", "web"); handoffOf(t, r) == "" {
		t.Fatalf("redirect %s", r)
	}
	got, err := h.ids.GetByUserProvider(context.Background(), u.ID, "oidc")
	if err != nil || got.Issuer != other.IssuerURL() || got.Subject != "new-subject" {
		t.Fatalf("identity must follow the new issuer: %+v %v", got, err)
	}
	if n, _ := h.ids.CountByUser(context.Background(), u.ID); n != 1 {
		t.Fatalf("one row per slot, got %d", n)
	}
}

// The recovery half of the squatting defence: a completed password reset proves
// the mailbox, so every sign-in method that never proved it goes — but the one
// that vouches for the very address just proven stays, because obtaining it
// needs that same mailbox.
func TestReclaimAccount(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "owner@example.test", model.AlgorithmArgon2id)
	ctx := context.Background()
	mine := model.NewIdentity(vo.NewId(), u.ID, "google", h.fake.IssuerURL(), "g-owner", "Owner@Example.test", h.clock.Now())
	theirs := model.NewIdentity(vo.NewId(), u.ID, "oidc", h.fake.IssuerURL(), "o-squatter", "squatter@example.test", h.clock.Now())
	for _, i := range []*model.Identity{mine, theirs} {
		saveIdentity(t, h, i)
	}

	// A sign-in code minted moments ago is a session waiting to be claimed, and
	// 60 seconds is long enough to hold one across a reset.
	pending := &model.OAuthHandoff{CodeHash: "pending-hash", Kind: model.OAuthHandoffKindLogin, UserID: u.ID,
		Provider: "google", FlowHash: "fh", CreatedAt: h.clock.Now(), ExpiresAt: h.clock.Now().Add(model.OAuthHandoffTTL)}
	if err := h.hands.Insert(ctx, pending); err != nil {
		t.Fatal(err)
	}

	n, grants, err := h.svc.ReclaimAccount(ctx, u.ID, "owner@example.test")
	if err != nil || n != 1 {
		t.Fatalf("removed %d identities (%v), want 1", n, err)
	}
	if grants != 1 {
		t.Fatalf("revoked %d pending grants, want 1", grants)
	}
	if _, err := h.hands.Get(ctx, "pending-hash"); err == nil {
		t.Fatal("an unredeemed handoff must not survive the reclaim")
	}
	if _, err := h.ids.GetByUserProvider(ctx, u.ID, "oidc"); err == nil {
		t.Fatal("an identity claiming another address must not survive the reclaim")
	}
	if _, err := h.ids.GetByUserProvider(ctx, u.ID, "google"); err != nil {
		t.Fatalf("the identity vouching for the proven address stays: %v", err)
	}

	// Idempotent: nothing foreign or pending is left to remove.
	if n, grants, err := h.svc.ReclaimAccount(ctx, u.ID, "owner@example.test"); err != nil || n != 0 || grants != 0 {
		t.Fatalf("removed %d/%d (%v), want 0/0", n, grants, err)
	}
}

// saveIdentity seeds an identity through the guarded write at the user's
// current generation, the way a live flow would.
func saveIdentity(t *testing.T, h *harness, i *model.Identity) {
	t.Helper()
	n, err := h.ids.InsertIfCurrent(context.Background(), i, h.users.generationOf(i.UserID))
	if err != nil || n != 1 {
		t.Fatalf("seed identity: %d %v", n, err)
	}
}

// The reported race, without the racing: a redemption that consumed its
// handoff before the reclaim must not mint a session after it. The fence is a
// generation stamped on the handoff and checked by the database at insert
// time, so the outcome does not depend on when the goroutine was paused.
func TestExchangeHandoff_RefusedAfterAReclaim(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "owner@example.test", model.AlgorithmNone)
	saveIdentity(t, h, model.NewIdentity(vo.NewId(), u.ID, "google", h.fake.IssuerURL(), h.fake.Subject, "owner@example.test", h.clock.Now()))
	h.fake.Email, h.fake.EmailVerified = "owner@example.test", true
	redirect := h.login("google", "web")
	if handoffOf(t, redirect) == "" {
		t.Fatalf("redirect %s", redirect)
	}

	h.users.reclaim(u.ID) // the owner completes a password reset

	if _, err := h.svc.ExchangeHandoff(context.Background(), h.exchangeReq(t, redirect), "ua"); err == nil {
		t.Fatal("a handoff resolved before the reclaim must not mint a session after it")
	}
}

// The other half of the same race: a callback already in flight must not land
// an identity the reclaim has just removed.
func TestCallback_IdentityWriteRefusedAfterAReclaim(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "owner@example.test", model.AlgorithmNone)
	h.users.reclaim(u.ID) // the flow below reads the stale generation 0

	stale := model.NewIdentity(vo.NewId(), u.ID, "google", h.fake.IssuerURL(), "sub", "owner@example.test", h.clock.Now())
	n, err := h.ids.InsertIfCurrent(context.Background(), stale, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("a write from before the reclaim must affect no rows")
	}
	if _, err := h.ids.GetByUserProvider(context.Background(), u.ID, "google"); err == nil {
		t.Fatal("no identity may exist")
	}
}

// The gap the fence used to leave open: the callback reads the identity, the
// reclaim commits, and only then does the callback read the user. Reading the
// generation WITH the user closes it — the identity row is already gone, so the
// fenced UPDATE writes nothing and the flow is void.
func TestCallback_ExistingIdentity_ReclaimBetweenIdentityAndUserReadIsRefused(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "victim@example.test", model.AlgorithmNone)
	saveIdentity(t, h, model.NewIdentity(vo.NewId(), u.ID, "google", h.fake.IssuerURL(), h.fake.Subject, "squatter@example.test", h.clock.Now()))
	h.fake.Email, h.fake.EmailVerified = "squatter@example.test", true
	// The reclaim lands after GetByProviderSubject and before FindByID.
	h.users.beforeFindByID = func() { h.users.reclaim(u.ID) }

	redirect := h.login("google", "web")
	if !strings.Contains(redirect, "oauthError=provider_error") {
		t.Fatalf("expected provider_error redirect, got %s", redirect)
	}
	if n := handoffCount(t, h.db); n != 0 {
		t.Fatalf("a handoff was minted after the reclaim: %d", n)
	}
	if _, err := h.ids.GetByProviderSubject(context.Background(), "google", h.fake.IssuerURL(), h.fake.Subject); err == nil {
		t.Fatal("the reclaimed identity was resurrected")
	}
}

func handoffCount(t *testing.T, db *dbtest.DB) int {
	t.Helper()
	var n int
	if err := db.Raw.QueryRow(`SELECT COUNT(*) FROM oauth_handoffs`).Scan(&n); err != nil {
		t.Fatalf("count handoffs: %v", err)
	}
	return n
}
