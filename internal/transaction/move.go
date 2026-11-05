package transaction

import (
	"context"
	"sort"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/sortkey"
	"github.com/econumo/econumo/internal/shared/vo"
)

var (
	errDayChanged = &errs.ValidationError{Msg: "This day changed meanwhile. Refresh and try again.", MsgCode: errs.CodeTransactionDayChanged}
	errCannotMove = &errs.ValidationError{Msg: "This transaction can't be moved to another day.", MsgCode: errs.CodeTransactionCannotMove}
)

// MoveTransaction puts a transaction where the user dropped it on an account
// page: directly below req.AboveId on that account's day, or at the top of the
// day when AboveId is null. Only the moved row's key on that account's side
// changes, so the rest of the day is never renumbered and the other account of
// a transfer keeps its order; a drop into another day also changes the date,
// keeping the time. What the page showed is checked before anything is written:
// the row must still be on req.FromDate, or a drag within a stale day would
// quietly undo another device's date change, and an AboveId no longer on the
// target day (moved or deleted elsewhere meanwhile) is refused rather than
// guessed at.
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
	aboveID := ""
	if req.AboveId != nil {
		aboveID = *req.AboveId
	}
	if aboveID == req.Id {
		return nil, errDayChanged
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
		if t.SpentAt.Format(datetime.DateLayout) != req.FromDate {
			return errDayChanged
		}
		if req.FromDate != req.Date {
			if merr := s.moveToDay(ctx, userID, t, day); merr != nil {
				return merr
			}
		}
		keys, kerr := s.repo.DayKeys(ctx, accountID, day)
		if kerr != nil {
			return kerr
		}
		delete(keys, t.ID.String())
		if aboveID != "" {
			if _, ok := keys[aboveID]; !ok {
				return errDayChanged
			}
		}
		order := dayOrder(keys)
		split, serr := s.splitTies(ctx, accountID, order)
		if serr != nil {
			return serr
		}
		key, berr := keyBelow(order, aboveID)
		if berr != nil {
			return berr
		}
		if werr := s.repo.SetSideKey(ctx, accountID, t.ID, key); werr != nil {
			return werr
		}
		touched = append([]vo.Id{t.ID}, split...)
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

type placedRow struct {
	id  string
	key string
}

// dayOrder lists a day's rows as the list shows them: key descending, id
// ascending among equal keys.
func dayOrder(keys map[string]string) []placedRow {
	out := make([]placedRow, 0, len(keys))
	for id, key := range keys {
		out = append(out, placedRow{id: id, key: key})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].key != out[j].key {
			return out[i].key > out[j].key
		}
		return out[i].id < out[j].id
	})
	return out
}

// splitTies gives every row that shares its key with the row above it (two
// inserts that raced for the same key) a key of its own, keeping the order the
// list already shows, so a key strictly between any two neighbours exists. The
// top row of a tied run keeps its key; the rest go just below it. It rewrites
// day in place and returns the ids it re-keyed.
func (s *Service) splitTies(ctx context.Context, accountID vo.Id, day []placedRow) ([]vo.Id, error) {
	var split []vo.Id
	for start := 0; start < len(day); {
		end := start + 1
		for end < len(day) && day[end].key == day[start].key {
			end++
		}
		if end-start > 1 {
			floor := ""
			if end < len(day) {
				floor = day[end].key
			}
			prev := floor
			for i := end - 1; i > start; i-- {
				key, err := sortkey.Between(sortkey.Key(prev), sortkey.Key(day[start].key))
				if err != nil {
					return nil, err
				}
				rid, err := vo.ParseId(day[i].id)
				if err != nil {
					return nil, err
				}
				if err := s.repo.SetSideKey(ctx, accountID, rid, string(key)); err != nil {
					return nil, err
				}
				day[i].key, prev = string(key), string(key)
				split = append(split, rid)
			}
		}
		start = end
	}
	return split, nil
}

// keyBelow is the key that places a row directly below aboveID in day (the top
// of the day when aboveID is empty).
func keyBelow(day []placedRow, aboveID string) (string, error) {
	if aboveID == "" {
		top := ""
		if len(day) > 0 {
			top = day[0].key
		}
		key, err := sortkey.Between(sortkey.Key(top), "")
		return string(key), err
	}
	for i, r := range day {
		if r.id != aboveID {
			continue
		}
		below := ""
		if i+1 < len(day) {
			below = day[i+1].key
		}
		key, err := sortkey.Between(sortkey.Key(below), sortkey.Key(r.key))
		return string(key), err
	}
	return "", errDayChanged
}

// moveToDay changes t's date to day's, keeping its time of day. That edits the
// transaction itself, so it needs what an edit needs: write access to every
// account it touches. Update drops both side keys, so the row goes to the top
// of its new day on the other side of a transfer, and the caller then places
// it on this side.
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
