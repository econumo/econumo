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

// ReclaimAccount implements the user feature's recovery port. A completed
// password reset proves control of provenEmail, so everything on the oauth side
// that could sign in as this user WITHOUT that proof has to go:
//
//   - identities whose provider vouches for a different address. Registration
//     does not always verify email, so someone who claimed the address first
//     could have linked their own provider account to it. An identity claiming
//     the proven address stays: obtaining one needs that mailbox.
//   - every pending grant: an unredeemed handoff is a session in waiting (60
//     seconds is long enough to hold one across a reset), and an in-flight link
//     request names the account it would attach an identity to.
//
// Returns how many identities and how many pending grants were removed.
func (s *Service) ReclaimAccount(ctx context.Context, userID vo.Id, provenEmail string) (int64, int64, error) {
	rows, err := s.identities.ListByUser(ctx, userID)
	if err != nil {
		return 0, 0, err
	}
	proven := strings.ToLower(strings.TrimSpace(provenEmail))
	var identities int64
	for _, r := range rows {
		if strings.ToLower(strings.TrimSpace(r.Email)) == proven {
			continue
		}
		n, derr := s.identities.DeleteByUserProvider(ctx, userID, r.Provider)
		if derr != nil {
			return identities, 0, derr
		}
		identities += n
	}
	grants, err := s.handoffs.DeleteByUser(ctx, userID)
	if err != nil {
		return identities, 0, err
	}
	states, err := s.states.DeleteByLinkUser(ctx, userID)
	if err != nil {
		return identities, grants, err
	}
	return identities, grants + states, nil
}
