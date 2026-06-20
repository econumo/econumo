// Register use case: create a new user with default options.
package user

import (
	"context"
	"fmt"
	"strings"

	"github.com/econumo/econumo/internal/domain/shared/errs"
	domuser "github.com/econumo/econumo/internal/domain/user"
)

// Register creates a new user (no token returned; see COMPATIBILITY.md). It
// generates a salt, hashes the password, encrypts the email, seeds the four
// default options, and optionally connects the user to all existing users.
func (s *Service) Register(ctx context.Context, req RegisterRequest) (*RegisterResult, error) {
	if !s.allowRegistration {
		return nil, errs.NewValidation("Registration disabled")
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	identifier := s.encode.Hash(email)

	exists, err := s.repo.ExistsByIdentifier(ctx, identifier)
	if err != nil {
		return nil, err
	}
	if exists {
		// "User already exists" -> 400.
		return nil, errs.NewValidation("User already exists")
	}

	encryptedEmail, eerr := s.encode.Encode(req.Email)
	if eerr != nil {
		return nil, eerr
	}
	salt, serr := newSalt()
	if serr != nil {
		return nil, serr
	}
	now := s.clock.Now()
	passwordHash := s.hasher.Hash(req.Password, salt)
	avatarURL := fmt.Sprintf("https://www.gravatar.com/avatar/%s", md5Hex(email))

	u := domuser.NewUser(s.repo.NextIdentity(), identifier, encryptedEmail, req.Name, avatarURL, passwordHash, salt, now)
	u.SeedDefaultOptions(s.repo.NextIdentity, now)

	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.repo.Save(ctx, u); err != nil {
			return err
		}
		if s.connectUsers {
			// TODO(connection-module): registration should also create a first
			// "All accounts" folder and a connection invite, and (when
			// ECONUMO_CONNECT_USERS) connect the new user to every existing
			// user bidirectionally. Connections live in the not-yet-ported
			// connection module (users_connections table). Wire this once that
			// module exists; for now connect-users is a no-op so registration
			// still succeeds. See notes.
			_ = ctx
		}
		return nil
	}); err != nil {
		return nil, err
	}

	cur, cerr := s.toCurrentUser(ctx, u)
	if cerr != nil {
		return nil, cerr
	}
	return &RegisterResult{User: cur}, nil
}
