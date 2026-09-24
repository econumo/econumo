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

// ListIdentityEmails returns the address each linked provider vouched for, for
// CC'ing the account's notice emails. Rows whose provider reported no address
// are skipped (the column defaults to ""). Unlike ListIdentities this is not a
// wire DTO: the caller is the mail path, not an HTTP handler.
func (s *Service) ListIdentityEmails(ctx context.Context, userID vo.Id) ([]string, error) {
	rows, err := s.identities.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if strings.TrimSpace(r.Email) == "" {
			continue
		}
		out = append(out, r.Email)
	}
	return out, nil
}

// UnlinkIdentity refuses to remove the last identity of a user with no other
// way in — passwordless, or password sign-in switched off: it would lock them
// out. The whole check-then-delete runs in one transaction
// that opens by taking the user row's lock, so two unlinks arriving together
// cannot both count two identities and both delete.
func (s *Service) UnlinkIdentity(ctx context.Context, userID vo.Id, req model.UnlinkIdentityRequest) (*model.UnlinkIdentityResult, error) {
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.users.LockRow(ctx, userID); err != nil {
			return err
		}
		if _, err := s.identities.GetByUserProvider(ctx, userID, req.Provider); err != nil {
			if _, ok := errs.AsNotFound(err); ok {
				return &errs.ValidationError{Msg: "Linked account not found", MsgCode: errs.CodeOAuthIdentityNotFound}
			}
			return err
		}
		u, err := s.users.FindByID(ctx, userID)
		if err != nil {
			return err
		}
		if !u.HasPassword() || s.passwordLoginDisabled {
			n, cerr := s.identities.CountByUser(ctx, userID)
			if cerr != nil {
				return cerr
			}
			if n <= 1 {
				// "Set a password" is no remedy while passwords are off.
				if s.passwordLoginDisabled {
					return &errs.ValidationError{Msg: "You can't unlink your only sign-in method", MsgCode: errs.CodeOAuthLastSignInMethod}
				}
				return &errs.ValidationError{Msg: "Set a password before unlinking your only sign-in method", MsgCode: errs.CodeOAuthLastIdentity}
			}
		}
		if _, err := s.identities.DeleteByUserProvider(ctx, userID, req.Provider); err != nil {
			return err
		}
		// An unredeemed handoff is a session in waiting: a sign-in this provider
		// authorized minutes ago must not still be redeemable once the owner cut
		// the provider off. The lock taken above serializes this delete with any
		// redemption, which takes the same lock before it consumes its code.
		_, err = s.handoffs.DeleteByUserProvider(ctx, userID, req.Provider)
		return err
	})
	if err != nil {
		return nil, err
	}
	// Same reason the link is announced, in reverse: losing a sign-in method is
	// worth detecting too, and on a passwordless account an unlink someone else
	// performed is a step towards locking the owner out. Best-effort and outside
	// the transaction — the identity is already gone, so a dead mailer must not
	// fail the unlink. The reclaim's identity sweep (ReclaimAccount) stays
	// silent: it runs inside a password reset the owner just completed, which
	// announces itself.
	if s.notifier != nil {
		if nerr := s.notifier.IdentityUnlinked(ctx, userID, s.providerName(req.Provider)); nerr != nil {
			logWarn(ctx, "oauth unlink-identity: identity-unlinked notice", nerr, "user_id", userID.String(), "provider", req.Provider)
		}
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
