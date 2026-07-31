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
	if err := s.issueEmailChangeCode(ctx, u.ID, newEmail, u.Name, now); err != nil {
		return nil, err
	}
	s.markEmailChangeSent(key)
	if s.changeMailer != nil {
		if nerr := s.changeMailer.SendEmailChangeNotice(ctx, strings.TrimSpace(currentEmail), u.Name, newEmail); nerr != nil {
			return nil, nerr
		}
	}
	return &model.RequestEmailChangeResult{}, nil
}

// ConfirmEmailChange validates the code, commits the new email, marks it
// verified, deletes the pending row, and revokes other sessions. A
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
	// Commit-time race guard: the target could have been taken since the request.
	exists, eerr := s.repo.ExistsByEmail(ctx, cr.NewEmail)
	if eerr != nil {
		return empty, eerr
	}
	if exists {
		return empty, &errs.ValidationError{Msg: "User already exists", MsgCode: errs.CodeUserAlreadyExists}
	}

	encrypted, eerr := s.encode.Encode(cr.NewEmail)
	if eerr != nil {
		return empty, eerr
	}
	var updated *model.User
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		u, gerr := s.repo.GetByID(ctx, userID)
		if gerr != nil {
			return gerr
		}
		u.UpdateEmail(encrypted, now)
		u.MarkEmailVerified(now)
		if serr := s.repo.Save(ctx, u); serr != nil {
			return serr
		}
		if derr := s.emailChangeRequests.DeleteByUser(ctx, userID); derr != nil {
			return derr
		}
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
	if err := s.issueEmailChangeCode(ctx, userID, cr.NewEmail, u.Name, now); err != nil {
		return nil, 0, err
	}
	return result, fullGap, nil
}

// issueEmailChangeCode generates a fresh code, replaces any pending row for the
// user (preserving newEmail), and emails the code to the new address. It does
// NOT rate-limit — callers own that.
func (s *Service) issueEmailChangeCode(ctx context.Context, userID vo.Id, newEmail, name string, now time.Time) error {
	code, err := generatePasswordCode()
	if err != nil {
		return err
	}
	cr := model.NewEmailChangeRequest(vo.NewId(), userID, newEmail, HashResetCode(code), now)
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		if derr := s.emailChangeRequests.DeleteByUser(ctx, userID); derr != nil {
			return derr
		}
		return s.emailChangeRequests.Save(ctx, cr)
	}); err != nil {
		return err
	}
	if s.changeMailer != nil {
		return s.changeMailer.SendEmailChangeCode(ctx, newEmail, name, code)
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
