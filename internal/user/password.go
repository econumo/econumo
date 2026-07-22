// Password use cases: change, remind, and reset the password.
package user

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// passwordCodeBytes is the random byte count for a reset code; hex-encoded it
// yields the frozen 12-character code length.
const passwordCodeBytes = 6

// generatePasswordCode returns a fresh 12-char hex reset code.
func generatePasswordCode() (string, error) {
	b := make([]byte, passwordCodeBytes)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// HashResetCode maps a reset code to its at-rest storage/lookup key. The
// plaintext code is emailed to the user; only this sha256 hex is persisted, so a
// database disclosure does not hand out usable reset codes.
func HashResetCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

func isNotFound(err error) bool {
	var nf *errs.NotFoundError
	return errors.As(err, &nf)
}

// UpdatePassword verifies the old password then stores the new hash. A wrong
// old password yields a ValidationError -> 400 ("Password is not correct").
// On success every OTHER session is revoked (the presenting one —
// currentTokenID — survives); PATs are untouched.
func (s *Service) UpdatePassword(ctx context.Context, userID vo.Id, currentTokenID vo.Id, req model.UpdatePasswordRequest) (*model.UpdatePasswordResult, error) {
	_, err := s.mutate(ctx, userID, func(u *model.User, now time.Time) error {
		if !s.hasher.Verify(u.Algorithm, u.Password, req.OldPassword, u.Salt) {
			return &errs.ValidationError{Msg: "Password is not correct", MsgCode: errs.CodeUserPasswordIncorrect}
		}
		newHash, herr := s.hasher.Hash(req.NewPassword)
		if herr != nil {
			return herr
		}
		u.UpdatePassword(newHash, model.AlgorithmArgon2id, now)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := s.revokeSessions(ctx, userID, currentTokenID, s.clock.Now()); err != nil {
		return nil, err
	}
	return &model.UpdatePasswordResult{}, nil
}

// RemindPassword issues a password-reset code: it replaces the user's existing
// codes with a fresh one (10-min expiry) and emails it. A missing user is hidden
// (returns success) to avoid account enumeration.
func (s *Service) RemindPassword(ctx context.Context, req model.RemindPasswordRequest) (*model.RemindPasswordResult, error) {
	lowered := strings.ToLower(strings.TrimSpace(req.Username))
	if err := s.allowAttempt(RateScopeRemind, lowered); err != nil {
		return nil, err
	}
	s.failAttempt(RateScopeRemind, lowered) // every remind sends an email, so every request counts

	u, err := s.repo.GetByIdentifier(ctx, s.encode.Hash(lowered))
	if err != nil {
		if isNotFound(err) {
			return &model.RemindPasswordResult{}, nil // anti-enumeration
		}
		return nil, err
	}

	code, err := generatePasswordCode()
	if err != nil {
		return nil, err
	}
	pr := model.NewPasswordRequest(vo.NewId(), u.ID, HashResetCode(code), s.clock.Now())
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		if derr := s.passwordRequests.DeleteByUser(ctx, u.ID); derr != nil {
			return derr
		}
		return s.passwordRequests.Save(ctx, pr)
	}); err != nil {
		return nil, err
	}

	// Email the code to the address the caller submitted (trimmed, original case).
	if s.mailer != nil {
		if err := s.mailer.SendResetPasswordCode(ctx, strings.TrimSpace(req.Username), u.Name, code); err != nil {
			return nil, err
		}
	}
	return &model.RemindPasswordResult{}, nil
}

// ResetPassword validates the (email, code) reset request, and on success sets
// the new password and consumes the code. An unknown user/code yields a generic
// validation error; an expired code yields the frozen "The code is expired".
func (s *Service) ResetPassword(ctx context.Context, req model.ResetPasswordRequest) (*model.ResetPasswordResult, error) {
	lowered := strings.ToLower(strings.TrimSpace(req.Username))
	if err := s.allowAttempt(RateScopeReset, lowered); err != nil {
		return nil, err
	}
	u, err := s.repo.GetByIdentifier(ctx, s.encode.Hash(lowered))
	if err != nil {
		if isNotFound(err) {
			s.failAttempt(RateScopeReset, lowered)
			return nil, &errs.ValidationError{Msg: "Reset password error", MsgCode: errs.CodeUserResetPasswordError}
		}
		return nil, err
	}

	pr, err := s.passwordRequests.GetByUserAndCode(ctx, u.ID, HashResetCode(strings.TrimSpace(req.Code)))
	if err != nil {
		if isNotFound(err) {
			s.failAttempt(RateScopeReset, lowered)
			return nil, &errs.ValidationError{Msg: "Reset password error", MsgCode: errs.CodeUserResetPasswordError}
		}
		return nil, err
	}
	if pr.IsExpired(s.clock.Now()) {
		s.failAttempt(RateScopeReset, lowered)
		return nil, &errs.ValidationError{Msg: "The code is expired", MsgCode: errs.CodeUserResetCodeExpired}
	}

	newHash, herr := s.hasher.Hash(req.Password)
	if herr != nil {
		return nil, herr
	}
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		u.UpdatePassword(newHash, model.AlgorithmArgon2id, s.clock.Now())
		// Completing a reset proves mailbox ownership, so it also satisfies the
		// email-verification gate.
		u.MarkEmailVerified(s.clock.Now())
		if serr := s.repo.Save(ctx, u); serr != nil {
			return serr
		}
		return s.passwordRequests.Delete(ctx, pr.ID)
	}); err != nil {
		return nil, err
	}
	// The reset flow has no presenting session, so ALL sessions are revoked —
	// whoever holds the account's email owns the account now.
	if err := s.revokeSessions(ctx, u.ID, vo.Id{}, s.clock.Now()); err != nil {
		return nil, err
	}
	s.clearAttempt(RateScopeReset, lowered)
	return &model.ResetPasswordResult{}, nil
}
