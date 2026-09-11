package oauth

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *Service) ListIdentities(ctx context.Context, userID vo.Id) ([]model.IdentityItem, error) {
	rows, err := s.identities.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]model.IdentityItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, model.IdentityItem{Provider: r.Provider, Email: r.Email, CreatedAt: r.CreatedAt.UTC().Format(datetime.Layout)})
	}
	return out, nil
}

// UnlinkIdentity refuses to remove the last identity of a passwordless user:
// it would lock them out.
func (s *Service) UnlinkIdentity(ctx context.Context, userID vo.Id, req model.UnlinkIdentityRequest) (*model.UnlinkIdentityResult, error) {
	if _, err := s.identities.GetByUserProvider(ctx, userID, req.Provider); err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			return nil, &errs.ValidationError{Msg: "Linked account not found", MsgCode: errs.CodeOAuthIdentityNotFound}
		}
		return nil, err
	}
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !u.HasPassword() {
		n, cerr := s.identities.CountByUser(ctx, userID)
		if cerr != nil {
			return nil, cerr
		}
		if n <= 1 {
			return nil, &errs.ValidationError{Msg: "Set a password before unlinking your only sign-in method", MsgCode: errs.CodeOAuthLastIdentity}
		}
	}
	if _, err := s.identities.DeleteByUserProvider(ctx, userID, req.Provider); err != nil {
		return nil, err
	}
	return &model.UnlinkIdentityResult{}, nil
}
