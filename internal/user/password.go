// Password use cases: change, remind, and reset the password.
package user

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// passwordCodeDigits is the length of an emailed reset/verification code.
// Six digits is short enough to retype from a phone; the brute-force margin
// comes from the per-username attempt caps and the code's short TTL, not from
// the size of the code space.
const passwordCodeDigits = 6

// generatePasswordCode returns a fresh 6-digit numeric code, zero-padded so
// every code is exactly passwordCodeDigits long.
func generatePasswordCode() (string, error) {
	max := big.NewInt(1)
	for i := 0; i < passwordCodeDigits; i++ {
		max.Mul(max, big.NewInt(10))
	}
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%0*d", passwordCodeDigits, n), nil
}

// HashResetCode maps a reset code to its at-rest storage/lookup key. The
// plaintext code is emailed to the user; only this sha256 hex is persisted, so a
// database disclosure does not hand out usable reset codes.
func HashResetCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

func isNotFound(err error) bool {
	var nf *errs.NotFoundError
	return errors.As(err, &nf)
}

// UpdatePassword is a credential rotation: it verifies the old password, then
// in ONE locked transaction writes the new hash (fenced on the generation the
// old hash was read under), bumps the generation, drops the grants issued under
// the old password and revokes every OTHER session. A wrong old password yields
// a ValidationError -> 400 ("Password is not correct"). The presenting session
// (currentTokenID), PATs and linked identities survive — the owner is acting,
// not recovering.
func (s *Service) UpdatePassword(ctx context.Context, userID vo.Id, currentTokenID vo.Id, req model.UpdatePasswordRequest) (*model.UpdatePasswordResult, error) {
	if err := s.requirePasswordLogin(); err != nil {
		return nil, err
	}
	incorrect := &errs.ValidationError{Msg: "Password is not correct", MsgCode: errs.CodeUserPasswordIncorrect}
	u, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	// Verify and hash before the lock: argon2 must never run under the row lock.
	if !s.hasher.Verify(u.Algorithm, u.Password, req.OldPassword, u.Salt) {
		return nil, incorrect
	}
	newHash, herr := s.hasher.Hash(req.NewPassword)
	if herr != nil {
		return nil, herr
	}
	now := s.clock.Now()
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.repo.LockRow(ctx, userID); err != nil {
			return err
		}
		// The fence: the hash we verified belongs to the generation we read it
		// under; if a reclaim or another rotation moved it, this rotation
		// proved nothing and writes nothing.
		n, err := s.repo.UpdatePasswordIfGeneration(ctx, userID, newHash, u.Salt, model.AlgorithmArgon2id, now, u.CredentialsGeneration)
		if err != nil {
			return err
		}
		if n != 1 {
			return incorrect
		}
		// A rotation invalidates every flow that read the old credential:
		// in-flight logins (the bump), pending reset codes and email changes
		// (grants issued under the old password), and the other sessions.
		if err := s.repo.BumpCredentialsGeneration(ctx, userID); err != nil {
			return err
		}
		if err := s.passwordRequests.DeleteByUser(ctx, userID); err != nil {
			return err
		}
		if err := s.emailChangeRequests.DeleteByUser(ctx, userID); err != nil {
			return err
		}
		return s.revokeTokens(ctx, userID, currentTokenID, now, model.TokenKindSession)
	}); err != nil {
		return nil, err
	}
	return &model.UpdatePasswordResult{}, nil
}

// RemindPassword issues a password-reset code: it replaces the user's existing
// codes with a fresh one (10-min expiry) and emails it. A missing user is hidden
// (returns success) to avoid account enumeration.
func (s *Service) RemindPassword(ctx context.Context, req model.RemindPasswordRequest) (*model.RemindPasswordResult, error) {
	if err := s.requirePasswordLogin(); err != nil {
		return nil, err
	}
	lowered := strings.ToLower(strings.TrimSpace(req.Username))
	if err := s.allowAttempt(RateScopeRemind, lowered); err != nil {
		return nil, err
	}
	s.failAttempt(RateScopeRemind, lowered) // every remind sends an email, so every request counts

	u, err := s.repo.GetByEmail(ctx, lowered)
	if err != nil {
		if isNotFound(err) {
			return &model.RemindPasswordResult{}, nil // anti-enumeration
		}
		return nil, err
	}

	code, err := generatePasswordCode()
	if err != nil {
		return nil, err
	}
	pr := model.NewPasswordRequest(vo.NewId(), u.ID, HashResetCode(code), s.clock.Now())
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		// Issuing a code and consuming one serialize on the same row lock the
		// reset takes, so "issue B" and "consume A" have one defined order:
		// without it a reset could consume a code this remind has already
		// replaced.
		if lerr := s.repo.LockRow(ctx, u.ID); lerr != nil {
			return lerr
		}
		if derr := s.passwordRequests.DeleteByUser(ctx, u.ID); derr != nil {
			return derr
		}
		return s.passwordRequests.Save(ctx, pr)
	}); err != nil {
		return nil, err
	}

	// Email the code to the address the caller submitted (trimmed, original case).
	if s.mailer != nil {
		if err := s.mailer.SendResetPasswordCode(ctx, strings.TrimSpace(req.Username), u.Name, code); err != nil {
			return nil, err
		}
	}
	return &model.RemindPasswordResult{}, nil
}

// ResetPassword validates the (email, code) reset request, and on success sets
// the new password and consumes the code. An unknown user/code yields a generic
// validation error; an expired code yields the frozen "The code is expired".
func (s *Service) ResetPassword(ctx context.Context, req model.ResetPasswordRequest) (*model.ResetPasswordResult, error) {
	if err := s.requirePasswordLogin(); err != nil {
		return nil, err
	}
	lowered := strings.ToLower(strings.TrimSpace(req.Username))
	if err := s.allowAttempt(RateScopeReset, lowered); err != nil {
		return nil, err
	}
	u, err := s.repo.GetByEmail(ctx, lowered)
	if err != nil {
		if isNotFound(err) {
			s.failAttempt(RateScopeReset, lowered)
			return nil, &errs.ValidationError{Msg: "Reset password error", MsgCode: errs.CodeUserResetPasswordError}
		}
		return nil, err
	}

	hashedCode := HashResetCode(strings.TrimSpace(req.Code))
	pr, err := s.passwordRequests.GetByUserAndCode(ctx, u.ID, hashedCode)
	if err != nil {
		if isNotFound(err) {
			s.failAttempt(RateScopeReset, lowered)
			return nil, &errs.ValidationError{Msg: "Reset password error", MsgCode: errs.CodeUserResetPasswordError}
		}
		return nil, err
	}
	if pr.IsExpired(s.clock.Now()) {
		s.failAttempt(RateScopeReset, lowered)
		return nil, &errs.ValidationError{Msg: "The code is expired", MsgCode: errs.CodeUserResetCodeExpired}
	}

	newHash, herr := s.hasher.Hash(req.Password)
	if herr != nil {
		return nil, herr
	}
	// A completed reset is the account's ownership proof: it is the one flow
	// that demonstrates control of the mailbox. So everything that could sign in
	// WITHOUT that proof goes with the old password — see reclaimCredentials,
	// which runs inside the password write's transaction so a half-done reclaim
	// cannot happen.
	userID := u.ID
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		// The row lock first, then a fresh read: the lookups above ran outside
		// this transaction, and saving that aggregate back would undo whatever
		// committed in between (a confirmed email change, a renamed profile).
		// Every other writer takes the same lock first, so they serialize.
		if lerr := s.repo.LockRow(ctx, userID); lerr != nil {
			return lerr
		}
		u, gerr := s.repo.GetByID(ctx, userID)
		if gerr != nil {
			return gerr
		}
		// The code proves control of `lowered` and of nothing else, so the row
		// the lock handed back must still be that address's account: a change of
		// email confirmed since the lookup above means this reset would set the
		// password (and the verified stamp) on someone else's login key.
		cur, derr := s.encode.Decode(u.Email)
		if derr != nil {
			return derr
		}
		if strings.ToLower(strings.TrimSpace(cur)) != lowered {
			return &errs.ValidationError{Msg: "Reset password error", MsgCode: errs.CodeUserResetPasswordError}
		}
		// The code is the evidence, so it is re-read and consumed under the
		// same lock: the read above ran before it, and a remind that replaced
		// the code (or a concurrent reset that already consumed it) in between
		// leaves nothing to take. Row-counted, so two resets holding one code
		// cannot both succeed with the later password winning.
		held, lookupErr := s.passwordRequests.GetByUserAndCode(ctx, u.ID, hashedCode)
		if lookupErr != nil {
			if isNotFound(lookupErr) {
				return &errs.ValidationError{Msg: "Reset password error", MsgCode: errs.CodeUserResetPasswordError}
			}
			return lookupErr
		}
		if held.IsExpired(s.clock.Now()) {
			return &errs.ValidationError{Msg: "The code is expired", MsgCode: errs.CodeUserResetCodeExpired}
		}
		consumed, consumeErr := s.passwordRequests.Consume(ctx, held.ID, u.ID)
		if consumeErr != nil {
			return consumeErr
		}
		if consumed != 1 {
			return &errs.ValidationError{Msg: "Reset password error", MsgCode: errs.CodeUserResetPasswordError}
		}
		u.UpdatePassword(newHash, model.AlgorithmArgon2id, s.clock.Now())
		// Completing a reset proves mailbox ownership, so it also satisfies the
		// email-verification gate.
		u.MarkEmailVerified(s.clock.Now())
		if serr := s.repo.Save(ctx, u); serr != nil {
			return serr
		}
		return s.reclaimCredentials(ctx, u, lowered)
	}); err != nil {
		return nil, err
	}
	s.clearAttempt(RateScopeReset, lowered)
	return &model.ResetPasswordResult{}, nil
}
