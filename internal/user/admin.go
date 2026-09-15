// Admin use cases driven by the CLI (the resource:action commands), not the HTTP
// API: create-user, change-user-email, change-user-password, activate-user,
// deactivate-users. They reuse the same repo/encode/hasher/clock/tx seams the
// HTTP handlers use, so behavior matches the API exactly.
package user

import (
	"context"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

// AdminCreateUser creates a user regardless of ECONUMO_ALLOW_REGISTRATION and
// returns the new id.
func (s *Service) AdminCreateUser(ctx context.Context, name, email, password string) (vo.Id, error) {
	u, err := s.createUser(ctx, name, email, password, false)
	if err != nil {
		return vo.Id{}, err
	}
	return u.ID, nil
}

// mutateByEmail runs fn over the user identified by email, inside one
// transaction, under the user's row lock taken BEFORE the row is read: the
// lookup that resolves the email only decides whose lock to take, so a reclaim
// committing in between is re-read rather than overwritten by fn's
// whole-aggregate Save. fn owns the save and anything that must share its
// transaction. The users row is the first lock every path takes (the reclaim
// included), so these serialize and cannot deadlock.
func (s *Service) mutateByEmail(ctx context.Context, email string, fn func(ctx context.Context, u *model.User) error) (*model.User, error) {
	found, err := s.userByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	var saved *model.User
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		if lerr := s.repo.LockRow(ctx, found.ID); lerr != nil {
			return lerr
		}
		u, gerr := s.repo.GetByID(ctx, found.ID)
		if gerr != nil {
			return gerr
		}
		if ferr := fn(ctx, u); ferr != nil {
			return ferr
		}
		saved = u
		return nil
	}); err != nil {
		return nil, err
	}
	return saved, nil
}

// AdminChangeEmail changes a user's stored (encrypted) email, looked up by the
// current email. The avatar is left unchanged.
func (s *Service) AdminChangeEmail(ctx context.Context, oldEmail, newEmail string) error {
	loweredNew := strings.ToLower(strings.TrimSpace(newEmail))
	_, err := s.mutateByEmail(ctx, oldEmail, func(ctx context.Context, u *model.User) error {
		currentEmail, derr := s.encode.Decode(u.Email)
		if derr != nil {
			return derr
		}
		if loweredNew != strings.ToLower(currentEmail) {
			exists, eerr := s.repo.ExistsByEmail(ctx, loweredNew)
			if eerr != nil {
				return eerr
			}
			if exists {
				return &errs.ValidationError{Msg: "User already exists", MsgCode: errs.CodeUserAlreadyExists}
			}
		}
		encryptedEmail, eerr := s.encode.Encode(strings.TrimSpace(newEmail))
		if eerr != nil {
			return eerr
		}
		u.UpdateEmail(encryptedEmail, s.clock.Now())
		return s.repo.Save(ctx, u)
	})
	return err
}

// AdminChangePassword sets a user's password, rehashed with the current
// algorithm (argon2id), looked up by email. An operator setting a password is
// evicting whoever holds the account, so it is a full reclaim, exactly like a
// completed reset (see reclaimCredentials).
func (s *Service) AdminChangePassword(ctx context.Context, email, newPassword string) error {
	_, err := s.mutateByEmail(ctx, email, func(ctx context.Context, u *model.User) error {
		newHash, herr := s.hasher.Hash(newPassword)
		if herr != nil {
			return herr
		}
		// The operator is authoritative, so the account's own address is the
		// proven one: an identity vouching for it survives, everything else goes.
		provenEmail, derr := s.encode.Decode(u.Email)
		if derr != nil {
			return derr
		}
		u.UpdatePassword(newHash, model.AlgorithmArgon2id, s.clock.Now())
		if serr := s.repo.Save(ctx, u); serr != nil {
			return serr
		}
		return s.reclaimCredentials(ctx, u, provenEmail)
	})
	return err
}

// AdminActivate marks the user active, looked up by email.
func (s *Service) AdminActivate(ctx context.Context, email string) error {
	_, err := s.mutateByEmail(ctx, email, func(ctx context.Context, u *model.User) error {
		u.Activate(s.clock.Now())
		return s.repo.Save(ctx, u)
	})
	return err
}

// AdminDeactivate marks the user inactive, looked up by email, and revokes
// EVERY credential (sessions AND personal tokens) — this is why per-request
// authentication needs no is_active join: a deactivated user simply has no
// live tokens left. The revoke runs INSIDE the same transaction as the
// deactivation, and the credentials generation is bumped alongside it: a
// password login that read the row while still active must not be able to
// mint a session after this commits (see createSession's generation fence).
// Unlike a password reclaim, this must not unlink oauth identities or drop
// pending grants — deactivation is reversible, so those stay put for
// AdminActivate to hand back.
func (s *Service) AdminDeactivate(ctx context.Context, email string) error {
	_, err := s.mutateByEmail(ctx, email, func(ctx context.Context, u *model.User) error {
		u.Deactivate(s.clock.Now())
		if err := s.repo.Save(ctx, u); err != nil {
			return err
		}
		if err := s.repo.BumpCredentialsGeneration(ctx, u.ID); err != nil {
			return err
		}
		return s.revokeTokens(ctx, u.ID, vo.Id{}, s.clock.Now(), model.TokenKindSession, model.TokenKindPersonal)
	})
	return err
}

// AdminVerifyEmail marks a user's email verified (support/rescue hatch for
// the ECONUMO_EMAIL_VERIFICATION gate) and drops any pending code.
func (s *Service) AdminVerifyEmail(ctx context.Context, email string) error {
	_, err := s.mutateByEmail(ctx, email, func(ctx context.Context, u *model.User) error {
		u.MarkEmailVerified(s.clock.Now())
		if err := s.repo.Save(ctx, u); err != nil {
			return err
		}
		return s.emailVerifications.DeleteByUser(ctx, u.ID)
	})
	return err
}

// AdminSetAccess sets a user's access level and optional expiry, looked up by
// email. A nil until means the level never expires. The user is returned so
// the CLI can log the action keyed by id (log lines carry ids, never emails).
func (s *Service) AdminSetAccess(ctx context.Context, email string, level model.AccessLevel, until *time.Time) (*model.User, error) {
	return s.mutateByEmail(ctx, email, func(ctx context.Context, u *model.User) error {
		u.SetAccess(level, until, s.clock.Now())
		return s.repo.Save(ctx, u)
	})
}

// AdminShowUser looks up a user by email and returns the raw stored access
// alongside the effective level (collapsed against now) so an operator gets
// both the inputs and the answer in one call.
func (s *Service) AdminShowUser(ctx context.Context, email string) (*model.User, model.AccessLevel, error) {
	u, err := s.userByEmail(ctx, email)
	if err != nil {
		return nil, "", err
	}
	return u, u.EffectiveAccessLevel(s.clock.Now()), nil
}

// userByEmail resolves a user from a plaintext email (case-insensitive). A
// miss returns the repo's NotFound error.
func (s *Service) userByEmail(ctx context.Context, email string) (*model.User, error) {
	return s.repo.GetByEmail(ctx, strings.TrimSpace(email))
}

// AdminUserByID loads a user by id with the email decrypted: the admin
// listener (which addresses users by id, never by email), and any other
// caller that only holds an id (e.g. the oauth identity-linked notifier).
func (s *Service) AdminUserByID(ctx context.Context, id vo.Id) (*model.User, string, error) {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, "", err
	}
	email, err := s.encode.Decode(u.Email)
	if err != nil {
		return nil, "", err
	}
	return u, email, nil
}

// AdminSetAccessByID is AdminSetAccess keyed by id: an operator running the CLI
// has an email address, the payment portal has a user id. It returns the user
// as written (plus the decrypted email) so the caller can build its response
// without a second read that a concurrent write could have moved past.
func (s *Service) AdminSetAccessByID(ctx context.Context, id vo.Id, level model.AccessLevel, until *time.Time) (*model.User, string, error) {
	u, err := s.mutate(ctx, id, func(u *model.User, now time.Time) error {
		// The prior state on the audit line distinguishes a webhook retry (same
		// -> same) from a real change; it is read under the row lock, so it is
		// the state this write actually replaced. Ids and levels only — never
		// the email.
		reqctx.AddLogAttr(ctx, "old_access_level", string(u.AccessLevel))
		oldUntil := ""
		if u.AccessUntil != nil {
			oldUntil = u.AccessUntil.UTC().Format(datetime.Layout)
		}
		reqctx.AddLogAttr(ctx, "old_access_until", oldUntil)
		u.SetAccess(level, until, now)
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	email, err := s.encode.Decode(u.Email)
	if err != nil {
		return nil, "", err
	}
	return u, email, nil
}
