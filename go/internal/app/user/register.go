// Register use case: create a new user with default options.
package user

import (
	"context"
	"fmt"
	"strings"

	"github.com/econumo/econumo/internal/domain/shared/errs"
	domuser "github.com/econumo/econumo/internal/domain/user"
)

// Register creates a new user (no token returned; see COMPATIBILITY.md). It is
// gated on ECONUMO_ALLOW_REGISTRATION; the actual creation is shared with the
// ungated CLI admin path via createUser.
func (s *Service) Register(ctx context.Context, req RegisterRequest) (*RegisterResult, error) {
	if !s.allowRegistration {
		return nil, errs.NewValidation("Registration disabled")
	}

	u, err := s.createUser(ctx, req.Name, req.Email, req.Password)
	if err != nil {
		return nil, err
	}

	cur, cerr := s.toCurrentUser(ctx, u)
	if cerr != nil {
		return nil, cerr
	}
	return &RegisterResult{User: cur}, nil
}

// createUser is the shared, UNGATED account-creation core used by Register
// (which adds the registration gate) and AdminCreateUser (the CLI, which does
// not). It generates a salt, hashes the password, encrypts the email, seeds the
// four default options, persists, and optionally connects the user to all
// existing users. It returns the saved aggregate. A duplicate email -> a
// validation error ("User already exists").
func (s *Service) createUser(ctx context.Context, name, email, password string) (*domuser.User, error) {
	loweredEmail := strings.ToLower(strings.TrimSpace(email))
	identifier := s.encode.Hash(loweredEmail)

	exists, err := s.repo.ExistsByIdentifier(ctx, identifier)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, errs.NewValidation("User already exists")
	}

	encryptedEmail, eerr := s.encode.Encode(email)
	if eerr != nil {
		return nil, eerr
	}
	salt, serr := newSalt()
	if serr != nil {
		return nil, serr
	}
	now := s.clock.Now()
	passwordHash := s.hasher.Hash(password, salt)
	avatarURL := fmt.Sprintf("https://www.gravatar.com/avatar/%s", md5Hex(loweredEmail))

	u := domuser.NewUser(s.repo.NextIdentity(), identifier, encryptedEmail, name, avatarURL, passwordHash, salt, now)
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

	return u, nil
}
