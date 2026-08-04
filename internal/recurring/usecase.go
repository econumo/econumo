package recurring

import (
	"context"
	"sort"

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
	labels   LabelOwnership
	tx       port.TxRunner
	ops      port.OperationGuard
	clock    port.Clock
}

func NewService(repo Repository, accounts AccountResolver, grants AccountGrants, visible VisibleAccounts, creator TransactionCreator, labels LabelOwnership, tx port.TxRunner, ops port.OperationGuard, clock port.Clock) *Service {
	return &Service{repo: repo, accounts: accounts, grants: grants, visible: visible, creator: creator, labels: labels, tx: tx, ops: ops, clock: clock}
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

// toResult builds the wire DTO from an already-resolved label id list, so
// callers control label-loading strategy (list batches; create/update/skip
// pass an already in-hand set).
func toResult(rt *model.RecurringTransaction, labelIDs []string) model.RecurringTransactionResult {
	// Always a list, never null, so clients never need a nil check.
	labelIds := []string{}
	labelIds = append(labelIds, labelIDs...)
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
		LabelIds:           labelIds,
	}
}

func idStr(id *vo.Id) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}

// maxRecurringLabels mirrors transaction.maxTransactionLabels: the same cap
// and error code (errs.CodeTransactionTooManyLabels) apply here rather than
// inventing a second constant/code for the identical concern.
const maxRecurringLabels = 50

// resolveLabels parses, dedupes, and authorizes the wire labelIds for a
// non-transfer template: each must parse as a valid id, exist, and be owned
// by ownerID (the template's account owner) — else the frozen
// item-not-available validation error, same as transaction.resolveLabels.
// Deduped ids are sorted by string so an immediate create/update response
// shows the same order a later read does (LabelsByRecurringIDs orders by
// label_id for cross-engine stability).
func (s *Service) resolveLabels(ctx context.Context, ownerID vo.Id, rawLabelIDs []string) ([]vo.Id, error) {
	if len(rawLabelIDs) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(rawLabelIDs))
	ids := make([]vo.Id, 0, len(rawLabelIDs))
	for _, raw := range rawLabelIDs {
		id, err := vo.ParseId(raw)
		if err != nil {
			return nil, err
		}
		key := id.String()
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) > maxRecurringLabels {
		return nil, &errs.ValidationError{Msg: "A transaction can have at most 50 labels.", MsgCode: errs.CodeTransactionTooManyLabels}
	}
	owners, err := s.labels.LabelOwners(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		owner, ok := owners[id.String()]
		if !ok || !owner.Equal(ownerID) {
			return nil, &errs.ValidationError{Msg: "transaction.transaction.not_available", MsgCode: errs.CodeTransactionItemNotAvailable}
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	return ids, nil
}

// labelIDStrings converts a resolved label id slice to its wire string form
// (nil-safe: a transfer or a never-classified template has none).
func labelIDStrings(ids []vo.Id) []string {
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}
	return out
}

// labelIDsFor loads id's persisted label ids: GetByID/ListByAccountIDs don't
// hydrate them (see Repository.LabelsByRecurringIDs's doc), so a caller that
// needs a specific template's current labels (skip, post) fetches them
// separately, same as transaction's LabelsByTransactionIDs callers.
func (s *Service) labelIDsFor(ctx context.Context, id vo.Id) ([]vo.Id, error) {
	m, err := s.repo.LabelsByRecurringIDs(ctx, []vo.Id{id})
	if err != nil {
		return nil, err
	}
	raw := m[id.String()]
	if len(raw) == 0 {
		return nil, nil
	}
	ids := make([]vo.Id, len(raw))
	for i, s2 := range raw {
		pid, perr := vo.ParseId(s2)
		if perr != nil {
			return nil, perr
		}
		ids[i] = pid
	}
	return ids, nil
}
