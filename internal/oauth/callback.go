package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/infra/oidc"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

// CallbackInput is what the provider sent back: query parameters for Google
// and the custom slot, form fields for Apple (AppleUser is Apple's one-time
// JSON `user` field carrying the name).
type CallbackInput struct {
	Code      string
	State     string
	Error     string
	AppleUser string
}

// Callback resolves the provider's answer to a redirect URL. It never returns
// an error: every failure becomes an error redirect (spec §6.2/§6.3), and the
// internal cause goes to the operation log only.
func (s *Service) Callback(ctx context.Context, provider string, in CallbackInput) string {
	st, err := s.consumeState(ctx, provider, in.State)
	if err != nil {
		logWarn(ctx, "oauth callback: state", err, "provider", provider)
		// The row (and with it the intent) is gone; the client prefix on the
		// state value itself still says which surface to report on. It is a
		// display hint only — nothing is authorized by it.
		return s.errorURL(clientFromState(in.State), "invalid_state")
	}
	reqctx.AddLogAttr(ctx, "oauth_intent", st.Intent)
	if in.Error != "" {
		return s.errorURLFor(st, "denied")
	}
	p, perr := s.provider(provider)
	if perr != nil {
		return s.errorURLFor(st, "provider_error")
	}
	now := s.clock.Now()
	toks, err := p.Client.Exchange(ctx, in.Code, st.CodeVerifier, s.RedirectURI(provider), now)
	if err != nil {
		logWarn(ctx, "oauth callback: exchange", err, "provider", provider)
		return s.errorURLFor(st, "provider_error")
	}
	claims, err := p.Client.VerifyIDToken(ctx, toks.IDToken, st.Nonce, now)
	if err != nil {
		logWarn(ctx, "oauth callback: id token", err, "provider", provider)
		return s.errorURLFor(st, "provider_error")
	}
	if claims.Email == "" && toks.AccessToken != "" {
		claims = s.userinfoFallback(ctx, p, toks.AccessToken, claims)
	}
	if claims.Name == "" && in.AppleUser != "" {
		claims.Name = appleName(in.AppleUser)
	}
	// Step 4: email present and verified/trusted, for every intent.
	if claims.Email == "" {
		return s.errorURLFor(st, "email_required")
	}
	if !claims.EmailVerified && !p.Client.Issuer().TrustEmail {
		return s.errorURLFor(st, "email_unverified")
	}
	email := strings.ToLower(strings.TrimSpace(claims.Email))
	issuer := p.Client.Issuer().IssuerURL

	if st.Intent == model.OAuthIntentLink {
		return s.link(ctx, st, provider, issuer, claims.Subject, email)
	}
	return s.login(ctx, st, provider, issuer, claims, email, toks.IDToken)
}

// consumeState loads and DELETES the state row; a consumed state is invalid.
// The delete's affected-row count decides the race: a presenter whose delete
// removed no row is rejected, so a concurrent replay cannot both pass on
// either engine.
func (s *Service) consumeState(ctx context.Context, provider, state string) (*model.OAuthState, error) {
	if state == "" {
		return nil, errs.NewNotFound("state missing")
	}
	hash := oidc.Sha256Hex(state)
	st, err := s.states.Get(ctx, hash)
	if err != nil {
		return nil, err
	}
	n, derr := s.states.Delete(ctx, hash)
	if derr != nil {
		return nil, derr
	}
	if n != 1 {
		return nil, errs.NewNotFound("state already consumed")
	}
	if st.Provider != provider {
		return nil, errs.NewNotFound("state provider mismatch")
	}
	if st.IsExpired(s.clock.Now()) {
		return nil, errs.NewNotFound("state expired")
	}
	return st, nil
}

// clientFromState reads the display-hint prefix start() bakes into the state
// value (web./app.) so a miss in consumeState can still pick an error
// surface. Anything that isn't recognizably the app's prefix defaults to web,
// same as an empty/garbage State would have hit the web page before this
// change.
func clientFromState(state string) string {
	if strings.HasPrefix(state, model.OAuthClientApp+".") {
		return model.OAuthClientApp
	}
	return model.OAuthClientWeb
}

// userinfoFallback fills email/verified/name only where the ID token left them
// empty, and only when userinfo's sub matches. Failure is not fatal.
func (s *Service) userinfoFallback(ctx context.Context, p Provider, accessToken string, claims oidc.Claims) oidc.Claims {
	info, err := p.Client.UserInfo(ctx, accessToken)
	if err != nil || info.Subject != claims.Subject {
		if err != nil {
			logWarn(ctx, "oauth callback: userinfo", err, "provider", p.Client.Issuer().ID)
		}
		return claims
	}
	if claims.Email == "" {
		claims.Email = info.Email
		claims.EmailVerified = info.EmailVerified
	}
	if claims.Name == "" {
		claims.Name = info.Name
	}
	return claims
}

// appleName extracts "firstName lastName" from Apple's first-sign-in user field.
func appleName(raw string) string {
	var u struct {
		Name struct {
			FirstName string `json:"firstName"`
			LastName  string `json:"lastName"`
		} `json:"name"`
	}
	if json.Unmarshal([]byte(raw), &u) != nil {
		return ""
	}
	return strings.TrimSpace(u.Name.FirstName + " " + u.Name.LastName)
}

func (s *Service) login(ctx context.Context, st *model.OAuthState, provider, issuer string, claims oidc.Claims, email, idToken string) string {
	now := s.clock.Now()
	var tokenForSession *string
	if provider == model.OAuthProviderOIDC {
		t := idToken
		tokenForSession = &t
	}

	// Step 5: existing identity.
	id, err := s.identities.GetByProviderSubject(ctx, provider, issuer, claims.Subject)
	if err == nil {
		u, uerr := s.users.FindByID(ctx, id.UserID)
		if uerr != nil {
			logWarn(ctx, "oauth callback: identity owner", uerr, "provider", provider)
			return s.errorURLFor(st, "provider_error")
		}
		if !u.IsActive {
			return s.errorURLFor(st, "account_inactive")
		}
		// u.CredentialsGeneration is the fence value read WITH the user row. The
		// identity row was read before it, so a reclaim landing between the two
		// reads has deleted the identity — unless it vouches for the very
		// address the reset proved, which ReclaimAccount keeps, and the flow
		// legitimately continues under the new generation. The fenced UPDATE
		// decides which of the two happened.
		id.UpdateEmail(email, now)
		if n, serr := s.identities.UpdateIfCurrent(ctx, id, u.CredentialsGeneration); serr != nil || n != 1 {
			logWarn(ctx, "oauth callback: identity save", orReclaimed(serr), "provider", provider)
			return s.errorURLFor(st, "provider_error")
		}
		s.mirrorEmailDrift(ctx, u, email, provider)
		return s.mintHandoff(ctx, st, u.ID, provider, tokenForSession, u.CredentialsGeneration)
	}
	if _, ok := errs.AsNotFound(err); !ok {
		logWarn(ctx, "oauth callback: identity lookup", err, "provider", provider)
		return s.errorURLFor(st, "provider_error")
	}

	// Step 6: auto-link by (verified) email.
	u, err := s.users.FindByEmail(ctx, email)
	if err == nil {
		if !u.IsActive {
			return s.errorURLFor(st, "account_inactive")
		}
		// A password on the account proves nothing about who owns the address:
		// registration does not always verify it, so anyone could have claimed
		// it first. Merging into such an account is refused outright — no
		// eviction is thorough enough, because everything the squatter left
		// behind (other linked identities, shared budgets, invites) would be
		// inherited by whoever the provider just vouched for. The account owner
		// signs in with their password (or resets it through the mailbox the
		// provider just proved they hold) and links the provider from Settings,
		// where the link is bound to an authenticated session.
		if u.HasPassword() {
			reqctx.AddLogAttr(ctx, "oauth_password_account", true)
			return s.errorURLFor(st, "account_exists_password")
		}
		if serr := s.autoLink(ctx, u, provider, issuer, claims.Subject, email, now, u.CredentialsGeneration); serr != nil {
			logWarn(ctx, "oauth callback: auto-link", serr, "provider", provider)
			return s.errorURLFor(st, "provider_error")
		}
		reqctx.AddLogAttr(ctx, "oauth_linked", true)
		return s.mintHandoff(ctx, st, u.ID, provider, tokenForSession, u.CredentialsGeneration)
	}
	if _, ok := errs.AsNotFound(err); !ok {
		logWarn(ctx, "oauth callback: user lookup", err, "provider", provider)
		return s.errorURLFor(st, "provider_error")
	}

	// Step 7: provision.
	if !s.allowRegistration {
		return s.errorURLFor(st, "registration_disabled")
	}
	u, err = s.users.ProvisionExternal(ctx, deriveName(claims.Name, email), email)
	if err != nil {
		logWarn(ctx, "oauth callback: provision", err, "provider", provider)
		return s.errorURLFor(st, "provider_error")
	}
	// Freshly provisioned: nothing can have reclaimed it, so the generation the
	// provisioning read returned is still current; the write takes the same
	// guarded path so there is only one way to persist an identity.
	if n, serr := s.identities.InsertIfCurrent(ctx, model.NewIdentity(s.identities.NextIdentity(), u.ID, provider, issuer, claims.Subject, email, now), u.CredentialsGeneration); serr != nil || n != 1 {
		logWarn(ctx, "oauth callback: identity insert", orReclaimed(serr), "provider", provider)
		return s.errorURLFor(st, "provider_error")
	}
	reqctx.AddLogAttr(ctx, "oauth_provisioned", true)
	return s.mintHandoff(ctx, st, u.ID, provider, tokenForSession, u.CredentialsGeneration)
}

// autoLink attaches the provider identity to a PASSWORDLESS account found by
// email. Such an account was created through a provider, so its owner already
// proved the address and a second provider asserting the same verified address
// is the same person; an account with a password never reaches here (step 6).
// The owner is still told, best-effort: gaining a sign-in method without
// asking for one must be noticeable.
func (s *Service) autoLink(ctx context.Context, u *model.User, provider, issuer, subject, email string, now time.Time, generation int64) error {
	if err := s.saveLinkedIdentity(ctx, u.ID, provider, issuer, subject, email, now, generation); err != nil {
		return err
	}
	if s.notifier != nil {
		if nerr := s.notifier.IdentityLinked(ctx, u.ID, s.byID[provider].Name); nerr != nil {
			logWarn(ctx, "oauth callback: identity-linked notice", nerr, "user_id", u.ID.String(), "provider", provider)
		}
	}
	return nil
}

// saveLinkedIdentity writes the slot's identity for a user, repointing the
// existing row when the issuer or subject moved (an operator changing
// ECONUMO_OIDC_ISSUER_URL): one row per (user, provider) is all the schema
// allows, and the verified email already proved the account is theirs.
func (s *Service) saveLinkedIdentity(ctx context.Context, userID vo.Id, provider, issuer, subject, email string, now time.Time, generation int64) error {
	existing, err := s.identities.GetByUserProvider(ctx, userID, provider)
	switch {
	case err == nil:
		existing.Repoint(issuer, subject, email, now)
		return rowsOrReclaimed(s.identities.UpdateIfCurrent(ctx, existing, generation))
	default:
		if _, ok := errs.AsNotFound(err); !ok {
			return err
		}
		return rowsOrReclaimed(s.identities.InsertIfCurrent(ctx, model.NewIdentity(s.identities.NextIdentity(), userID, provider, issuer, subject, email, now), generation))
	}
}

// errReclaimed reports a write the account's reclaim fence refused: the user
// reset their password after this flow read its evidence, so the flow is void.
var errReclaimed = errors.New("account reclaimed mid-flow")

func rowsOrReclaimed(n int64, err error) error {
	if err != nil {
		return err
	}
	if n != 1 {
		return errReclaimed
	}
	return nil
}

func orReclaimed(err error) error {
	if err != nil {
		return err
	}
	return errReclaimed
}

// mirrorEmailDrift applies the spec's email-drift rule for an existing identity.
func (s *Service) mirrorEmailDrift(ctx context.Context, u *model.User, email, provider string) {
	if strings.EqualFold(strings.TrimSpace(u.Email), email) {
		return
	}
	if u.HasPassword() {
		return
	}
	n, err := s.identities.CountByUser(ctx, u.ID)
	if err != nil || n != 1 {
		return
	}
	other, ferr := s.users.FindByEmail(ctx, email)
	if ferr == nil {
		slog.WarnContext(ctx, "oauth email drift: address belongs to another user",
			"user_id", u.ID.String(), "other_user_id", other.ID.String(), "provider", provider)
		return
	}
	// Only a NotFound proves the address is free; any other failure is
	// inconclusive, so the mirror is skipped rather than risking a collision.
	if _, ok := errs.AsNotFound(ferr); !ok {
		logWarn(ctx, "oauth email drift: address lookup failed", ferr, "user_id", u.ID.String(), "provider", provider)
		return
	}
	if err := s.users.ReplaceVerifiedEmail(ctx, u.ID, email); err != nil {
		logWarn(ctx, "oauth email drift: replace failed", err, "user_id", u.ID.String())
	}
}

func (s *Service) mintHandoff(ctx context.Context, st *model.OAuthState, userID vo.Id, provider string, idToken *string, generation int64) string {
	code, err := oidc.RandomToken()
	if err != nil {
		return s.errorURLFor(st, "provider_error")
	}
	now := s.clock.Now()
	if err := s.handoffs.Insert(ctx, &model.OAuthHandoff{CodeHash: oidc.Sha256Hex(code), Kind: model.OAuthHandoffKindLogin, UserID: userID, Provider: provider,
		FlowHash: st.FlowHash, IDToken: idToken, Generation: generation, CreatedAt: now, ExpiresAt: now.Add(model.OAuthHandoffTTL)}); err != nil {
		logWarn(ctx, "oauth callback: handoff insert", err, "provider", provider)
		return s.errorURLFor(st, "provider_error")
	}
	reqctx.AddLogAttr(ctx, "user_id", userID.String())
	return s.successURL(st.Client, code)
}

// link resolves the callback of a link flow WITHOUT writing the identity. The
// callback carries no proof of who started the flow — the provider redirects
// whichever browser followed the authorization URL — so an attacker could
// otherwise mail their own start-link URL to a victim and have the victim's
// identity saved against the attacker's account. The resolved identity is
// parked in a one-shot link handoff instead; CompleteLink performs the write
// once the initiating client presents its flow secret on an authenticated
// request. The taken/already-linked checks still run here so the user sees the
// real reason on the redirect rather than after a pointless round trip.
func (s *Service) link(ctx context.Context, st *model.OAuthState, provider, issuer, subject, email string) string {
	// The fence travels on the handoff: CompleteLink runs on a later request
	// whose session was authenticated before the write, so reading it there
	// would leave the same gap this capture closes.
	owner, oerr := s.users.FindByID(ctx, st.LinkUserID)
	if oerr != nil {
		logWarn(ctx, "oauth link: owner lookup", oerr, "provider", provider)
		return s.errorURLFor(st, "provider_error")
	}
	existing, err := s.identities.GetByProviderSubject(ctx, provider, issuer, subject)
	switch {
	case err == nil && !existing.UserID.Equal(st.LinkUserID):
		return s.errorURLFor(st, "identity_taken")
	case err == nil:
		// Already this user's identity; CompleteLink refreshes its email.
	default:
		if _, ok := errs.AsNotFound(err); !ok {
			logWarn(ctx, "oauth link: identity lookup", err, "provider", provider)
			return s.errorURLFor(st, "provider_error")
		}
		if _, gerr := s.identities.GetByUserProvider(ctx, st.LinkUserID, provider); gerr == nil {
			return s.errorURLFor(st, "provider_already_linked")
		}
	}
	code, cerr := oidc.RandomToken()
	if cerr != nil {
		return s.errorURLFor(st, "provider_error")
	}
	now := s.clock.Now()
	if ierr := s.handoffs.Insert(ctx, &model.OAuthHandoff{CodeHash: oidc.Sha256Hex(code), Kind: model.OAuthHandoffKindLink,
		UserID: st.LinkUserID, Provider: provider, Issuer: issuer, Subject: subject, Email: email,
		FlowHash: st.FlowHash, Generation: owner.CredentialsGeneration, CreatedAt: now, ExpiresAt: now.Add(model.OAuthHandoffTTL)}); ierr != nil {
		logWarn(ctx, "oauth link: handoff insert", ierr, "provider", provider)
		return s.errorURLFor(st, "provider_error")
	}
	reqctx.AddLogAttr(ctx, "user_id", st.LinkUserID.String())
	return s.linkPendingURL(st.Client, code)
}
