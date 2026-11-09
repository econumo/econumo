package server

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/web/middleware"
)

type scopeAuthenticator struct{ scope model.TokenScope }

func (a scopeAuthenticator) Authenticate(context.Context, string) (model.Principal, error) {
	return model.Principal{UserID: vo.NewId(), TokenID: vo.NewId(), Level: model.AccessLevelFull, Scope: a.scope}, nil
}

type languageAuthenticator struct {
	scopeAuthenticator
	asked vo.Id
}

func (a *languageAuthenticator) StoredLanguage(_ context.Context, userID vo.Id) string {
	a.asked = userID
	return "ru"
}

type idempotencyAuthenticator struct {
	scopeAuthenticator
	store middleware.IdempotencyStore
}

func (a idempotencyAuthenticator) IdempotencyStore() middleware.IdempotencyStore { return a.store }

func TestReadScopeGateForwardsIdempotencyStore(t *testing.T) {
	store := &nopIdempotencyStore{}
	gate, ok := NewReadScopeGate(idempotencyAuthenticator{store: store}, true).(middleware.IdempotencyProvider)
	if !ok || gate.IdempotencyStore() != middleware.IdempotencyStore(store) {
		t.Fatal("the gate must forward the inner's idempotency store")
	}
	bare, ok := NewReadScopeGate(scopeAuthenticator{}, true).(middleware.IdempotencyProvider)
	if !ok || bare.IdempotencyStore() != nil {
		t.Fatal("without an inner provider the gate must report no store")
	}
}

type nopIdempotencyStore struct{}

func (*nopIdempotencyStore) Find(context.Context, vo.Id, string) (string, bool, error) {
	return "", false, nil
}
func (*nopIdempotencyStore) Insert(context.Context, vo.Id, string, string) error { return nil }
func (*nopIdempotencyStore) Delete(context.Context, vo.Id, string) error         { return nil }
func (*nopIdempotencyStore) MaybePurge(context.Context)                          {}

func TestReadScopeGate(t *testing.T) {
	ctx := context.Background()
	for _, scope := range []model.TokenScope{model.TokenScopeFull, model.TokenScopeMCP, model.TokenScopeIngest} {
		p, err := NewReadScopeGate(scopeAuthenticator{scope}, false).Authenticate(ctx, "t")
		if err != nil || p.Scope != scope {
			t.Fatalf("disabled gate on %s: %+v %v", scope, p, err)
		}
	}
	_, err := NewReadScopeGate(scopeAuthenticator{model.TokenScopeRead}, false).Authenticate(ctx, "t")
	u, ok := errs.AsUnauthorized(err)
	if !ok || u.Error() != "Invalid access token" {
		t.Fatalf("disabled gate must refuse read: %v", err)
	}
	p, err := NewReadScopeGate(scopeAuthenticator{model.TokenScopeRead}, true).Authenticate(ctx, "t")
	if err != nil || p.Scope != model.TokenScopeRead {
		t.Fatalf("enabled gate must pass read: %+v %v", p, err)
	}

	if _, ok := NewReadScopeGate(scopeAuthenticator{}, true).(middleware.StoredLanguageResolver); ok {
		t.Fatal("the gate must not claim a language capability its inner lacks")
	}
	inner := &languageAuthenticator{}
	lr, ok := NewReadScopeGate(inner, true).(middleware.StoredLanguageResolver)
	if !ok {
		t.Fatal("the gate must forward the inner's language capability")
	}
	uid := vo.NewId()
	if got := lr.StoredLanguage(ctx, uid); got != "ru" || !inner.asked.Equal(uid) {
		t.Fatalf("StoredLanguage = %q, asked %v", got, inner.asked)
	}
}
