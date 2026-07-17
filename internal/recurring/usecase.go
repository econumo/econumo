package recurring

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/port"
	"github.com/econumo/econumo/internal/shared/vo"
)

type Service struct {
	repo     Repository
	accounts AccountResolver
	grants   AccountGrants
	visible  VisibleAccounts
	creator  TransactionCreator
	tx       port.TxRunner
	ops      port.OperationGuard
	clock    port.Clock
}

func NewService(repo Repository, accounts AccountResolver, grants AccountGrants, visible VisibleAccounts, creator TransactionCreator, tx port.TxRunner, ops port.OperationGuard, clock port.Clock) *Service {
	return &Service{repo: repo, accounts: accounts, grants: grants, visible: visible, creator: creator, tx: tx, ops: ops, clock: clock}
}

// Same matrix as transaction writes: owner or admin/user grant; guest denied.
func (s *Service) checkWriteAccess(ctx context.Context, userID, accountID vo.Id) error {
	owner, err := s.accounts.AccountOwner(ctx, accountID)
	if err != nil {
		return &errs.ValidationError{Msg: "account.account.not_available", MsgCode: errs.CodeTransactionAccountNotAvailable}
	}
	if owner.Equal(userID) {
		return nil
	}
	ok, err := s.grants.HasWriteGrant(ctx, accountID, userID)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	return &errs.ValidationError{Msg: "account.account.not_available", MsgCode: errs.CodeTransactionAccountNotAvailable}
}

func parseType(alias string) (model.TransactionType, error) {
	switch alias {
	case "expense":
		return model.TransactionTypeExpense, nil
	case "income":
		return model.TransactionTypeIncome, nil
	case "transfer":
		return model.TransactionTypeTransfer, nil
	}
	return 0, errs.NewValidation("Validation failed", errs.FieldError{Key: "type", Message: "The value you selected is not a valid choice.", Code: errs.CodeInvalidChoice})
}

func toResult(rt *model.RecurringTransaction) model.RecurringTransactionResult {
	return model.RecurringTransactionResult{
		Id:                 rt.ID.String(),
		OwnerUserId:        rt.UserID.String(),
		Type:               rt.Type.Alias(),
		AccountId:          rt.AccountID.String(),
		AccountRecipientId: idStr(rt.AccountRecipID),
		Amount:             vo.NewDecimal(rt.Amount).String(),
		CategoryId:         idStr(rt.CategoryID),
		PayeeId:            idStr(rt.PayeeID),
		TagId:              idStr(rt.TagID),
		Description:        rt.Description,
		Schedule:           string(rt.Schedule),
		NextPaymentAt:      rt.NextPaymentAt.Format(datetime.Layout),
		CreatedAt:          rt.CreatedAt.Format(datetime.Layout),
		UpdatedAt:          rt.UpdatedAt.Format(datetime.Layout),
	}
}

func idStr(id *vo.Id) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}
