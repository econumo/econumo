// Admin use cases driven by the CLI (bin/console app:*), not the HTTP API:
// create-user, change-user-email, change-user-password, activate-user,
// deactivate-users. They reuse the same repo/encode/hasher/clock/tx seams the
// HTTP handlers use, so behavior matches the API exactly. Ports the PHP
// CreateUserCommand / ChangeUserEmailCommand / ChangeUserPasswordCommand and the
// EconumoCloudBundle ActivateUserCommand / DeactivateUsersCommand.
package user

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
	domuser "github.com/econumo/econumo/internal/domain/user"
)

// AdminCreateUser creates a user regardless of ECONUMO_ALLOW_REGISTRATION and
// returns the new id. Ports CreateUserCommand (UserService::register).
func (s *Service) AdminCreateUser(ctx context.Context, name, email, password string) (vo.Id, error) {
	u, err := s.createUser(ctx, name, email, password)
	if err != nil {
		return vo.Id{}, err
	}
	return u.Id(), nil
}

// AdminChangeEmail changes a user's email (identifier, ciphertext, avatar),
// looked up by the current email. Ports ChangeUserEmailCommand
// (UserService::updateEmail).
func (s *Service) AdminChangeEmail(ctx context.Context, oldEmail, newEmail string) error {
	u, err := s.userByEmail(ctx, oldEmail)
	if err != nil {
		return err
	}

	loweredNew := strings.ToLower(strings.TrimSpace(newEmail))
	newIdentifier := s.encode.Hash(loweredNew)
	if newIdentifier != u.Identifier() {
		exists, err := s.repo.ExistsByIdentifier(ctx, newIdentifier)
		if err != nil {
			return err
		}
		if exists {
			return errs.NewValidation("User already exists")
		}
	}

	encryptedEmail, err := s.encode.Encode(strings.TrimSpace(newEmail))
	if err != nil {
		return err
	}
	avatarURL := fmt.Sprintf("https://www.gravatar.com/avatar/%s", md5Hex(loweredNew))

	return s.tx.WithTx(ctx, func(ctx context.Context) error {
		u.UpdateEmail(newIdentifier, encryptedEmail, avatarURL, s.clock.Now())
		return s.repo.Save(ctx, u)
	})
}

// AdminChangePassword sets a user's password (hashed with the user's salt),
// looked up by email. Ports ChangeUserPasswordCommand
// (UserPasswordService::updatePassword).
func (s *Service) AdminChangePassword(ctx context.Context, email, newPassword string) error {
	u, err := s.userByEmail(ctx, email)
	if err != nil {
		return err
	}
	return s.tx.WithTx(ctx, func(ctx context.Context) error {
		u.UpdatePassword(s.hasher.Hash(newPassword, u.Salt()), s.clock.Now())
		return s.repo.Save(ctx, u)
	})
}

// AdminActivate marks the user active, looked up by email. Ports
// ActivateUserCommand (User::activate).
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

// AdminDeactivateOlderThan deactivates every active user created strictly before
// cutoff, returning the number changed. Ports DeactivateUsersCommand
// (iterate all users, deactivate those older than --date).
func (s *Service) AdminDeactivateOlderThan(ctx context.Context, cutoff time.Time) (int, error) {
	ids, err := s.repo.ListIDs(ctx)
	if err != nil {
		return 0, err
	}
	count := 0
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		for _, id := range ids {
			u, err := s.repo.GetByID(ctx, id)
			if err != nil {
				return err
			}
			if u.IsActive() && u.CreatedAt().Before(cutoff) {
				u.Deactivate(s.clock.Now())
				if err := s.repo.Save(ctx, u); err != nil {
					return err
				}
				count++
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}

// userByEmail resolves a user from a plaintext email via the md5 identifier
// (the same lookup key registration computes). A miss returns the repo's
// NotFound error.
func (s *Service) userByEmail(ctx context.Context, email string) (*domuser.User, error) {
	lowered := strings.ToLower(strings.TrimSpace(email))
	return s.repo.GetByIdentifier(ctx, s.encode.Hash(lowered))
}
