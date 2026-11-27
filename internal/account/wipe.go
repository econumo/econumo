package account

import (
	"context"

	"github.com/econumo/econumo/internal/shared/vo"
)

// LockOwnedForWipe locks every account row the user owns, in id order
// (FOR UPDATE on PostgreSQL, nothing on SQLite). It runs on the caller's
// transaction context.
func (s *Service) LockOwnedForWipe(ctx context.Context, userID vo.Id) error {
	return s.accounts.LockByOwner(ctx, userID)
}

// UnwindGrantsOf unwinds every grant the user holds and every grant on the
// user's accounts. It runs on the caller's transaction context.
func (s *Service) UnwindGrantsOf(ctx context.Context, userID vo.Id) error {
	// ListReceived carries pending grants too, and ListIssued joins accounts
	// without a deletion filter, so soft-deleted accounts' grants go as well.
	received, err := s.access.ListReceived(ctx, userID)
	if err != nil {
		return err
	}
	issued, err := s.access.ListIssued(ctx, userID)
	if err != nil {
		return err
	}
	for _, g := range append(received, issued...) {
		if err := s.unwindGrant(ctx, g.AccountID, g.UserID); err != nil {
			return err
		}
	}
	return nil
}
