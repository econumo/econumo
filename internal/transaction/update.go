package transaction

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// parseSpentAt parses the wire date ("Y-m-d H:i:s").
func parseSpentAt(v string) (time.Time, error) {
	t, err := time.Parse(datetime.Layout, v)
	if err != nil {
		return time.Time{}, errs.NewValidation("Validation failed",
			errs.FieldError{Key: "date", Message: "Invalid date format, expected Y-m-d H:i:s", Code: errs.CodeInvalidDatetimeFormat})
	}
	return t, nil
}

// UpdateTransaction applies a full update to the transaction (access required on
// the target account) and returns it plus the refreshed account list. This is
// the REST contract: req.LabelIds fully replaces the label set, same as
// categoryId/payeeId/tagId (omitting it clears).
func (s *Service) UpdateTransaction(ctx context.Context, userID vo.Id, req model.UpdateTransactionRequest) (*model.UpdateTransactionResult, error) {
	return s.updateTransaction(ctx, userID, req, false)
}

// UpdateTransactionPreservingLabels is UpdateTransaction except the
// transaction's existing labels are left exactly as they are, instead of
// being replaced by req.LabelIds. Used exclusively by the MCP
// update_transaction tool: MCP has no label_ids argument (a later plan owns
// adding one), so req.LabelIds is always its zero value — routing that
// through the normal full-replace path would silently delete every label a
// transaction had. A future MCP label_ids argument should switch MCP back to
// plain UpdateTransaction once it can supply a real value.
func (s *Service) UpdateTransactionPreservingLabels(ctx context.Context, userID vo.Id, req model.UpdateTransactionRequest) (*model.UpdateTransactionResult, error) {
	return s.updateTransaction(ctx, userID, req, true)
}

func (s *Service) updateTransaction(ctx context.Context, userID vo.Id, req model.UpdateTransactionRequest, preserveLabels bool) (*model.UpdateTransactionResult, error) {
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

	var updated *model.Transaction
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		if aerr := s.checkWriteAccess(ctx, userID, accountID, "account.account.not_available"); aerr != nil {
			return aerr
		}
		t, gerr := s.repo.GetByID(ctx, id)
		if gerr != nil {
			return gerr
		}
		// The row being mutated must ALSO be on an account the caller may write to
		// — otherwise a valid transaction UUID plus any account the caller owns
		// would let them overwrite and relocate a stranger's transaction.
		if aerr := s.checkWriteAccess(ctx, userID, t.AccountID, "transaction.transaction.not_available"); aerr != nil {
			return aerr
		}
		labelIDs := req.LabelIds
		if preserveLabels {
			// Seed with what's ACTUALLY persisted (not req.LabelIds, which the
			// preserving caller never populates), so checkReferences's full
			// replace below ends up replacing the label set with itself — a
			// true no-op — rather than clearing it. One lookup, scoped to this
			// transaction; unlike the (reverted) bulk-update attempt at this
			// same pattern, the result is genuinely used by the Save+
			// ReplaceLabels calls that follow.
			existing, lerr := s.repo.LabelsByTransactionIDs(ctx, []vo.Id{id})
			if lerr != nil {
				return lerr
			}
			labelIDs = existing[id.String()]
		}
		now := s.clock.Now()
		st, berr := buildState(id, userID, typ, accountID, req.Amount.String(),
			req.AmountRecipient.StrPtr(), req.AccountRecipientId, req.CategoryId, req.PayeeId, req.TagId,
			description, spentAt, now)
		if berr != nil {
			return berr
		}
		if nerr := s.normalizeTransferAmounts(ctx, &st); nerr != nil {
			return nerr
		}
		if rerr := s.checkReferences(ctx, userID, &st, labelIDs); rerr != nil {
			return rerr
		}
		t.Update(st, now)
		if serr := s.repo.Save(ctx, t); serr != nil {
			return serr
		}
		if lerr := s.repo.ReplaceLabels(ctx, t.ID, t.LabelIDs); lerr != nil {
			return lerr
		}
		updated = t
		return nil
	}); err != nil {
		return nil, err
	}

	item, err := s.toResult(ctx, updated, labelIDStrings(updated.LabelIDs))
	if err != nil {
		return nil, err
	}
	accounts, err := s.accountListEmbed(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &model.UpdateTransactionResult{Item: item, Accounts: accounts}, nil
}
