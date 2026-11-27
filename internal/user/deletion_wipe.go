package user

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type WipeOutcome string

const (
	WipeDone      WipeOutcome = "deleted"
	WipeNotMarked WipeOutcome = "not_marked" // cancelled meanwhile, or a different mark than selected
	WipeReleased  WipeOutcome = "released"   // inactive mark cancelled by the household rule
)

type deletedAuditDetails struct {
	Reason         model.DeletionReason `json:"reason"`
	LifetimeAccess bool                 `json:"lifetimeAccess"`
}

// Wipe deletes one marked user. selectedMark is the deletion_marked_at an age-based
// caller selected (nil for user:delete): a different value under the lock is WipeNotMarked.
func (s *Service) Wipe(ctx context.Context, userID vo.Id, selectedMark *time.Time) (WipeOutcome, error) {
	if s.deletion == nil || s.audit == nil {
		return "", errors.New("user deletion is not wired")
	}
	if s.wiper == nil {
		return "", errors.New("wipe is not wired")
	}
	var outcome WipeOutcome
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.deletion.LockWipes(ctx); err != nil {
			return err
		}
		if err := s.wiper.LockBudgets(ctx, userID); err != nil {
			return err
		}
		// FOR UPDATE, not LockRow: a request that authenticated before the mark
		// could otherwise still insert a row authored by the user into a
		// partner's account after Erase scanned it, and the cascade would
		// delete it with no correction.
		if err := s.deletion.LockRowForWipe(ctx, userID); err != nil {
			return err
		}
		st, err := s.deletion.GetState(ctx, userID)
		if err != nil {
			if _, ok := errs.AsNotFound(err); ok {
				outcome = WipeNotMarked
				return nil
			}
			return err
		}
		if st.MarkedAt == nil || (selectedMark != nil && !st.MarkedAt.Equal(*selectedMark)) {
			outcome = WipeNotMarked
			return nil
		}
		if st.Reason == model.DeletionReasonInactive {
			released, err := s.releaseInactiveMark(ctx, userID)
			if err != nil {
				return err
			}
			if released {
				outcome = WipeReleased
				return nil
			}
		}
		u, err := s.repo.GetByID(ctx, userID)
		if err != nil {
			return err
		}
		email, err := s.encode.Decode(u.Email)
		if err != nil {
			return err
		}
		lifetime := u.AccessLevel == model.AccessLevelFull && u.AccessUntil == nil
		if err := s.wiper.Erase(ctx, userID); err != nil {
			return err
		}
		n, err := s.deletion.DeleteUser(ctx, userID)
		if err != nil {
			return err
		}
		if n != 1 {
			return fmt.Errorf("wipe: deleted %d user rows, want 1", n)
		}
		var hash *string
		if lifetime {
			h := model.EmailHash(email)
			hash = &h
		}
		// Read last: wipes are serialized by LockWipes, so deleted rows become
		// visible in created_at order as long as the clock does not go back.
		now := s.clock.Now()
		if err := s.insertAudit(ctx, userID, model.AuditDeleted, deletedAuditDetails{Reason: st.Reason, LifetimeAccess: lifetime}, hash, now); err != nil {
			return err
		}
		outcome = WipeDone
		return nil
	})
	if err != nil {
		return "", err
	}
	return outcome, nil
}

// releaseInactiveMark applies the household rule under the lock. The
// connected users' rows are locked first, in id order: their sign-in and
// activity stamp write those rows, so they land before the check or wait for
// the wipe, never between the check and the delete.
func (s *Service) releaseInactiveMark(ctx context.Context, userID vo.Id) (bool, error) {
	connected, err := s.deletion.ConnectedUserIDs(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, id := range connected {
		if err := s.repo.LockRow(ctx, id); err != nil {
			return false, err
		}
	}
	allowed, err := s.inactiveMarkStillAllowed(ctx, userID)
	if err != nil || allowed {
		return false, err
	}
	if _, err := s.deletion.Unmark(ctx, userID); err != nil {
		return false, err
	}
	return true, s.writeDeletionAudit(ctx, userID, model.AuditDeletionCancelled,
		deletionAuditDetails{Reason: model.DeletionReasonInactive, Cause: model.CancelCauseActiveConnection}, s.clock.Now())
}

func (s *Service) inactiveMarkStillAllowed(ctx context.Context, userID vo.Id) (bool, error) {
	return true, nil
}
