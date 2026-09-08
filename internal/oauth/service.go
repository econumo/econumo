package oauth

import (
	"context"
	"log/slog"
	"net/url"
	"strings"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/port"
)

type Service struct {
	providers         []Provider
	byID              map[string]Provider
	users             Users
	identities        Identities
	states            States
	handoffs          Handoffs
	tx                port.TxRunner
	clock             port.Clock
	limiter           AttemptLimiter
	appURL            string
	allowRegistration bool
}

func NewService(providers []Provider, users Users, identities Identities, states States, handoffs Handoffs,
	tx port.TxRunner, clock port.Clock, limiter AttemptLimiter, appURL string, allowRegistration bool) *Service {
	byID := map[string]Provider{}
	for _, p := range providers {
		byID[p.Client.Issuer().ID] = p
	}
	return &Service{providers: providers, byID: byID, users: users, identities: identities, states: states,
		handoffs: handoffs, tx: tx, clock: clock, limiter: limiter, appURL: strings.TrimSuffix(appURL, "/"),
		allowRegistration: allowRegistration}
}

// allowStart guards the optional limiter, mirroring the user feature's pattern.
func (s *Service) allowStart() error {
	if s.limiter == nil {
		return nil
	}
	return s.limiter.Allow(RateScopeOAuthStart, "")
}

func (s *Service) ListProviders() []model.ProviderItem {
	out := make([]model.ProviderItem, 0, len(s.providers))
	for _, p := range s.providers {
		out = append(out, model.ProviderItem{Id: p.Client.Issuer().ID, Name: p.Name})
	}
	return out
}

func (s *Service) provider(id string) (Provider, error) {
	p, ok := s.byID[id]
	if !ok {
		return Provider{}, &errs.ValidationError{Msg: "Sign-in provider is not configured", MsgCode: errs.CodeOAuthProviderNotConfigured}
	}
	return p, nil
}

// RedirectURI is the one URL an operator registers per provider.
func (s *Service) RedirectURI(provider string) string {
	return s.appURL + "/api/v1/oauth/callback-" + provider
}

// EndSessionURL implements the user feature's LogoutURLBuilder port.
func (s *Service) EndSessionURL(ctx context.Context, provider, idToken string) (string, error) {
	p, ok := s.byID[provider]
	if !ok {
		return "", nil
	}
	return p.Client.EndSessionURL(ctx, idToken, s.appURL+"/login")
}

// redirect targets (spec §6.3)

func (s *Service) successURL(client, handoff string) string {
	if client == model.OAuthClientApp {
		return "econumo://oauth?handoff=" + url.QueryEscape(handoff)
	}
	return s.appURL + "/oauth/callback#handoff=" + url.QueryEscape(handoff)
}

func (s *Service) linkedURL(client, provider string) string {
	if client == model.OAuthClientApp {
		return "econumo://oauth?linked=" + url.QueryEscape(provider)
	}
	return s.appURL + "/settings/profile/linked-accounts?linked=" + url.QueryEscape(provider)
}

// errorURLFor reports the failure on the surface the flow started from: a link
// started in Settings lands back there, not on the login page (where a
// signed-in user would see nothing).
func (s *Service) errorURLFor(st *model.OAuthState, code string) string {
	if st.Intent != model.OAuthIntentLink {
		return s.errorURL(st.Client, code)
	}
	if st.Client == model.OAuthClientApp {
		return "econumo://oauth?linkError=" + url.QueryEscape(code)
	}
	return s.appURL + "/settings/profile/linked-accounts?oauthError=" + url.QueryEscape(code)
}

func (s *Service) errorURL(client, code string) string {
	if client == model.OAuthClientApp {
		return "econumo://oauth?error=" + url.QueryEscape(code)
	}
	return s.appURL + "/login?oauthError=" + url.QueryEscape(code)
}

func logWarn(ctx context.Context, msg string, err error, attrs ...any) {
	slog.WarnContext(ctx, msg, append([]any{"err", err.Error()}, attrs...)...)
}
