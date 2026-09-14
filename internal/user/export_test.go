package user

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
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
