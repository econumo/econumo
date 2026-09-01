package account

import (
	"context"
	"log/slog"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// allTimeBalanceBound is the exclusive spent_at bound that includes every
// transaction, future-dated ones included: a deleted account must end at
// exactly zero, not "zero as of today".
var allTimeBalanceBound = time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)

// balanceScale is the transactions.amount NUMERIC(19,8) scale; a residue below
// it cannot be expressed by any transaction, so it never warrants a correction.
const balanceScale = 8

// ZeroDeletedAccounts writes a balance-correction transaction for every
// soft-deleted account whose all-time balance is non-zero, dated at the
// account's deletion time (UpdatedAt). Idempotent; returns the number written.
func (s *Service) ZeroDeletedAccounts(ctx context.Context) (int, error) {
	accounts, err := s.accounts.ListDeleted(ctx)
	if err != nil {
		return 0, err
	}
	written := 0
	for _, acct := range accounts {
		var wrote bool
		err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
			var zerr error
			wrote, zerr = s.zeroBalance(txCtx, acct, acct.UpdatedAt, s.clock.Now(), correctionAccountDeleted)
			return zerr
		})
		if err != nil {
			return written, err
		}
		if wrote {
			written++
		}
	}
	return written, nil
}

// ZeroIfDeleted re-reconciles a DELETED account to zero with one correction
// dated spentAt, reporting whether a row was written; a live account is a no-op.
// It is the §5a self-healing entry point the transaction feature drives after a
// write that moved a deleted account's balance.
//
// Deliberately WITHOUT a WithTx of its own: it must join the caller's
// transaction (both features' repos take the tx from the context), so the
// transaction write and its correction commit or roll back together.
func (s *Service) ZeroIfDeleted(ctx context.Context, accountID vo.Id, spentAt time.Time, description string) (bool, error) {
	acct, err := s.accounts.GetByID(ctx, accountID)
	if err != nil {
		return false, err
	}
	if !acct.IsDeleted {
		return false, nil
	}
	return s.zeroBalance(ctx, acct, spentAt, s.clock.Now(), description)
}

// zeroBalance reconciles acct to a zero balance with one correction dated
// spentAt (the same sign rule as update-account: a positive balance is removed
// by an EXPENSE, a negative one filled by an INCOME). The description says
// which flow zeroed it (account deletion vs a §5a self-healing write).
// Returns whether a row was written.
func (s *Service) zeroBalance(ctx context.Context, acct *model.Account, spentAt, now time.Time, description string) (bool, error) {
	raw, err := s.balances.Balance(ctx, acct.ID, allTimeBalanceBound)
	if err != nil {
		return false, err
	}
	balance := vo.NewDecimal(raw).Round(balanceScale)
	if balance.IsZero() {
		return false, nil
	}
	corrType := int16(0) // expense
	if balance.IsNegative() {
		corrType = 1 // income
	}
	corr := model.AccountCorrection{
		ID:          s.accounts.NextIdentity(),
		UserID:      acct.UserID,
		AccountID:   acct.ID,
		Description: description,
		Type:        corrType,
		Amount:      balance.Abs().String(),
		SpentAt:     spentAt,
		CreatedAt:   now,
	}
	if err := s.accounts.SaveCorrection(ctx, corr); err != nil {
		return false, err
	}
	slog.InfoContext(ctx, "zero-account-balance", "account_id", acct.ID.String(), "amount", corr.Amount, "type", corrType)
	return true, nil
}
