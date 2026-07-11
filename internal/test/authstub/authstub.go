// Package authstub provides a middleware.TokenAuthenticator for feature api
// tests: the bearer token IS the user id string, so a harness needs no token
// store — pass the seeded user's id as the Authorization token. The token row
// id it reports equals the user id (feature tests never inspect it).
package authstub

import (
	"context"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type Authenticator struct{}

func (Authenticator) Authenticate(_ context.Context, token string) (vo.Id, vo.Id, error) {
	id, err := vo.ParseId(token)
	if err != nil {
		return vo.Id{}, vo.Id{}, errs.NewUnauthorized("Invalid access token")
	}
	return id, id, nil
}
