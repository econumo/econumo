// Login use case: authenticate by identifier and issue a JWT.
package user

import (
	"context"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/shared/errs"
)

// Login authenticates by identifier (md5 of the lowercased username), verifies
// the password against the stored hash+salt, issues a JWT, and returns the
// token + current user. A bad username or password yields an UnauthorizedError
// (HTTP 401, "Invalid credentials.").
func (s *Service) Login(ctx context.Context, req LoginRequest, now time.Time) (*LoginResult, error) {
	identifier := s.encode.Hash(strings.ToLower(req.Username))
	u, err := s.repo.GetByIdentifier(ctx, identifier)
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			return nil, errs.NewUnauthorized("Invalid credentials.")
		}
		return nil, err
	}
	if !u.IsActive() || !s.hasher.Verify(u.Password(), req.Password, u.Salt()) {
		return nil, errs.NewUnauthorized("Invalid credentials.")
	}

	email, derr := s.encode.Decode(u.Email())
	if derr != nil {
		return nil, derr
	}
	token, terr := s.jwt.Issue(u.Id().String(), email, now)
	if terr != nil {
		return nil, terr
	}
	cur, cerr := s.toCurrentUserWithEmail(ctx, u, email)
	if cerr != nil {
		return nil, cerr
	}
	return &LoginResult{Token: token, User: cur}, nil
}
