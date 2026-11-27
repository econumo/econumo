package user

import (
	"context"
	"encoding/json"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type deletionAuditDetails struct {
	Reason model.DeletionReason `json:"reason"`
	Cause  string               `json:"cause,omitempty"`
}

// markForDeletion runs inside the caller's transaction, after LockRow.
func (s *Service) markForDeletion(ctx context.Context, userID vo.Id, reason model.DeletionReason) error {
	if s.deletion == nil {
		return nil
	}
	now := s.clock.Now()
	n, err := s.deletion.Mark(ctx, userID, reason, now)
	if err != nil {
		return err
	}
	if n == 0 {
		return &errs.ValidationError{Msg: "This user is already marked for deletion.", MsgCode: errs.CodeUserDeletionAlreadyMarked}
	}
	// A marked account has no way in until a new sign-in cancels the mark; the
	// bump fences any sign-in that read its evidence before this point.
	if err := s.repo.BumpCredentialsGeneration(ctx, userID); err != nil {
		return err
	}
	if err := s.revokeTokens(ctx, userID, vo.Id{}, now, model.TokenKindSession, model.TokenKindPersonal, model.TokenKindOAuth); err != nil {
		return err
	}
	if err := s.revokeMCPGrants(ctx, userID); err != nil {
		return err
	}
	if err := s.deleteDeletionRequests(ctx, userID); err != nil {
		return err
	}
	return s.writeDeletionAudit(ctx, userID, model.AuditDeletionMarked, deletionAuditDetails{Reason: reason}, now)
}

// AdminMarkDeleted is the CLI's mark, looked up by email.
func (s *Service) AdminMarkDeleted(ctx context.Context, email string) error {
	_, err := s.mutateByEmail(ctx, email, func(ctx context.Context, u *model.User) error {
		return s.markForDeletion(ctx, u.ID, model.DeletionReasonCLI)
	})
	return err
}

// cancelMarkOnSession runs in the session mint's transaction, under the row
// lock, once the fenced insert has written the session.
func (s *Service) cancelMarkOnSession(ctx context.Context, userID vo.Id, now time.Time) error {
	if s.deletion == nil {
		return nil
	}
	st, err := s.deletion.GetState(ctx, userID)
	if err != nil {
		return err
	}
	if st.MarkedAt == nil {
		return nil
	}
	// Unmark blanks the reason, so it is read first for the audit row.
	reason := st.Reason
	if _, err := s.deletion.Unmark(ctx, userID); err != nil {
		return err
	}
	return s.writeDeletionAudit(ctx, userID, model.AuditDeletionCancelled, deletionAuditDetails{Reason: reason, Cause: model.CancelCauseSignIn}, now)
}

func (s *Service) stampActive(ctx context.Context, userID vo.Id, now time.Time) error {
	if s.deletion == nil {
		return nil
	}
	return s.deletion.TouchLastActive(ctx, userID, now)
}

// deleteDeletionRequests drops the user's pending deletion code: it is a grant
// issued under the current credential and must not outlive it.
func (s *Service) deleteDeletionRequests(ctx context.Context, userID vo.Id) error {
	if s.deletionRequests == nil {
		return nil
	}
	return s.deletionRequests.DeleteByUser(ctx, userID)
}

func (s *Service) writeDeletionAudit(ctx context.Context, userID vo.Id, event string, details deletionAuditDetails, now time.Time) error {
	raw, err := json.Marshal(details)
	if err != nil {
		return err
	}
	_, err = s.audit.Insert(ctx, model.UserAuditEvent{
		ID: vo.NewId(), UserID: userID, Event: event, Details: string(raw), CreatedAt: now,
	})
	return err
}
