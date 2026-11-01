package server

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/web/middleware"
)

// readScopeGate stops admitting already-issued read tokens once the share
// client is turned off, as disabling the OAuth server does for mcp tokens.
type readScopeGate struct {
	inner   middleware.TokenAuthenticator
	enabled bool
}

type readScopeGateWithLanguage struct {
	readScopeGate
	lang middleware.StoredLanguageResolver
}

func NewReadScopeGate(inner middleware.TokenAuthenticator, enabled bool) middleware.TokenAuthenticator {
	g := readScopeGate{inner: inner, enabled: enabled}
	if lr, ok := inner.(middleware.StoredLanguageResolver); ok {
		return &readScopeGateWithLanguage{readScopeGate: g, lang: lr}
	}
	return &g
}

func (g *readScopeGate) Authenticate(ctx context.Context, token string) (model.Principal, error) {
	p, err := g.inner.Authenticate(ctx, token)
	if err == nil && p.Scope == model.TokenScopeRead && !g.enabled {
		return model.Principal{}, errs.NewUnauthorized("Invalid access token")
	}
	return p, err
}

func (g *readScopeGateWithLanguage) StoredLanguage(ctx context.Context, userID vo.Id) string {
	return g.lang.StoredLanguage(ctx, userID)
}
