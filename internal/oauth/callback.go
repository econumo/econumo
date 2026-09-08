package oauth

import (
	"context"
	"encoding/json"
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
		// The intent (and the client) live in the state row, so an unknown or
		// expired state can only be reported on the web login page.
		return s.errorURL(model.OAuthClientWeb, "invalid_state")
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

	if st.Intent == model.OAuthIntentLink {
		return s.link(ctx, st, provider, claims.Subject, email)
	}
	return s.login(ctx, st, provider, claims, email, toks.IDToken)
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

func (s *Service) login(ctx context.Context, st *model.OAuthState, provider string, claims oidc.Claims, email, idToken string) string {
	now := s.clock.Now()
	var tokenForSession *string
	if provider == model.OAuthProviderOIDC {
		t := idToken
		tokenForSession = &t
	}

	// Step 5: existing identity.
	id, err := s.identities.GetByProviderSubject(ctx, provider, claims.Subject)
	if err == nil {
		u, uerr := s.users.FindByID(ctx, id.UserID)
		if uerr != nil {
			logWarn(ctx, "oauth callback: identity owner", uerr, "provider", provider)
			return s.errorURLFor(st, "provider_error")
		}
		if !u.IsActive {
			return s.errorURLFor(st, "account_inactive")
		}
		id.UpdateEmail(email, now)
		if serr := s.identities.Save(ctx, id); serr != nil {
			logWarn(ctx, "oauth callback: identity save", serr, "provider", provider)
			return s.errorURLFor(st, "provider_error")
		}
		s.mirrorEmailDrift(ctx, u, email, provider)
		return s.mintHandoff(ctx, st, u.ID, provider, tokenForSession)
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
		if serr := s.autoLink(ctx, u, provider, claims.Subject, email, now); serr != nil {
			logWarn(ctx, "oauth callback: auto-link", serr, "provider", provider)
			return s.errorURLFor(st, "provider_error")
		}
		reqctx.AddLogAttr(ctx, "oauth_linked", true)
		return s.mintHandoff(ctx, st, u.ID, provider, tokenForSession)
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
	if serr := s.identities.Save(ctx, model.NewIdentity(s.identities.NextIdentity(), u.ID, provider, claims.Subject, email, now)); serr != nil {
		logWarn(ctx, "oauth callback: identity insert", serr, "provider", provider)
		return s.errorURLFor(st, "provider_error")
	}
	reqctx.AddLogAttr(ctx, "oauth_provisioned", true)
	return s.mintHandoff(ctx, st, u.ID, provider, tokenForSession)
}

// autoLink attaches the provider identity to an account found by email. When
// that account has a password, whoever set it never had to prove they own the
// address (registration does not always verify), so the sign-in is treated
// like a password reset: every session of the account is revoked, and the
// provider's assertion marks the address verified. A passwordless account was
// created through a provider, so its owner already proved the address and
// keeps their sessions. The three writes share one transaction: a half-applied
// link would leave the eviction undone while step 5 signs the attacker in.
func (s *Service) autoLink(ctx context.Context, u *model.User, provider, subject, email string, now time.Time) error {
	return s.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.identities.Save(ctx, model.NewIdentity(s.identities.NextIdentity(), u.ID, provider, subject, email, now)); err != nil {
			return err
		}
		if !u.HasPassword() {
			return nil
		}
		if err := s.users.RevokeAllSessions(ctx, u.ID); err != nil {
			return err
		}
		return s.users.MarkEmailVerified(ctx, u.ID)
	})
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

func (s *Service) mintHandoff(ctx context.Context, st *model.OAuthState, userID vo.Id, provider string, idToken *string) string {
	code, err := oidc.RandomToken()
	if err != nil {
		return s.errorURLFor(st, "provider_error")
	}
	now := s.clock.Now()
	if err := s.handoffs.Insert(ctx, &model.OAuthHandoff{CodeHash: oidc.Sha256Hex(code), UserID: userID, Provider: provider,
		FlowHash: st.FlowHash, IDToken: idToken, CreatedAt: now, ExpiresAt: now.Add(model.OAuthHandoffTTL)}); err != nil {
		logWarn(ctx, "oauth callback: handoff insert", err, "provider", provider)
		return s.errorURLFor(st, "provider_error")
	}
	reqctx.AddLogAttr(ctx, "user_id", userID.String())
	return s.successURL(st.Client, code)
}

func (s *Service) link(ctx context.Context, st *model.OAuthState, provider, subject, email string) string {
	now := s.clock.Now()
	existing, err := s.identities.GetByProviderSubject(ctx, provider, subject)
	switch {
	case err == nil && !existing.UserID.Equal(st.LinkUserID):
		return s.errorURLFor(st, "identity_taken")
	case err == nil:
		existing.UpdateEmail(email, now)
		if serr := s.identities.Save(ctx, existing); serr != nil {
			return s.errorURLFor(st, "provider_error")
		}
		return s.linkedURL(st.Client, provider)
	}
	if _, ok := errs.AsNotFound(err); !ok {
		logWarn(ctx, "oauth link: identity lookup", err, "provider", provider)
		return s.errorURLFor(st, "provider_error")
	}
	if _, gerr := s.identities.GetByUserProvider(ctx, st.LinkUserID, provider); gerr == nil {
		return s.errorURLFor(st, "provider_already_linked")
	}
	if serr := s.identities.Save(ctx, model.NewIdentity(s.identities.NextIdentity(), st.LinkUserID, provider, subject, email, now)); serr != nil {
		logWarn(ctx, "oauth link: identity insert", serr, "provider", provider)
		return s.errorURLFor(st, "provider_error")
	}
	reqctx.AddLogAttr(ctx, "user_id", st.LinkUserID.String())
	return s.linkedURL(st.Client, provider)
}
