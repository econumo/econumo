package server

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
	appuser "github.com/econumo/econumo/internal/user"
)

func storedLanguageResolver(users *appuser.Service) *timezoneTrackingAuthenticator {
	return &timezoneTrackingAuthenticator{inner: nil, users: users}
}

func TestStoredLanguage(t *testing.T) {
	t.Run("returns the stored language", func(t *testing.T) {
		userSvc, f := timezoneFallbackFixture(t)
		userID := f.User(fixture.User{})
		uid, err := vo.ParseId(userID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := userSvc.UpdateLanguage(context.Background(), uid, model.UpdateLanguageRequest{Language: "ru"}); err != nil {
			t.Fatal(err)
		}
		if got := storedLanguageResolver(userSvc).StoredLanguage(context.Background(), uid); got != "ru" {
			t.Fatalf("StoredLanguage = %q, want ru", got)
		}
	})

	t.Run("unsupported or unknown stored value yields empty", func(t *testing.T) {
		userSvc, f := timezoneFallbackFixture(t)
		userID := f.User(fixture.User{})
		uid, err := vo.ParseId(userID)
		if err != nil {
			t.Fatal(err)
		}
		// default column value "en" is supported; force an unknown user instead
		unknown, err := vo.ParseId("00000000-0000-0000-0000-000000000001")
		if err != nil {
			t.Fatal(err)
		}
		if got := storedLanguageResolver(userSvc).StoredLanguage(context.Background(), unknown); got != "" {
			t.Fatalf("StoredLanguage(unknown user) = %q, want empty", got)
		}
		if got := storedLanguageResolver(userSvc).StoredLanguage(context.Background(), uid); got != "en" {
			t.Fatalf("StoredLanguage(default) = %q, want en", got)
		}
	})
}
