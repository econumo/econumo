package server

import (
	"net/http"
	"slices"

	"github.com/econumo/econumo/internal/infra/i18n"
	"github.com/econumo/econumo/internal/shared/reqctx"
	appuser "github.com/econumo/econumo/internal/user"
	"github.com/econumo/econumo/internal/web/middleware"
)

// languageFallback installs the stored UI language for requests that carried
// no (matching) Accept-Language header, mirroring timezoneFallback. Applied to
// /mcp only; REST keeps header-or-en with client-side translation.
func languageFallback(users *appuser.Service) middleware.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if !reqctx.IsLanguageExplicit(ctx) {
				if userID, ok := middleware.UserIDFromCtx(ctx); ok {
					if lang, err := users.GetLanguage(ctx, userID); err == nil && lang != "" && slices.Contains(i18n.Supported, lang) {
						ctx = reqctx.WithLanguage(ctx, lang)
						reqctx.AddLogAttr(ctx, "language", lang)
						r = r.WithContext(ctx)
					}
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
