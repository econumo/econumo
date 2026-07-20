package server

import (
	"context"
	"slices"

	"github.com/econumo/econumo/internal/infra/i18n"
	"github.com/econumo/econumo/internal/shared/vo"
)

// StoredLanguage implements middleware.StoredLanguageResolver on the wired
// authenticator decorator: the user's persisted UI language for requests that
// carried no (supported) Accept-Language header, mirroring the timezone
// fallback. Only supported tags are returned so an unexpected column value
// cannot leak into rendering; any lookup failure degrades to "" (header-or-en).
func (a *timezoneTrackingAuthenticator) StoredLanguage(ctx context.Context, userID vo.Id) string {
	lang, err := a.users.GetLanguage(ctx, userID)
	if err != nil || !slices.Contains(i18n.Supported, lang) {
		return ""
	}
	return lang
}
