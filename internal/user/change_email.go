// Self-service change-email use cases. The flow is fully authenticated (keyed
// by user id), verifies the NEW address with an emailed code, gates the request
// on the current password, and notifies the OLD address. On confirm it revokes
// every OTHER session (the presenting one survives); PATs are untouched.
package user

import (
	"context"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// RequestEmailChange verifies the current password, checks the new email is
// free and different, stores a pending change (replacing any prior), emails the
// code to the new address, and notifies the old address.
func (s *Service) RequestEmailChange(ctx context.Context, userID vo.Id, req model.RequestEmailChangeRequest) (*model.RequestEmailChangeResult, error) {
	key := userID.String()
	if err := s.allowAttempt(RateScopeRequestEmailChange, key); err != nil {
		return nil, err
	}
	s.failAttempt(RateScopeRequestEmailChange, key) // every send counts toward the cap

	u, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !s.hasher.Verify(u.Algorithm, u.Password, req.Password, u.Salt) {
		return nil, &errs.ValidationError{Msg: "Password is not correct", MsgCode: errs.CodeUserPasswordIncorrect}
	}

	newEmail := strings.TrimSpace(req.NewEmail)
	currentEmail, derr := s.encode.Decode(u.Email)
	if derr != nil {
		return nil, derr
	}
	if strings.EqualFold(newEmail, strings.TrimSpace(currentEmail)) {
		return nil, &errs.ValidationError{Msg: "The new email is the same as your current email.", MsgCode: errs.CodeUserEmailUnchanged}
	}
	exists, err := s.repo.ExistsByEmail(ctx, newEmail)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, &errs.ValidationError{Msg: "User already exists", MsgCode: errs.CodeUserAlreadyExists}
	}

	now := s.clock.Now()
	if err := s.issueEmailChangeCode(ctx, u, newEmail, now); err != nil {
		return nil, err
	}
	s.markEmailChangeSent(key)
	if s.changeMailer != nil {
		if nerr := s.changeMailer.SendEmailChangeNotice(ctx, strings.TrimSpace(currentEmail), u.Name, newEmail,
			s.linkedEmails(ctx, userID)); nerr != nil {
			return nil, nerr
		}
	}
	return &model.RequestEmailChangeResult{}, nil
}

// ConfirmEmailChange validates the code, then under the user's row lock
// consumes the pending request — the grant the new email rests on — and commits
// the address, marking it verified, before revoking the other sessions. A
// missing/wrong/expired code is a generic invalid-code error (anti-enumeration,
// though this is authenticated); failed attempts count toward the cap.
func (s *Service) ConfirmEmailChange(ctx context.Context, userID, currentTokenID vo.Id, req model.ConfirmEmailChangeRequest) (model.CurrentUserResult, error) {
	key := userID.String()
	var empty model.CurrentUserResult
	if err := s.allowAttempt(RateScopeConfirmEmailChange, key); err != nil {
		return empty, err
	}
	invalid := &errs.ValidationError{Msg: "The confirmation code is not valid.", MsgCode: errs.CodeUserVerificationCodeInvalid}

	cr, err := s.emailChangeRequests.GetByUser(ctx, userID)
	if err != nil {
		if isNotFound(err) {
			s.failAttempt(RateScopeConfirmEmailChange, key)
			return empty, invalid
		}
		return empty, err
	}
	if HashResetCode(strings.TrimSpace(req.Code)) != cr.Code {
		s.failAttempt(RateScopeConfirmEmailChange, key)
		return empty, invalid
	}
	now := s.clock.Now()
	if cr.IsExpired(now) {
		s.failAttempt(RateScopeConfirmEmailChange, key)
		return empty, &errs.ValidationError{Msg: "The code is expired", MsgCode: errs.CodeUserVerificationCodeExpired}
	}
	encrypted, eerr := s.encode.Encode(cr.NewEmail)
	if eerr != nil {
		return empty, eerr
	}
	var updated *model.User
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		// The users row first, then the grant row: the same order the reclaim
		// takes (see reclaimCredentials), so the two can only serialize, never
		// deadlock.
		if lerr := s.repo.LockRow(ctx, userID); lerr != nil {
			return lerr
		}
		// The pending row is this confirmation's authority, so consuming it is
		// what proves the authority was still there: a reclaim (or a concurrent
		// confirm) that already took it leaves nothing to delete.
		taken, cerr := s.emailChangeRequests.Consume(ctx, cr.ID, userID)
		if cerr != nil {
			return cerr
		}
		if taken != 1 {
			return invalid
		}
		u, gerr := s.repo.GetByID(ctx, userID)
		if gerr != nil {
			return gerr
		}
		// Commit-time race guard: the target could have been taken since the
		// request. Inside the transaction, so the check and the write agree.
		exists, xerr := s.repo.ExistsByEmail(ctx, cr.NewEmail)
		if xerr != nil {
			return xerr
		}
		if exists {
			return &errs.ValidationError{Msg: "User already exists", MsgCode: errs.CodeUserAlreadyExists}
		}
		// Only the email columns, under the generation read after the lock: a
		// full aggregate Save would carry the pre-reset password back with it.
		rows, uerr := s.repo.ReplaceEmailIfGeneration(ctx, userID, encrypted, now, u.CredentialsGeneration)
		if uerr != nil {
			return uerr
		}
		if rows != 1 {
			return invalid
		}
		u.UpdateEmail(encrypted, now)
		u.MarkEmailVerified(now)
		updated = u
		return nil
	}); err != nil {
		return empty, err
	}
	s.clearAttempt(RateScopeConfirmEmailChange, key)
	if err := s.revokeSessions(ctx, userID, currentTokenID, now); err != nil {
		return empty, err
	}
	return s.toCurrentUser(ctx, updated)
}

// ResendEmailChangeCode re-sends the code to the pending new address, at most
// once per resend gap. Silent no-op if there is no pending change. Returns the
// seconds-until-next as a Duration (the edge emits Retry-After).
func (s *Service) ResendEmailChangeCode(ctx context.Context, userID vo.Id) (*model.ResendEmailChangeCodeResult, time.Duration, error) {
	key := userID.String()
	now := s.clock.Now()
	wait := s.emailChangeSentCooldown(key, now)

	if err := s.allowAttempt(RateScopeRequestEmailChange, key); err != nil {
		return nil, 0, err
	}
	s.failAttempt(RateScopeRequestEmailChange, key)

	result := &model.ResendEmailChangeCodeResult{}
	fullGap := model.EmailVerificationResendGap
	if wait > 0 {
		return result, wait, nil
	}
	s.markEmailChangeSent(key)

	cr, err := s.emailChangeRequests.GetByUser(ctx, userID)
	if err != nil {
		if isNotFound(err) {
			return result, fullGap, nil // no pending change: silent no-op
		}
		return nil, 0, err
	}
	u, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return nil, 0, err
	}
	if err := s.issueEmailChangeCode(ctx, u, cr.NewEmail, now); err != nil {
		return nil, 0, err
	}
	return result, fullGap, nil
}

// issueEmailChangeCode generates a fresh code, replaces any pending row for the
// user (preserving newEmail), and emails the code to the new address. It does
// NOT rate-limit — callers own that. The insert is fenced on u's credentials
// generation — the one read together with the password the caller verified —
// so a reset committing in between refuses the grant rather than handing the
// old session a way to rewrite the recovered account's login key. The users row
// is locked first (the same order the reclaim takes, so the two serialize and
// cannot deadlock) because the fence alone is not enough on PostgreSQL: under
// READ COMMITTED a reclaim whose users UPDATE has not committed yet is
// invisible to the EXISTS check, so an unfenced insert could still land after
// the reclaim swept the pending rows.
func (s *Service) issueEmailChangeCode(ctx context.Context, u *model.User, newEmail string, now time.Time) error {
	code, err := generatePasswordCode()
	if err != nil {
		return err
	}
	cr := model.NewEmailChangeRequest(vo.NewId(), u.ID, newEmail, HashResetCode(code), now)
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		if lerr := s.repo.LockRow(ctx, u.ID); lerr != nil {
			return lerr
		}
		if derr := s.emailChangeRequests.DeleteByUser(ctx, u.ID); derr != nil {
			return derr
		}
		rows, serr := s.emailChangeRequests.Save(ctx, cr, u.CredentialsGeneration)
		if serr != nil {
			return serr
		}
		if rows != 1 {
			return errs.NewUnauthorized("Invalid access token")
		}
		return nil
	}); err != nil {
		return err
	}
	if s.changeMailer != nil {
		return s.changeMailer.SendEmailChangeCode(ctx, newEmail, u.Name, code)
	}
	return nil
}

func (s *Service) markEmailChangeSent(key string) {
	if s.limiter != nil {
		s.limiter.Mark(RateScopeEmailChangeSent, key)
	}
}

func (s *Service) emailChangeSentCooldown(key string, now time.Time) time.Duration {
	if s.limiter == nil {
		return 0
	}
	last, ok := s.limiter.LastAttempt(RateScopeEmailChangeSent, key)
	if !ok {
		return 0
	}
	remaining := last.Add(model.EmailVerificationResendGap).Sub(now)
	if remaining <= 0 {
		return 0
	}
	if remaining > model.EmailVerificationResendGap {
		return model.EmailVerificationResendGap
	}
	if rem := remaining % time.Second; rem > 0 {
		remaining += time.Second - rem
	}
	return remaining
}

// linkedEmails resolves the addresses the user's providers vouched for. The
// copy list is a nicety, so a lookup failure yields none rather than failing
// the notice the user is waiting on.
func (s *Service) linkedEmails(ctx context.Context, userID vo.Id) []string {
	if s.identityEmails == nil {
		return nil
	}
	addrs, err := s.identityEmails.ListEmails(ctx, userID)
	if err != nil {
		return nil
	}
	return addrs
}
