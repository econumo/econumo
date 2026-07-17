package recurring

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *Service) CreateRecurringTransaction(ctx context.Context, userID vo.Id, req model.CreateRecurringTransactionRequest) (*model.CreateRecurringTransactionResult, error) {
	opID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	st, err := s.buildState(req.Type, req.AccountId, req.AccountRecipientId, req.Amount.String(),
		req.CategoryId, req.PayeeId, req.TagId, req.Description, req.Schedule, req.NextPaymentAt)
	if err != nil {
		return nil, err
	}
	st.ID = vo.NewId()
	st.UserID = userID

	var created *model.RecurringTransaction
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		if aerr := s.checkWriteAccess(ctx, userID, st.AccountID); aerr != nil {
			return aerr
		}
		now := s.clock.Now()
		already, cerr := s.ops.Claim(ctx, opID, now)
		if cerr != nil {
			return cerr
		}
		if already {
			return &errs.ValidationError{Msg: "Operation is locked", MsgCode: errs.CodeOperationLocked}
		}
		st.CreatedAt = now
		created = model.NewRecurringTransaction(st)
		if serr := s.repo.Save(ctx, created); serr != nil {
			return serr
		}
		return s.ops.MarkHandled(ctx, opID, now)
	}); err != nil {
		return nil, err
	}
	return &model.CreateRecurringTransactionResult{Item: toResult(created)}, nil
}

// buildState parses and validates the shared create/update payload into a
// RecurringNewState (ID/UserID/CreatedAt left for the caller).
func (s *Service) buildState(typAlias, accountID string, accountRecipID *string, amount string,
	categoryID, payeeID, tagID *string, description *string, schedule, nextPaymentAt string) (model.RecurringNewState, error) {
	var st model.RecurringNewState

	typ, err := parseType(typAlias)
	if err != nil {
		return st, err
	}
	accID, err := vo.ParseId(accountID)
	if err != nil {
		return st, err
	}
	sched, ok := model.ParseRecurringSchedule(schedule)
	if !ok {
		return st, errs.NewValidation("Validation failed", errs.FieldError{Key: "schedule", Message: "The value you selected is not a valid choice.", Code: errs.CodeInvalidChoice})
	}
	nextAt, err := time.Parse(datetime.Layout, nextPaymentAt)
	if err != nil {
		return st, errs.NewValidation("Validation failed", errs.FieldError{Key: "nextPaymentAt", Message: "This value is not a valid datetime.", Code: errs.CodeInvalidDatetime})
	}
	recip, err := parseOptID(accountRecipID)
	if err != nil {
		return st, err
	}
	if typ.IsTransfer() && recip == nil {
		return st, errs.NewValidation("Validation failed", errs.FieldError{Key: "accountRecipientId", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	cat, err := parseOptID(categoryID)
	if err != nil {
		return st, err
	}
	payee, err := parseOptID(payeeID)
	if err != nil {
		return st, err
	}
	tag, err := parseOptID(tagID)
	if err != nil {
		return st, err
	}

	st.Type = typ
	st.AccountID = accID
	st.AccountRecipID = recip
	st.Amount = vo.NewDecimal(amount).String()
	st.CategoryID = cat
	st.PayeeID = payee
	st.TagID = tag
	if description != nil {
		st.Description = *description
	}
	st.Schedule = sched
	st.NextPaymentAt = nextAt
	return st, nil
}

func parseOptID(s *string) (*vo.Id, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	id, err := vo.ParseId(*s)
	if err != nil {
		return nil, err
	}
	return &id, nil
}
