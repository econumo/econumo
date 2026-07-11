// Personal access tokens: user-created bearer credentials for integrations.
// Full-access (no scopes); optional fixed expiry; never touched by password
// changes — only explicit revocation or user:deactivate kills them.
package user

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *Service) CreatePersonalToken(ctx context.Context, userID vo.Id, req model.CreatePersonalTokenRequest) (*model.CreatePersonalTokenResult, error) {
	now := s.clock.Now()
	var expiresAt *time.Time
	if req.ExpiresAt != "" {
		exp, err := time.Parse(datetime.Layout, req.ExpiresAt)
		if err != nil {
			// Validate() already rejects this; defense in depth.
			return nil, errs.NewValidation("Invalid expiration date")
		}
		if !exp.After(now) {
			return nil, errs.NewValidation("Expiration date must be in the future")
		}
		expiresAt = &exp
	}
	raw, hash, err := generateAccessToken(model.TokenKindPersonal)
	if err != nil {
		return nil, err
	}
	name := req.Name
	t := &model.AccessToken{
		ID: vo.NewId(), UserID: userID, Kind: model.TokenKindPersonal, TokenHash: hash,
		Name: &name, CreatedAt: now, LastUsedAt: now, ExpiresAt: expiresAt,
	}
	if err := s.tokens.Insert(ctx, t); err != nil {
		return nil, err
	}
	return &model.CreatePersonalTokenResult{
		Id: t.ID.String(), Name: name, Token: raw,
		CreatedAt: now.UTC().Format(datetime.Layout),
		ExpiresAt: formatOptionalDatetime(expiresAt),
	}, nil
}

// ListPersonalTokens returns the user's LIVE personal tokens; the raw token is
// never part of any list response.
func (s *Service) ListPersonalTokens(ctx context.Context, userID vo.Id) ([]model.PersonalTokenItem, error) {
	rows, err := s.tokens.ListByUser(ctx, userID, model.TokenKindPersonal)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	out := make([]model.PersonalTokenItem, 0, len(rows))
	for i := range rows {
		if !rows[i].IsLive(now) {
			continue
		}
		name := ""
		if rows[i].Name != nil {
			name = *rows[i].Name
		}
		out = append(out, model.PersonalTokenItem{
			Id: rows[i].ID.String(), Name: name,
			CreatedAt:  rows[i].CreatedAt.UTC().Format(datetime.Layout),
			LastUsedAt: rows[i].LastUsedAt.UTC().Format(datetime.Layout),
			ExpiresAt:  formatOptionalDatetime(rows[i].ExpiresAt),
		})
	}
	return out, nil
}

// RevokePersonalToken revokes one of the caller's PATs. A foreign, unknown, or
// session id is the uniform domain-not-found error (no existence oracle).
func (s *Service) RevokePersonalToken(ctx context.Context, userID vo.Id, req model.RevokePersonalTokenRequest) (*model.RevokePersonalTokenResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, errs.NewNotFound("Token not found")
	}
	t, err := s.tokens.GetByID(ctx, id)
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			return nil, errs.NewNotFound("Token not found")
		}
		return nil, err
	}
	if !t.UserID.Equal(userID) || t.Kind != model.TokenKindPersonal {
		return nil, errs.NewNotFound("Token not found")
	}
	t.Revoke(s.clock.Now())
	if err := s.tokens.Update(ctx, t); err != nil {
		return nil, err
	}
	return &model.RevokePersonalTokenResult{}, nil
}

func formatOptionalDatetime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(datetime.Layout)
	return &s
}
