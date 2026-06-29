// Password use cases: change, remind, and reset the password.
package user

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
	domuser "github.com/econumo/econumo/internal/domain/user"
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

func isNotFound(err error) bool {
	var nf *errs.NotFoundError
	return errors.As(err, &nf)
}

// UpdatePassword verifies the old password then stores the new hash. A wrong
// old password yields a ValidationError -> 400 ("Password is not correct").
func (s *Service) UpdatePassword(ctx context.Context, userID vo.Id, req UpdatePasswordRequest) (*UpdatePasswordResult, error) {
	_, err := s.mutate(ctx, userID, func(u *domuser.User, now time.Time) error {
		if !s.hasher.Verify(u.Password(), req.OldPassword, u.Salt()) {
			return errs.NewValidation("Password is not correct")
		}
		u.UpdatePassword(s.hasher.Hash(req.NewPassword, u.Salt()), now)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &UpdatePasswordResult{}, nil
}

// RemindPassword issues a password-reset code: it replaces the user's existing
// codes with a fresh one (10-min expiry) and emails it. A missing user is hidden
// (returns success) to avoid account enumeration.
func (s *Service) RemindPassword(ctx context.Context, req RemindPasswordRequest) (*RemindPasswordResult, error) {
	lowered := strings.ToLower(strings.TrimSpace(req.Username))
	u, err := s.repo.GetByIdentifier(ctx, s.encode.Hash(lowered))
	if err != nil {
		if isNotFound(err) {
			return &RemindPasswordResult{}, nil // anti-enumeration
		}
		return nil, err
	}

	code, err := generatePasswordCode()
	if err != nil {
		return nil, err
	}
	pr := domuser.NewPasswordRequest(vo.NewId(), u.Id(), code, s.clock.Now())
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		if derr := s.passwordRequests.DeleteByUser(ctx, u.Id()); derr != nil {
			return derr
		}
		return s.passwordRequests.Save(ctx, pr)
	}); err != nil {
		return nil, err
	}

	// Email the code to the address the caller submitted (trimmed, original case).
	if s.mailer != nil {
		if err := s.mailer.SendResetPasswordCode(ctx, strings.TrimSpace(req.Username), u.Name(), code); err != nil {
			return nil, err
		}
	}
	return &RemindPasswordResult{}, nil
}

// ResetPassword validates the (email, code) reset request, and on success sets
// the new password and consumes the code. An unknown user/code yields a generic
// validation error; an expired code yields the frozen "The code is expired".
func (s *Service) ResetPassword(ctx context.Context, req ResetPasswordRequest) (*ResetPasswordResult, error) {
	lowered := strings.ToLower(strings.TrimSpace(req.Username))
	u, err := s.repo.GetByIdentifier(ctx, s.encode.Hash(lowered))
	if err != nil {
		if isNotFound(err) {
			return nil, errs.NewValidation("Reset password error")
		}
		return nil, err
	}

	pr, err := s.passwordRequests.GetByUserAndCode(ctx, u.Id(), strings.TrimSpace(req.Code))
	if err != nil {
		if isNotFound(err) {
			return nil, errs.NewValidation("Reset password error")
		}
		return nil, err
	}
	if pr.IsExpired(s.clock.Now()) {
		return nil, errs.NewValidation("The code is expired")
	}

	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		u.UpdatePassword(s.hasher.Hash(req.Password, u.Salt()), s.clock.Now())
		if serr := s.repo.Save(ctx, u); serr != nil {
			return serr
		}
		return s.passwordRequests.Delete(ctx, pr.Id())
	}); err != nil {
		return nil, err
	}
	return &ResetPasswordResult{}, nil
}
