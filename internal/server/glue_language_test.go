package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
	"github.com/econumo/econumo/internal/test/mcptest"
	appuser "github.com/econumo/econumo/internal/user"
)

// runLanguageFallback runs the middleware over ctx and reports the language
// the wrapped handler observed via reqctx.Language.
func runLanguageFallback(t *testing.T, users *appuser.Service, ctx context.Context) string {
	t.Helper()
	var got string
	h := languageFallback(users)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = reqctx.Language(r.Context())
	}))
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil).WithContext(ctx)
	h.ServeHTTP(httptest.NewRecorder(), req)
	return got
}

func TestLanguageFallback(t *testing.T) {
	t.Run("non-explicit uses stored language", func(t *testing.T) {
		userSvc, f := timezoneFallbackFixture(t)
		userID := f.User(fixture.User{})
		uid, err := vo.ParseId(userID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := userSvc.UpdateLanguage(context.Background(), uid, model.UpdateLanguageRequest{Language: "ru"}); err != nil {
			t.Fatal(err)
		}
		ctx := mcptest.CtxWithUser(t, userID)
		if got := runLanguageFallback(t, userSvc, ctx); got != "ru" {
			t.Fatalf("language = %q, want ru", got)
		}
	})

	t.Run("explicit header wins over stored language", func(t *testing.T) {
		userSvc, f := timezoneFallbackFixture(t)
		userID := f.User(fixture.User{})
		uid, err := vo.ParseId(userID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := userSvc.UpdateLanguage(context.Background(), uid, model.UpdateLanguageRequest{Language: "ru"}); err != nil {
			t.Fatal(err)
		}
		ctx := reqctx.WithLanguage(mcptest.CtxWithUser(t, userID), "en")
		if got := runLanguageFallback(t, userSvc, ctx); got != "en" {
			t.Fatalf("language = %q, want en", got)
		}
	})

	t.Run("no stored language falls back to en", func(t *testing.T) {
		userSvc, f := timezoneFallbackFixture(t)
		userID := f.User(fixture.User{})
		ctx := mcptest.CtxWithUser(t, userID)
		if got := runLanguageFallback(t, userSvc, ctx); got != "en" {
			t.Fatalf("language = %q, want en", got)
		}
	})

	t.Run("no auth user in context defaults to en without panic", func(t *testing.T) {
		userSvc, _ := timezoneFallbackFixture(t)
		ctx := context.Background()
		if got := runLanguageFallback(t, userSvc, ctx); got != "en" {
			t.Fatalf("language = %q, want en", got)
		}
	})
}
