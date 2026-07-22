// Email-verification use cases (ECONUMO_EMAIL_VERIFICATION): an unverified
// user's login is denied with a 403 signal (sending the code email as a side
// effect), and the public confirm/resend endpoints mirror the reset-password
// pair's anti-enumeration and rate-limit discipline — the emailed code is the
// sole secret on the confirm route, so failures are generic and counted.
package user

import (
	"context"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// requireVerifiedEmail gates an unverified user's login: it ensures an
// outstanding verification email exists (sending one when none is live) and
// denies with email_verification_required. Confirming happens on the
// dedicated confirm-email endpoint, never in login.
func (s *Service) requireVerifiedEmail(ctx context.Context, u *model.User, email string, limitKey string) error {
	if err := s.ensureVerificationCode(ctx, u, email, false, limitKey, s.clock.Now()); err != nil {
		return err
	}
	return &errs.AccessDeniedError{Msg: "Please verify your email address.", Code: errs.CodeUserEmailVerificationRequired}
}

// ConfirmEmail validates the (email, code) pair and marks the email verified.
// Unknown user, missing row, and wrong code are indistinguishable (generic
// invalid-code error) so the route cannot be used for account enumeration;
// failed attempts count toward the confirm-email cap and clear on success.
func (s *Service) ConfirmEmail(ctx context.Context, req model.ConfirmEmailRequest) (*model.ConfirmEmailResult, error) {
	lowered := strings.ToLower(strings.TrimSpace(req.Username))
	if err := s.allowAttempt(RateScopeConfirmEmail, lowered); err != nil {
		return nil, err
	}
	invalid := &errs.ValidationError{Msg: "The confirmation code is not valid.", MsgCode: errs.CodeUserVerificationCodeInvalid}

	u, err := s.repo.GetByIdentifier(ctx, s.encode.Hash(lowered))
	if err != nil {
		if isNotFound(err) {
			s.failAttempt(RateScopeConfirmEmail, lowered)
			return nil, invalid
		}
		return nil, err
	}
	ev, err := s.emailVerifications.GetByUser(ctx, u.ID)
	if err != nil {
		if isNotFound(err) {
			s.failAttempt(RateScopeConfirmEmail, lowered)
			return nil, invalid
		}
		return nil, err
	}
	if HashResetCode(strings.TrimSpace(req.Code)) != ev.Code {
		s.failAttempt(RateScopeConfirmEmail, lowered)
		return nil, invalid
	}
	if ev.IsExpired(s.clock.Now()) {
		s.failAttempt(RateScopeConfirmEmail, lowered)
		return nil, &errs.ValidationError{Msg: "The code is expired", MsgCode: errs.CodeUserVerificationCodeExpired}
	}

	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		u.MarkEmailVerified(s.clock.Now())
		if serr := s.repo.Save(ctx, u); serr != nil {
			return serr
		}
		return s.emailVerifications.DeleteByUser(ctx, u.ID)
	}); err != nil {
		return nil, err
	}
	s.clearAttempt(RateScopeConfirmEmail, lowered)
	return &model.ConfirmEmailResult{}, nil
}

// ResendVerificationCode force-sends a fresh code to an unverified user. It
// rate-limits UNCONDITIONALLY, before the existence check — mirroring
// RemindPassword — so an unknown/already-verified username consumes the
// verify-email cap exactly like a real unverified user, and hits the same 429
// once the cap is spent; without this, the limiter would only ever be touched
// for genuine unverified accounts, letting a caller distinguish "unverified
// account exists" from "unknown/verified" (an enumeration oracle). It always
// reports success on a non-send path — an unknown or already-verified
// username is a silent no-op — so the route cannot be used for account
// enumeration via the response body either.
func (s *Service) ResendVerificationCode(ctx context.Context, req model.ResendVerificationCodeRequest) (*model.ResendVerificationCodeResult, error) {
	lowered := strings.ToLower(strings.TrimSpace(req.Username))
	if err := s.allowAttempt(RateScopeVerifyEmail, lowered); err != nil {
		return nil, err
	}
	s.failAttempt(RateScopeVerifyEmail, lowered) // every resend counts, like remind

	u, err := s.repo.GetByIdentifier(ctx, s.encode.Hash(lowered))
	if err != nil {
		if isNotFound(err) {
			return &model.ResendVerificationCodeResult{}, nil // anti-enumeration
		}
		return nil, err
	}
	if u.EmailVerified {
		return &model.ResendVerificationCodeResult{}, nil
	}
	email, derr := s.encode.Decode(u.Email)
	if derr != nil {
		return nil, derr
	}
	if err := s.issueVerificationCode(ctx, u, email, s.clock.Now()); err != nil {
		return nil, err
	}
	return &model.ResendVerificationCodeResult{}, nil
}

// ensureVerificationCode sends a fresh code when none is outstanding, the
// outstanding one expired, or the caller forced a resend. A send counts
// toward the verify-email rate cap under limitKey; a live code is otherwise
// reused silently so repeated code-less logins cannot spam the mailbox.
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
	return s.issueVerificationCode(ctx, u, email, now)
}

// issueVerificationCode generates a fresh code, replaces any outstanding row,
// and emails it. It does NOT rate-limit — callers own that decision (and the
// accounting), so a real send is never counted twice.
func (s *Service) issueVerificationCode(ctx context.Context, u *model.User, email string, now time.Time) error {
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
