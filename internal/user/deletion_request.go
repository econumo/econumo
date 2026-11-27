package user

import (
	"context"
	"errors"
	"strings"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

var errDeletionNotWired = errors.New("user deletion is not wired")

// RequestUserDeletion emails a code that confirms the deletion. The password
// is checked only where one can be used to sign in: a passwordless user, or
// any user on a server with password sign-in disabled, has only the mailbox.
func (s *Service) RequestUserDeletion(ctx context.Context, userID vo.Id, req model.RequestUserDeletionRequest) (*model.RequestUserDeletionResult, error) {
	if s.deletionRequests == nil || s.deletionSender == nil {
		return nil, errDeletionNotWired
	}
	if err := s.countAttempt(ctx, RateScopeRequestUserDeletion, userID.String()); err != nil {
		return nil, err
	}
	u, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if u.Algorithm != model.AlgorithmNone && !s.passwordLoginDisabled &&
		!s.hasher.Verify(u.Algorithm, u.Password, req.Password, u.Salt) {
		return nil, &errs.ValidationError{Msg: "Password is not correct", MsgCode: errs.CodeUserPasswordIncorrect}
	}
	to, err := s.encode.Decode(u.Email)
	if err != nil {
		return nil, err
	}
	code, err := generatePasswordCode()
	if err != nil {
		return nil, err
	}
	r := model.NewDeletionRequest(vo.NewId(), u.ID, u.Email, HashResetCode(code), s.clock.Now())
	// Fenced on the generation read with the verified password, under the row
	// lock, for the reasons given on issueEmailChangeCode.
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		if lerr := s.repo.LockRow(ctx, u.ID); lerr != nil {
			return lerr
		}
		if derr := s.deletionRequests.DeleteByUser(ctx, u.ID); derr != nil {
			return derr
		}
		rows, serr := s.deletionRequests.Save(ctx, r, u.CredentialsGeneration)
		if serr != nil {
			return serr
		}
		if rows != 1 {
			return errs.NewUnauthorized("Invalid access token")
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if err := s.deletionSender.SendDeletionCode(ctx, strings.TrimSpace(to), u.Name, code); err != nil {
		return nil, err
	}
	return &model.RequestUserDeletionResult{}, nil
}

// ConfirmUserDeletion checks the code and marks the user, which revokes every
// credential, the presenting session included.
func (s *Service) ConfirmUserDeletion(ctx context.Context, userID vo.Id, req model.ConfirmUserDeletionRequest) (*model.ConfirmUserDeletionResult, error) {
	if s.deletionRequests == nil || s.deletion == nil {
		return nil, errDeletionNotWired
	}
	key := userID.String()
	if err := s.allowAttempt(ctx, RateScopeConfirmUserDeletion, key); err != nil {
		return nil, err
	}
	attempt, err := s.reserveAttempt(RateScopeConfirmUserDeletion, key)
	if err != nil {
		return nil, err
	}
	defer attempt.giveBack()
	invalid := &errs.ValidationError{Msg: "The confirmation code is not valid.", MsgCode: errs.CodeUserVerificationCodeInvalid}

	r, err := s.deletionRequests.GetByUser(ctx, userID)
	if err != nil {
		if isNotFound(err) {
			attempt.keep()
			return nil, invalid
		}
		return nil, err
	}
	if HashResetCode(strings.TrimSpace(req.Code)) != r.Code {
		attempt.keep()
		return nil, invalid
	}
	expired := &errs.ValidationError{Msg: "The code is expired", MsgCode: errs.CodeUserVerificationCodeExpired}
	if r.IsExpired(s.clock.Now()) {
		attempt.keep()
		return nil, expired
	}
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		if lerr := s.repo.LockRow(ctx, userID); lerr != nil {
			return lerr
		}
		u, gerr := s.repo.GetByID(ctx, userID)
		if gerr != nil {
			return gerr
		}
		// The code proves the mailbox it was sent to, and an email change does
		// not bump the credentials generation, so the address is compared here.
		if u.Email != r.Email {
			return invalid
		}
		// Again under the lock: the confirm may have waited behind another
		// writer of the row while the code ran out.
		if r.IsExpired(s.clock.Now()) {
			return expired
		}
		taken, cerr := s.deletionRequests.Consume(ctx, r.ID, userID)
		if cerr != nil {
			return cerr
		}
		if taken != 1 {
			return invalid
		}
		return s.markForDeletion(ctx, userID, model.DeletionReasonSelf)
	}); err != nil {
		if errors.Is(err, invalid) || errors.Is(err, expired) {
			attempt.keep()
		}
		return nil, err
	}
	s.clearAttempt(RateScopeConfirmUserDeletion, key)
	attempt.keep()
	return &model.ConfirmUserDeletionResult{}, nil
}
