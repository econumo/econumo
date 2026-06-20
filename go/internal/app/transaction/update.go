// Update + delete use cases.
package transaction

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
	domtransaction "github.com/econumo/econumo/internal/domain/transaction"
)

// parseSpentAt parses the wire date ("Y-m-d H:i:s").
func parseSpentAt(v string) (time.Time, error) {
	t, err := time.Parse(apiDatetimeLayout, v)
	if err != nil {
		return time.Time{}, errs.NewValidation("Validation failed",
			errs.FieldError{Key: "date", Message: "Invalid date format, expected Y-m-d H:i:s"})
	}
	return t, nil
}

// UpdateTransaction applies a full update to the transaction (access required on
// the target account) and returns it plus the refreshed account list.
func (s *Service) UpdateTransaction(ctx context.Context, userID vo.Id, req UpdateTransactionRequest) (*UpdateTransactionResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	typ, err := parseType(req.Type)
	if err != nil {
		return nil, err
	}
	accountID, err := vo.ParseId(req.AccountId)
	if err != nil {
		return nil, err
	}
	spentAt, err := parseSpentAt(req.Date)
	if err != nil {
		return nil, err
	}
	description := ""
	if req.Description != nil {
		description = *req.Description
	}

	var updated *domtransaction.Transaction
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		if aerr := s.checkAccountOwned(ctx, userID, accountID); aerr != nil {
			return aerr
		}
		t, gerr := s.repo.GetByID(ctx, id)
		if gerr != nil {
			return gerr
		}
		now := s.clock.Now()
		st, berr := buildState(id, userID, typ, accountID, req.Amount,
			req.AmountRecipient, req.AccountRecipientId, req.CategoryId, req.PayeeId, req.TagId,
			description, spentAt, now)
		if berr != nil {
			return berr
		}
		t.Update(st, now)
		if serr := s.repo.Save(ctx, t); serr != nil {
			return serr
		}
		updated = t
		return nil
	}); err != nil {
		return nil, err
	}

	item, err := s.toResult(ctx, updated)
	if err != nil {
		return nil, err
	}
	accounts, err := s.accountListEmbed(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &UpdateTransactionResult{Item: item, Accounts: accounts}, nil
}
