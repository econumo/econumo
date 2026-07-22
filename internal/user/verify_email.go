// Email-verification-at-login use case (ECONUMO_EMAIL_VERIFICATION): an
// unverified user with correct credentials must present a code that was
// emailed to them. The code is only ever processed AFTER the password check,
// so this surface cannot be probed without valid credentials.
package user

import (
	"context"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// verifyEmailOnLogin gates an unverified user's login. Without a code it
// ensures an outstanding verification email exists (sending one when none is
// live, or always on resend) and denies with email_verification_required.
// With a code it verifies and consumes it, letting the login proceed.
func (s *Service) verifyEmailOnLogin(ctx context.Context, u *model.User, email string, req model.LoginRequest, limitKey string) error {
	now := s.clock.Now()
	code := strings.TrimSpace(req.Code)
	if code == "" {
		if err := s.ensureVerificationCode(ctx, u, email, req.Resend, limitKey, now); err != nil {
			return err
		}
		return &errs.AccessDeniedError{Msg: "Please verify your email address.", Code: errs.CodeUserEmailVerificationRequired}
	}

	invalid := &errs.AccessDeniedError{Msg: "The confirmation code is not valid.", Code: errs.CodeUserVerificationCodeInvalid}
	ev, err := s.emailVerifications.GetByUser(ctx, u.ID)
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			s.failAttempt(RateScopeLogin, limitKey)
			return invalid
		}
		return err
	}
	if HashResetCode(code) != ev.Code || ev.IsExpired(now) {
		s.failAttempt(RateScopeLogin, limitKey)
		return invalid
	}
	return s.tx.WithTx(ctx, func(ctx context.Context) error {
		u.MarkEmailVerified(s.clock.Now())
		if err := s.repo.Save(ctx, u); err != nil {
			return err
		}
		return s.emailVerifications.DeleteByUser(ctx, u.ID)
	})
}

// ensureVerificationCode sends a fresh code when none is outstanding, the
// outstanding one expired, or the caller forced a resend. Every send counts
// toward the verify-email rate cap; a live code is otherwise reused silently
// so repeated code-less logins cannot spam the mailbox.
func (s *Service) ensureVerificationCode(ctx context.Context, u *model.User, email string, force bool, limitKey string, now time.Time) error {
	send := force
	if !send {
		ev, err := s.emailVerifications.GetByUser(ctx, u.ID)
		switch {
		case err != nil:
			if _, ok := errs.AsNotFound(err); !ok {
				return err
			}
			send = true
		case ev.IsExpired(now):
			send = true
		}
	}
	if !send {
		return nil
	}
	if err := s.allowAttempt(RateScopeVerifyEmail, limitKey); err != nil {
		return err
	}
	s.failAttempt(RateScopeVerifyEmail, limitKey)
	code, err := generatePasswordCode()
	if err != nil {
		return err
	}
	ev := model.NewEmailVerification(vo.NewId(), u.ID, HashResetCode(code), now)
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		if derr := s.emailVerifications.DeleteByUser(ctx, u.ID); derr != nil {
			return derr
		}
		return s.emailVerifications.Save(ctx, ev)
	}); err != nil {
		return err
	}
	if s.verifyMailer != nil {
		return s.verifyMailer.SendVerificationCode(ctx, email, u.Name, code)
	}
	return nil
}
