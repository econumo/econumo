package authserver

import (
	"context"
	"errors"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// DeleteAppsOwnedBy deletes every app the user owns the way DeleteApp does,
// revoking other users' grants and tokens on it. An app an operator made
// global, or deleted, after the listing is no longer the user's and is left
// alone.
func (s *Service) DeleteAppsOwnedBy(ctx context.Context, userID vo.Id) error {
	ids, err := s.repo.ListClientIDsByOwner(ctx, userID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := s.deleteAppWithRetries(ctx, id, &userID); err != nil {
			if errors.Is(err, errAppOwnerChanged) {
				continue
			}
			if _, ok := errs.AsNotFound(err); ok {
				continue
			}
			return err
		}
	}
	return nil
}
