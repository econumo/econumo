// Admin use cases driven by the CLI (the resource:action commands), not the HTTP
// API: create-user, change-user-email, change-user-password, activate-user,
// deactivate-users. They reuse the same repo/encode/hasher/clock/tx seams the
// HTTP handlers use, so behavior matches the API exactly.
package user

import (
	"context"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// AdminCreateUser creates a user regardless of ECONUMO_ALLOW_REGISTRATION and
// returns the new id.
func (s *Service) AdminCreateUser(ctx context.Context, name, email, password string) (vo.Id, error) {
	u, err := s.createUser(ctx, name, email, password, false)
	if err != nil {
		return vo.Id{}, err
	}
	return u.ID, nil
}

// AdminChangeEmail changes a user's email (identifier, ciphertext), looked up
// by the current email. The avatar is left unchanged.
func (s *Service) AdminChangeEmail(ctx context.Context, oldEmail, newEmail string) error {
	u, err := s.userByEmail(ctx, oldEmail)
	if err != nil {
		return err
	}

	loweredNew := strings.ToLower(strings.TrimSpace(newEmail))
	newIdentifier := s.encode.Hash(loweredNew)
	if newIdentifier != u.Identifier {
		exists, err := s.repo.ExistsByIdentifier(ctx, newIdentifier)
		if err != nil {
			return err
		}
		if exists {
			return &errs.ValidationError{Msg: "User already exists", MsgCode: errs.CodeUserAlreadyExists}
		}
	}

	encryptedEmail, err := s.encode.Encode(strings.TrimSpace(newEmail))
	if err != nil {
		return err
	}

	return s.tx.WithTx(ctx, func(ctx context.Context) error {
		u.UpdateEmail(newIdentifier, encryptedEmail, s.clock.Now())
		return s.repo.Save(ctx, u)
	})
}

// AdminChangePassword sets a user's password, rehashed with the current
// algorithm (argon2id), looked up by email. All the user's sessions are
// revoked (there is no presenting session on the CLI path); PATs survive.
func (s *Service) AdminChangePassword(ctx context.Context, email, newPassword string) error {
	u, err := s.userByEmail(ctx, email)
	if err != nil {
		return err
	}
	newHash, herr := s.hasher.Hash(newPassword)
	if herr != nil {
		return herr
	}
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		u.UpdatePassword(newHash, model.AlgorithmArgon2id, s.clock.Now())
		return s.repo.Save(ctx, u)
	}); err != nil {
		return err
	}
	return s.revokeSessions(ctx, u.ID, vo.Id{}, s.clock.Now())
}

// AdminActivate marks the user active, looked up by email.
func (s *Service) AdminActivate(ctx context.Context, email string) error {
	u, err := s.userByEmail(ctx, email)
	if err != nil {
		return err
	}
	return s.tx.WithTx(ctx, func(ctx context.Context) error {
		u.Activate(s.clock.Now())
		return s.repo.Save(ctx, u)
	})
}

// AdminDeactivate marks the user inactive, looked up by email, and revokes
// EVERY credential (sessions AND personal tokens) — this is why per-request
// authentication needs no is_active join: a deactivated user simply has no
// live tokens left.
func (s *Service) AdminDeactivate(ctx context.Context, email string) error {
	u, err := s.userByEmail(ctx, email)
	if err != nil {
		return err
	}
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		u.Deactivate(s.clock.Now())
		return s.repo.Save(ctx, u)
	}); err != nil {
		return err
	}
	return s.revokeTokens(ctx, u.ID, vo.Id{}, s.clock.Now(), model.TokenKindSession, model.TokenKindPersonal)
}

// AdminSetAccess sets a user's access level and optional expiry, looked up by
// email. A nil until means the level never expires.
func (s *Service) AdminSetAccess(ctx context.Context, email string, level model.AccessLevel, until *time.Time) error {
	u, err := s.userByEmail(ctx, email)
	if err != nil {
		return err
	}
	return s.tx.WithTx(ctx, func(ctx context.Context) error {
		u.SetAccess(level, until, s.clock.Now())
		return s.repo.Save(ctx, u)
	})
}

// AdminShowUser looks up a user by email and returns the raw stored access
// alongside the effective level (collapsed against now) so an operator gets
// both the inputs and the answer in one call.
func (s *Service) AdminShowUser(ctx context.Context, email string) (*model.User, model.AccessLevel, error) {
	u, err := s.userByEmail(ctx, email)
	if err != nil {
		return nil, "", err
	}
	return u, u.EffectiveAccessLevel(s.clock.Now()), nil
}

// userByEmail resolves a user from a plaintext email via the md5 identifier
// (the same lookup key registration computes). A miss returns the repo's
// NotFound error.
func (s *Service) userByEmail(ctx context.Context, email string) (*model.User, error) {
	lowered := strings.ToLower(strings.TrimSpace(email))
	return s.repo.GetByIdentifier(ctx, s.encode.Hash(lowered))
}

// AdminUserByID loads a user by id with the email decrypted, for the admin
// listener (which addresses users by id, never by email).
func (s *Service) AdminUserByID(ctx context.Context, id vo.Id) (*model.User, string, error) {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, "", err
	}
	email, err := s.encode.Decode(u.Email)
	if err != nil {
		return nil, "", err
	}
	return u, email, nil
}

// AdminSetAccessByID is AdminSetAccess keyed by id: an operator running the CLI
// has an email address, the payment portal has a user id.
func (s *Service) AdminSetAccessByID(ctx context.Context, id vo.Id, level model.AccessLevel, until *time.Time) error {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	return s.tx.WithTx(ctx, func(ctx context.Context) error {
		u.SetAccess(level, until, s.clock.Now())
		return s.repo.Save(ctx, u)
	})
}
