package user

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// PersistTimezone stores a caller-observed IANA timezone. It is called from
// the request hot path, so invalid names are dropped silently rather than
// surfaced: a bad header must never fail an otherwise-valid request.
func (s *Service) PersistTimezone(ctx context.Context, userID vo.Id, tz string) error {
	if tz == "" || tz == "Local" {
		return nil
	}
	if _, err := time.LoadLocation(tz); err != nil {
		return nil
	}
	return s.repo.UpdateTimezone(ctx, userID, tz)
}

func (s *Service) GetTimezone(ctx context.Context, userID vo.Id) (string, error) {
	return s.repo.GetTimezone(ctx, userID)
}
