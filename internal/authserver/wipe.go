package authserver

import (
	"context"
	"errors"

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
			// Only an app that is no longer the user's, or no longer exists, is
			// skipped. Any other error, a grantee's missing user row included, fails the
			// wipe: skipping the app would let the user-row cascade delete it
			// without revoking the other grantees' access tokens.
			if errors.Is(err, errAppOwnerChanged) || errors.Is(err, errAppGone) {
				continue
			}
			return err
		}
	}
	return nil
}
