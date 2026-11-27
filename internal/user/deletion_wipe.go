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

// AdminDelete is user:delete: wipes a marked user now, whatever the mark's age.
// WipeReleased is returned as an outcome, not an error: the caller reports it.
func (s *Service) AdminDelete(ctx context.Context, email string) (WipeOutcome, error) {
	if err := s.requireDeletionStores(); err != nil {
		return "", err
	}
	u, err := s.userByEmail(ctx, email)
	if err != nil {
		return "", err
	}
	outcome, err := s.Wipe(ctx, u.ID, nil)
	if err != nil {
		return "", err
	}
	if outcome == WipeNotMarked {
		return "", &errs.ValidationError{Msg: "This user is not marked for deletion.", MsgCode: errs.CodeUserDeletionNotMarked}
	}
	return outcome, nil
}

// ListPendingDeletion is user:list-pending-deletion, in ListMarked's order.
func (s *Service) ListPendingDeletion(ctx context.Context) ([]model.PendingDeletion, error) {
	if err := s.requireDeletionStores(); err != nil {
		return nil, err
	}
	marks, err := s.deletion.ListMarked(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]model.PendingDeletion, 0, len(marks))
	for _, m := range marks {
		email, err := s.encode.Decode(m.Email)
		if err != nil {
			return nil, err
		}
		p := model.PendingDeletion{ID: m.ID.String(), Email: email, Reason: string(m.Reason), MarkedAt: m.MarkedAt}
		if s.deletionCfg.DeletionDays > 0 {
			at := m.MarkedAt.Add(time.Duration(s.deletionCfg.DeletionDays) * 24 * time.Hour)
			p.WipeAt = &at
		}
		out = append(out, p)
	}
	return out, nil
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
	return true, s.unmarkInactive(ctx, userID)
}

// unmarkInactive cancels an inactive mark the household rule no longer allows.
func (s *Service) unmarkInactive(ctx context.Context, userID vo.Id) error {
	if _, err := s.deletion.Unmark(ctx, userID); err != nil {
		return err
	}
	cause := model.CancelCauseActiveConnection
	if s.deletionCfg.InactiveMonths == 0 {
		cause = model.CancelCauseInactivityOff
	}
	return s.writeDeletionAudit(ctx, userID, model.AuditDeletionCancelled,
		deletionAuditDetails{Reason: model.DeletionReasonInactive, Cause: cause}, s.clock.Now())
}

// inactiveMarkStillAllowed runs under the user's row lock. Partners count as
// active within the marked user's own period. With the policy off there is no
// window left to measure, so no inactive mark is allowed.
func (s *Service) inactiveMarkStillAllowed(ctx context.Context, userID vo.Id) (bool, error) {
	if s.deletionCfg.InactiveMonths == 0 {
		return false, nil
	}
	u, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return false, err
	}
	cutoff := s.inactiveCutoff(u.InactivityMonths(s.deletionCfg.InactiveMonths))
	active, err := s.deletion.HasActiveConnection(ctx, userID, cutoff)
	if err != nil {
		return false, err
	}
	return !active, nil
}

// maxInactivityMonths caps a stored period before it becomes a duration: the
// option is free text in the database, time.Duration overflows near 106,752
// days, and an overflowed cutoff would land in the future and mark everyone.
// It stays above every choosable period (twice the 600-month setting cap).
const maxInactivityMonths = 3500

// inactiveCutoff: the stamp has day precision, so the extra day keeps both
// errors on the side of keeping data.
func (s *Service) inactiveCutoff(months int) time.Time {
	months = min(months, maxInactivityMonths)
	return s.clock.Now().Add(-time.Duration(months*model.DaysPerInactivityMonth+1) * 24 * time.Hour)
}
