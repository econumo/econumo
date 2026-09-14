package oauth

import (
	"context"
	"strings"

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

// UnlinkForeignIdentities implements the user feature's recovery port: it
// removes every identity of the user whose provider does not vouch for
// provenEmail — the address a completed password reset just proved the caller
// controls. An identity claiming that same address stays: obtaining one
// requires the mailbox, so it cannot predate the owner. This is what stops a
// squatter's own linked provider account from outliving the reclaim of an
// address they never owned.
func (s *Service) UnlinkForeignIdentities(ctx context.Context, userID vo.Id, provenEmail string) (int64, error) {
	rows, err := s.identities.ListByUser(ctx, userID)
	if err != nil {
		return 0, err
	}
	proven := strings.ToLower(strings.TrimSpace(provenEmail))
	var removed int64
	for _, r := range rows {
		if strings.ToLower(strings.TrimSpace(r.Email)) == proven {
			continue
		}
		n, derr := s.identities.DeleteByUserProvider(ctx, userID, r.Provider)
		if derr != nil {
			return removed, derr
		}
		removed += n
	}
	return removed, nil
}
