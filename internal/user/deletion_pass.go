package user

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type DeletionPassResult struct {
	Released, Wiped, Marked int
	Failed                  []PassFailure // wipe failures only
	Err                     error         // a pass that could not run at all (listing failed)
}

type PassFailure struct {
	UserID  vo.Id
	ErrType string
}

func goErrType(err error) string { return fmt.Sprintf("%T", err) }

func (s *Service) requireDeletionStores() error {
	if s.deletion == nil || s.audit == nil {
		return errors.New("user deletion is not wired")
	}
	return nil
}

// ReleaseInactiveMarks cancels inactive marks the household rule no longer allows,
// and all of them when InactiveDays is 0.
func (s *Service) ReleaseInactiveMarks(ctx context.Context) (int, error) {
	if err := s.requireDeletionStores(); err != nil {
		return 0, err
	}
	marks, err := s.deletion.ListMarked(ctx)
	if err != nil {
		return 0, err
	}
	released := 0
	for _, m := range marks {
		if m.Reason != model.DeletionReasonInactive {
			continue
		}
		ok, err := s.releaseListedMark(ctx, m)
		if err != nil {
			slog.Warn("deletion: release failed", "user_id", m.ID.String(), "err_type", goErrType(err))
			continue
		}
		if ok {
			released++
		}
	}
	return released, nil
}

// releaseListedMark locks only the user's own row: unlike the wipe, a stale
// check here can only keep a mark one pass longer, never lose data, and locking
// partners would take their rows in the opposite order to their own release.
func (s *Service) releaseListedMark(ctx context.Context, m model.MarkedUser) (bool, error) {
	released := false
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.repo.LockRow(ctx, m.ID); err != nil {
			return err
		}
		st, err := s.deletion.GetState(ctx, m.ID)
		if err != nil {
			if _, ok := errs.AsNotFound(err); ok {
				return nil
			}
			return err
		}
		// Only the mark that was listed: one made after it (a sign-in and a new
		// mark in between) is not this pass's to cancel.
		if st.MarkedAt == nil || st.Reason != model.DeletionReasonInactive || !st.MarkedAt.Equal(m.MarkedAt) {
			return nil
		}
		allowed, err := s.inactiveMarkStillAllowed(ctx, m.ID)
		if err != nil || allowed {
			return err
		}
		released = true
		return s.unmarkInactive(ctx, m.ID)
	})
	if err != nil {
		return false, err
	}
	return released, nil
}

// WipeExpired wipes users marked at least DeletionDays ago; nobody when DeletionDays is 0.
func (s *Service) WipeExpired(ctx context.Context) (wiped int, failed []PassFailure, err error) {
	if err := s.requireDeletionStores(); err != nil {
		return 0, nil, err
	}
	if s.deletionCfg.DeletionDays == 0 {
		return 0, nil, nil
	}
	marks, err := s.deletion.ListMarked(ctx)
	if err != nil {
		return 0, nil, err
	}
	due := s.clock.Now().Add(-time.Duration(s.deletionCfg.DeletionDays) * 24 * time.Hour)
	for _, m := range marks {
		if m.MarkedAt.After(due) {
			continue
		}
		markedAt := m.MarkedAt
		outcome, werr := s.Wipe(ctx, m.ID, &markedAt)
		if werr != nil {
			slog.Error("deletion: wipe failed", "user_id", m.ID.String(), "err_type", goErrType(werr))
			failed = append(failed, PassFailure{UserID: m.ID, ErrType: goErrType(werr)})
			continue
		}
		if outcome == WipeDone {
			wiped++
		}
	}
	return wiped, failed, nil
}

// MarkInactive emails and marks; nobody when InactiveDays is 0.
func (s *Service) MarkInactive(ctx context.Context) (int, error) {
	if err := s.requireDeletionStores(); err != nil {
		return 0, err
	}
	if s.deletionCfg.InactiveDays == 0 {
		return 0, nil
	}
	if s.deletionSender == nil {
		return 0, errors.New("user deletion sender is not wired")
	}
	cutoff := s.inactiveCutoff()
	candidates, err := s.deletion.ListInactiveCandidates(ctx, cutoff)
	if err != nil {
		return 0, err
	}
	marked := 0
	for _, c := range candidates {
		email, err := s.encode.Decode(c.Email)
		if err != nil {
			slog.Warn("deletion: inactivity mark failed", "user_id", c.ID.String(), "err_type", goErrType(err))
			continue
		}
		if s.deletionCfg.Exclude[model.NormalizeEmail(email)] {
			continue
		}
		// Nobody is marked without the notice going out.
		if err := s.deletionSender.SendInactivityNotice(ctx, c.Language, email, c.Name, s.deletionCfg.InactiveDays); err != nil {
			slog.Warn("deletion: inactivity notice failed", "user_id", c.ID.String(), "err_type", goErrType(err))
			continue
		}
		ok, err := s.markIfStillInactive(ctx, c, cutoff)
		if err != nil {
			slog.Warn("deletion: inactivity mark failed", "user_id", c.ID.String(), "err_type", goErrType(err))
			continue
		}
		if ok {
			marked++
		}
	}
	return marked, nil
}

// markIfStillInactive re-reads, under the row lock, everything the selection
// relied on: the notice took time, and a user who signed in (or moved to another
// address) meanwhile must not be marked.
func (s *Service) markIfStillInactive(ctx context.Context, c model.InactiveCandidate, cutoff time.Time) (bool, error) {
	marked := false
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.repo.LockRow(ctx, c.ID); err != nil {
			return err
		}
		u, err := s.repo.GetByID(ctx, c.ID)
		if err != nil {
			if _, ok := errs.AsNotFound(err); ok {
				return nil
			}
			return err
		}
		if !u.IsActive || u.Email != c.Email {
			return nil
		}
		st, err := s.deletion.GetState(ctx, c.ID)
		if err != nil {
			return err
		}
		stamp := st.CreatedAt
		if st.LastActiveAt != nil {
			stamp = *st.LastActiveAt
		}
		if st.MarkedAt != nil || !stamp.Before(cutoff) {
			return nil
		}
		active, err := s.deletion.HasActiveConnection(ctx, c.ID, cutoff)
		if err != nil || active {
			return err
		}
		if err := s.markForDeletion(ctx, c.ID, model.DeletionReasonInactive); err != nil {
			return err
		}
		marked = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return marked, nil
}

// PurgeDeleted = ReleaseInactiveMarks + WipeExpired (user:purge-deleted).
func (s *Service) PurgeDeleted(ctx context.Context) DeletionPassResult {
	var res DeletionPassResult
	res.Released, res.Err = s.ReleaseInactiveMarks(ctx)
	if res.Err != nil {
		return res
	}
	res.Wiped, res.Failed, res.Err = s.WipeExpired(ctx)
	return res
}
