// Register use case: create a new user with default options.
package user

import (
	"context"
	"strings"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
)

// Register creates a new user (no token returned; see CLAUDE.md). It is
// gated on ECONUMO_ALLOW_REGISTRATION; the actual creation is shared with the
// ungated CLI admin path via createUser.
func (s *Service) Register(ctx context.Context, req model.RegisterRequest) (*model.RegisterResult, error) {
	limitKey := strings.ToLower(strings.TrimSpace(req.Email))
	if err := s.allowAttempt(RateScopeRegister, limitKey); err != nil {
		return nil, err
	}
	s.failAttempt(RateScopeRegister, limitKey) // every attempt counts toward the cap

	if !s.allowRegistration {
		return nil, &errs.ValidationError{Msg: "Registration disabled", MsgCode: errs.CodeUserRegistrationDisabled}
	}

	u, err := s.createUser(ctx, req.Name, req.Email, req.Password, true)
	if err != nil {
		return nil, err
	}

	cur, cerr := s.toCurrentUser(ctx, u)
	if cerr != nil {
		return nil, cerr
	}
	return &model.RegisterResult{User: cur}, nil
}

// createUser is the shared, UNGATED account-creation core used by Register
// (which adds the registration gate) and AdminCreateUser (the CLI, which does
// not). It generates a salt, hashes the password, encrypts the email, seeds the
// four default options, and persists. It returns the saved aggregate. A
// duplicate email -> a validation error ("User already exists"). New users are
// never auto-connected to existing users; connections are created only by
// accepting an invite. Self-service registration grants a trial; accounts an
// operator creates through the CLI do not — an admin explicitly provisioning
// a user is trusted access, not a lead to be time-boxed.
func (s *Service) createUser(ctx context.Context, name, email, password string, grantTrial bool) (*model.User, error) {
	loweredEmail := strings.ToLower(strings.TrimSpace(email))
	identifier := s.encode.Hash(loweredEmail)

	exists, err := s.repo.ExistsByIdentifier(ctx, identifier)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, &errs.ValidationError{Msg: "User already exists", MsgCode: errs.CodeUserAlreadyExists}
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
	passwordHash, herr := s.hasher.Hash(password)
	if herr != nil {
		return nil, herr
	}
	avatar := s.avatars.Pick()

	u := model.NewUser(s.repo.NextIdentity(), identifier, encryptedEmail, name, avatar, passwordHash, salt, now)
	u.SeedDefaultOptions(s.repo.NextIdentity, now)
	if grantTrial && s.trial == "end-of-next-month" {
		until := model.TrialEnd(now)
		u.SetAccess(model.AccessLevelFull, &until, now)
	}

	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		return s.repo.Save(ctx, u)
	}); err != nil {
		return nil, err
	}

	return u, nil
}
