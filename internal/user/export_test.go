package user

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// HashForTest exposes the Service's password hasher to the black-box test
// suite (login_test.go), which needs a hash produced by the same algorithm
// the service's own reset/rehash paths use.
func (s *Service) HashForTest(plaintext string) (string, error) {
	return s.hasher.Hash(plaintext)
}

// RehashLegacyPasswordForTest exposes rehashLegacyPassword so the reclaim-race
// test can drive it directly against a hand-built stale aggregate, without
// going through the full Login use case.
func (s *Service) RehashLegacyPasswordForTest(ctx context.Context, u *model.User, plaintext string, now time.Time) {
	s.rehashLegacyPassword(ctx, u, plaintext, now)
}

// RevokeTokensForTest exposes revokeTokens so a black-box test can simulate a
// reclaim landing between authentication and a write, without going through a
// full password-reset or deactivate use case.
func (s *Service) RevokeTokensForTest(ctx context.Context, userID vo.Id, exceptTokenID vo.Id, now time.Time, kinds ...string) error {
	return s.revokeTokens(ctx, userID, exceptTokenID, now, kinds...)
}
