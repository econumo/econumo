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
	wait, err := s.ensureVerificationCode(ctx, u, email, limitKey, s.clock.Now())
	if err != nil {
		return err
	}
	// The retry time rides along on the denial so the client's countdown is
	// correct the moment the verify dialog opens, with no extra round trip.
	return &errs.AccessDeniedError{
		Msg:        "Please verify your email address.",
		Code:       errs.CodeUserEmailVerificationRequired,
		RetryAfter: int(wait / time.Second),
	}
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
//
// The second return value is how long until another code may be requested; the
// HTTP edge emits it as Retry-After. It is the same for every caller — unknown
// and already-verified usernames included — so it can never be read as proof
// that an unverified account exists.
func (s *Service) ResendVerificationCode(ctx context.Context, req model.ResendVerificationCodeRequest) (*model.ResendVerificationCodeResult, time.Duration, error) {
	lowered := strings.ToLower(strings.TrimSpace(req.Username))
	now := s.clock.Now()

	// The cooldown is read BEFORE the attempt is recorded and comes from the
	// limiter, not the verification row: the limiter is keyed by username alone
	// and is written for every caller, so an unknown username counts down
	// exactly like a real one. Deriving it from the DB row instead would answer
	// "60" for everyone except genuine unverified accounts mid-cooldown, making
	// any other value proof that such an account exists.
	wait := s.resendCooldown(lowered, now)

	if err := s.allowAttempt(RateScopeVerifyEmail, lowered); err != nil {
		return nil, 0, err
	}
	s.failAttempt(RateScopeVerifyEmail, lowered) // every resend counts, like remind

	result := &model.ResendVerificationCodeResult{}
	if wait > 0 {
		// Still inside the gap: report the real remainder and send nothing. This
		// is the enforcement — ignoring the countdown, reloading, or opening a
		// second tab cannot trigger another email. The gap is NOT renewed here,
		// so waiting it out always eventually yields a new code.
		return result, wait, nil
	}
	// Past the gap: this call starts a new one. Marked for every username —
	// unknown ones included — so the countdown a caller observes is identical
	// whether or not the account exists.
	s.markSent(lowered)
	fullGap := model.EmailVerificationResendGap

	u, err := s.repo.GetByIdentifier(ctx, s.encode.Hash(lowered))
	if err != nil {
		if isNotFound(err) {
			return result, fullGap, nil // anti-enumeration
		}
		return nil, 0, err
	}
	if u.EmailVerified {
		return result, fullGap, nil
	}
	email, derr := s.encode.Decode(u.Email)
	if derr != nil {
		return nil, 0, derr
	}
	// Second gate, on the row's own timestamp: the limiter above is optional
	// (nil in the CLI and some tests), so the gap must also hold without it.
	// The reported wait still comes from the uniform path above, so this check
	// suppresses the email without leaking that the account exists.
	if rowWait, werr := s.verificationRetryAfter(ctx, u.ID, now); werr != nil {
		return nil, 0, werr
	} else if rowWait > 0 {
		return result, fullGap, nil
	}
	if err := s.issueVerificationCode(ctx, u, email, now); err != nil {
		return nil, 0, err
	}
	return result, fullGap, nil
}

// verificationRetryAfter reports how long until the user may be sent another
// code, from the outstanding row. No row means no recent send.
func (s *Service) verificationRetryAfter(ctx context.Context, userID vo.Id, now time.Time) (time.Duration, error) {
	ev, err := s.emailVerifications.GetByUser(ctx, userID)
	if err != nil {
		if isNotFound(err) {
			return 0, nil
		}
		return 0, err
	}
	return ev.RetryAfter(now), nil
}

// markSent starts a fresh resend gap for the username.
func (s *Service) markSent(username string) {
	if s.limiter != nil {
		s.limiter.Mark(RateScopeVerifySent, username)
	}
}

// resendCooldown reports how long until this username may trigger another code
// email, measured from the last SEND recorded by the limiter (see markSent) —
// never from the last attempt, which would let an impatient user renew their
// own lockout indefinitely by clicking again.
func (s *Service) resendCooldown(username string, now time.Time) time.Duration {
	if s.limiter == nil {
		return 0
	}
	last, ok := s.limiter.LastAttempt(RateScopeVerifySent, username)
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

// ensureVerificationCode sends a fresh code when none is outstanding or the
// outstanding one expired, and reports how long until another may be sent. A
// send counts toward the verify-email rate cap under limitKey; a live code is
// otherwise reused silently so repeated code-less logins cannot spam the
// mailbox.
func (s *Service) ensureVerificationCode(ctx context.Context, u *model.User, email string, limitKey string, now time.Time) (time.Duration, error) {
	send := false
	var live *model.EmailVerification
	ev, err := s.emailVerifications.GetByUser(ctx, u.ID)
	switch {
	case err != nil:
		if _, ok := errs.AsNotFound(err); !ok {
			return 0, err
		}
		send = true
	case ev.IsExpired(now):
		send = true
	default:
		live = ev
	}
	if !send {
		// Reuse the live code. The wait comes from the same limiter source the
		// resend endpoint reports, so the two endpoints never disagree; the
		// row's own timestamp is the fallback when no limiter is wired.
		if wait := s.resendCooldown(limitKey, now); wait > 0 {
			return wait, nil
		}
		return live.RetryAfter(now), nil
	}
	if err := s.allowAttempt(RateScopeVerifyEmail, limitKey); err != nil {
		return 0, err
	}
	s.failAttempt(RateScopeVerifyEmail, limitKey)
	if err := s.issueVerificationCode(ctx, u, email, now); err != nil {
		return 0, err
	}
	// Same clock the resend endpoint reads, so the 403's Retry-After and a
	// subsequent resend's retryAfter always agree.
	s.markSent(limitKey)
	return model.EmailVerificationResendGap, nil
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
