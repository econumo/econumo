// Password use cases: change, remind, and reset the password.
package user

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
	domuser "github.com/econumo/econumo/internal/domain/user"
)

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

// RemindPassword always succeeds from the caller's perspective (errors hidden
// to avoid user enumeration). Sending the actual email is not yet implemented —
// see notes.
func (s *Service) RemindPassword(ctx context.Context, req RemindPasswordRequest) (*RemindPasswordResult, error) {
	// TODO(mailer): the password reminder flow should create a
	// users_password_requests row and email a reset code. That requires the
	// mailer (infra/mailer, not built) and the password-request repository.
	// Until then this is a no-op that returns the same empty success envelope,
	// preserving the anti-enumeration contract. See notes.
	_ = ctx
	_ = req
	return &RemindPasswordResult{}, nil
}

// ResetPassword is not yet implemented (depends on the password-request flow).
func (s *Service) ResetPassword(ctx context.Context, req ResetPasswordRequest) (*ResetPasswordResult, error) {
	// TODO(password-reset): validate the code against users_password_requests,
	// check expiry, then set the new password. Depends on the not-yet-ported
	// password-request repository. Returns success envelope as a placeholder.
	_ = ctx
	_ = req
	return &ResetPasswordResult{}, nil
}
