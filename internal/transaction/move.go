package transaction

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

var (
	errDayChanged = &errs.ValidationError{Msg: "This day changed meanwhile. Refresh and try again.", MsgCode: errs.CodeTransactionDayChanged}
	errCannotMove = &errs.ValidationError{Msg: "This transaction can't be moved to another day.", MsgCode: errs.CodeTransactionCannotMove}
)

// MoveTransaction puts a transaction where the user dropped it on an account
// page: req.TransactionIds is that account's whole day afterwards, top first.
// Only that account's side keys change, so the other account of a transfer
// keeps its order; a drop into another day also changes the date, keeping the
// time. The client sends what it showed, so a day that changed meanwhile (a
// row added or moved elsewhere) is refused rather than guessed at.
func (s *Service) MoveTransaction(ctx context.Context, userID vo.Id, req model.MoveTransactionRequest) (*model.MoveTransactionResult, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	accountID, err := vo.ParseId(req.AccountId)
	if err != nil {
		return nil, err
	}
	day, err := time.Parse(datetime.DateLayout, req.Date)
	if err != nil {
		return nil, errs.NewValidation("Validation failed",
			errs.FieldError{Key: "date", Message: "Invalid date format, expected Y-m-d", Code: errs.CodeInvalidDatetimeFormat})
	}
	order := make([]vo.Id, 0, len(req.TransactionIds))
	for _, raw := range req.TransactionIds {
		oid, perr := vo.ParseId(raw)
		if perr != nil || containsID(order, oid) {
			return nil, errDayChanged
		}
		order = append(order, oid)
	}

	var touched []vo.Id
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		if aerr := s.checkWriteAccess(ctx, userID, accountID, "account.account.not_available"); aerr != nil {
			return aerr
		}
		t, gerr := s.repo.GetByID(ctx, id)
		if gerr != nil {
			return gerr
		}
		onAccount := t.AccountID.Equal(accountID) || (t.AccountRecipID != nil && t.AccountRecipID.Equal(accountID))
		if !onAccount {
			return &errs.ValidationError{Msg: "transaction.transaction.not_available", MsgCode: notAvailableCode("transaction.transaction.not_available")}
		}
		if t.SpentAt.Format(datetime.DateLayout) != req.Date {
			if merr := s.moveToDay(ctx, userID, t, day); merr != nil {
				return merr
			}
			touched = append(touched, t.ID)
		}
		keys, kerr := s.repo.DayKeys(ctx, accountID, day)
		if kerr != nil {
			return kerr
		}
		if len(keys) != len(order) {
			return errDayChanged
		}
		for _, oid := range order {
			if _, ok := keys[oid.String()]; !ok {
				return errDayChanged
			}
		}
		for i, oid := range order {
			want := int64(len(order) - i)
			if keys[oid.String()] == want {
				continue
			}
			if serr := s.repo.SetSideKey(ctx, accountID, oid, want); serr != nil {
				return serr
			}
			if !containsID(touched, oid) {
				touched = append(touched, oid)
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	labels, err := s.repo.LabelsByTransactionIDs(ctx, touched)
	if err != nil {
		return nil, err
	}
	items := make([]model.TransactionResult, 0, len(touched))
	for _, tid := range touched {
		t, gerr := s.repo.GetByID(ctx, tid)
		if gerr != nil {
			return nil, gerr
		}
		item, rerr := s.toResult(ctx, t, labels[tid.String()])
		if rerr != nil {
			return nil, rerr
		}
		items = append(items, item)
	}
	accounts, err := s.accountListEmbed(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &model.MoveTransactionResult{Items: items, Accounts: accounts}, nil
}

// moveToDay changes t's date to day's, keeping its time of day. That edits the
// transaction itself, so it needs what an edit needs: write access to every
// account it touches. Update drops both side keys, so the row goes to the top
// of its new day on the other side of a transfer, and the caller's order then
// places it on this side.
func (s *Service) moveToDay(ctx context.Context, userID vo.Id, t *model.Transaction, day time.Time) error {
	for _, p := range partySet(t) {
		if err := s.checkWriteAccess(ctx, userID, p, "account.account.not_available"); err != nil {
			return errCannotMove
		}
	}
	at := t.SpentAt
	spentAt := time.Date(day.Year(), day.Month(), day.Day(), at.Hour(), at.Minute(), at.Second(), 0, time.UTC)
	t.Update(model.NewState{
		Type: t.Type, AccountID: t.AccountID, AccountRecipID: t.AccountRecipID,
		Amount: t.Amount, AmountRecipient: t.AmountRecipient,
		CategoryID: t.CategoryID, PayeeID: t.PayeeID, TagID: t.TagID, LabelIDs: t.LabelIDs,
		Description: t.Description, SpentAt: spentAt,
	}, s.clock.Now())
	if err := s.repo.Save(ctx, t); err != nil {
		return err
	}
	for _, p := range partySet(t) {
		if err := s.zeroer.ZeroDeleted(ctx, p, spentAt, correctionEditedTx); err != nil {
			return err
		}
	}
	return nil
}
