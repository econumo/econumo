package transaction

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/port"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Service is the transaction write-side orchestrator.
type Service struct {
	repo     Repository
	accounts AccountResolver
	grants   AccountGrants
	visible  VisibleAccounts
	users    UserLookup
	export   ExportLookup
	importer Importer
	tx       port.TxRunner
	ops      port.OperationGuard
	clock    port.Clock
}

// NewService wires the transaction service.
func NewService(
	repo Repository,
	accounts AccountResolver,
	grants AccountGrants,
	visible VisibleAccounts,
	users UserLookup,
	export ExportLookup,
	importer Importer,
	tx port.TxRunner,
	ops port.OperationGuard,
	clock port.Clock,
) *Service {
	return &Service{repo: repo, accounts: accounts, grants: grants, visible: visible, users: users, export: export, importer: importer, tx: tx, ops: ops, clock: clock}
}

// checkWriteAccess verifies the user may add/update/delete a transaction on the
// account: they own it, or they hold an admin/user grant on it. A guest grant —
// or no grant at all — is denied. A denied or missing account yields a
// ValidationError carrying notAvailableMsg (create/update use
// "account.account.not_available"; delete uses
// "transaction.transaction.not_available").
func (s *Service) checkWriteAccess(ctx context.Context, userID, accountID vo.Id, notAvailableMsg string) error {
	owner, err := s.accounts.AccountOwner(ctx, accountID)
	if err != nil {
		return errs.NewValidation(notAvailableMsg)
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
	return errs.NewValidation(notAvailableMsg)
}

// checkViewAccess verifies the user may VIEW the account's transactions: owner
// OR any shared access, else AccessDenied (HTTP 403). The visible-account set
// already computes own + shared, so membership in it is exactly the access test.
func (s *Service) checkViewAccess(ctx context.Context, userID, accountID vo.Id) error {
	ids, err := s.visible.VisibleAccountIDs(ctx, userID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if id.Equal(accountID) {
			return nil
		}
	}
	return errs.NewAccessDenied("Access is not allowed")
}

// toResult builds the transaction result, resolving the author embed. The
// amountRecipient falls back to amount when nil. Single-transaction callers
// (create/update/delete) use this; the list endpoint uses buildResult with a
// per-request author cache to avoid an N+1 (see GetTransactionList).
func (s *Service) toResult(ctx context.Context, t *model.Transaction) (model.TransactionResult, error) {
	author, err := s.users.GetOwner(ctx, t.UserID.String())
	if err != nil {
		return model.TransactionResult{}, err
	}
	return s.buildResult(t, model.UserResult{Id: author.ID, Avatar: author.Avatar, Name: author.Name}), nil
}

// buildResult assembles the wire DTO from an already-resolved author (no DB
// access), so callers control author resolution / caching.
func (s *Service) buildResult(t *model.Transaction, author model.UserResult) model.TransactionResult {
	amountRecipient := t.Amount
	if ar := t.AmountRecipient; ar != nil {
		amountRecipient = *ar
	}
	var recipID, catID, payeeID, tagID *string
	if v := t.AccountRecipID; v != nil {
		s := v.String()
		recipID = &s
	}
	if v := t.CategoryID; v != nil {
		s := v.String()
		catID = &s
	}
	if v := t.PayeeID; v != nil {
		s := v.String()
		payeeID = &s
	}
	if v := t.TagID; v != nil {
		s := v.String()
		tagID = &s
	}
	return model.TransactionResult{
		Id:                 t.ID.String(),
		Author:             author,
		Type:               t.Type.Alias(),
		AccountId:          t.AccountID.String(),
		AccountRecipientId: recipID,
		Amount:             vo.NewDecimal(t.Amount).String(),
		AmountRecipient:    strPtrDecimal(amountRecipient),
		CategoryId:         catID,
		Description:        t.Description,
		PayeeId:            payeeID,
		TagId:              tagID,
		Date:               t.SpentAt.Format(datetime.Layout),
	}
}

// strPtrDecimal normalizes a decimal string and returns a pointer to it.
func strPtrDecimal(v string) *string {
	n := vo.NewDecimal(v).String()
	return &n
}

// accountListEmbed builds the account-list embed for the create/update/delete
// results (the full reversed list with balances).
func (s *Service) accountListEmbed(ctx context.Context, userID vo.Id) ([]model.AccountResult, error) {
	return s.accounts.AccountListForUser(ctx, userID)
}

// buildState converts the request's primitive fields into a domain
// model.NewState, applying the type-dependent field rules: a transfer keeps
// recipient account+amount and drops category/payee/tag; a non-transfer
// requires a category and keeps payee/tag, dropping recipient. amount is
// normalized.
func buildState(
	id, userID vo.Id, typ model.TransactionType, accountID vo.Id, amount string,
	amountRecipient, accountRecipientID, categoryID, payeeID, tagID *string,
	description string, spentAt, now time.Time,
) (model.NewState, error) {
	st := model.NewState{
		ID: id, UserID: userID, Type: typ, AccountID: accountID,
		Amount: vo.NewDecimal(amount).String(), Description: description,
		SpentAt: spentAt, CreatedAt: now, UpdatedAt: now,
	}
	if typ.IsTransfer() {
		if accountRecipientID != nil && *accountRecipientID != "" {
			rid, err := vo.ParseId(*accountRecipientID)
			if err != nil {
				return st, err
			}
			st.AccountRecipID = &rid
		}
		if amountRecipient != nil && *amountRecipient != "" {
			ar := vo.NewDecimal(*amountRecipient).String()
			st.AmountRecipient = &ar
		}
	} else {
		// Non-transfer requires a category.
		if categoryID == nil || *categoryID == "" {
			return st, errs.NewValidation("Validation failed",
				errs.FieldError{Key: "categoryId", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
		}
		cid, err := vo.ParseId(*categoryID)
		if err != nil {
			return st, err
		}
		st.CategoryID = &cid
		if payeeID != nil && *payeeID != "" {
			pid, err := vo.ParseId(*payeeID)
			if err != nil {
				return st, err
			}
			st.PayeeID = &pid
		}
		if tagID != nil && *tagID != "" {
			tid, err := vo.ParseId(*tagID)
			if err != nil {
				return st, err
			}
			st.TagID = &tid
		}
	}
	return st, nil
}

// normalizeTransferAmounts enforces the transfer amount-recipient invariants
// before persisting. The recipient account's balance is SUM(amount_recipient)
// and the reporting queries classify an exchange leg via
// amount != amount_recipient, so a stale or missing client value silently
// corrupts both. Same-currency transfers therefore always store
// amount_recipient = amount; cross-currency transfers must carry an explicit
// amountRecipient (the received amount is client-authoritative — the user's
// actual rate, not ours — so defaulting would fabricate it). No-op for
// non-transfers and for transfers without a recipient account.
func (s *Service) normalizeTransferAmounts(ctx context.Context, st *model.NewState) error {
	if !st.Type.IsTransfer() || st.AccountRecipID == nil {
		return nil
	}
	srcCur, err := s.accounts.AccountCurrency(ctx, st.AccountID)
	if err != nil {
		return err
	}
	dstCur, err := s.accounts.AccountCurrency(ctx, *st.AccountRecipID)
	if err != nil {
		return errs.NewValidation("account.account.not_available")
	}
	if srcCur.Equal(dstCur) {
		amount := st.Amount
		st.AmountRecipient = &amount
		return nil
	}
	if st.AmountRecipient == nil {
		return errs.NewValidation("Validation failed",
			errs.FieldError{Key: "amountRecipient", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	return nil
}

// parseType maps the wire alias to the domain TransactionType.
func parseType(alias string) (model.TransactionType, error) {
	switch alias {
	case "expense":
		return model.TransactionTypeExpense, nil
	case "income":
		return model.TransactionTypeIncome, nil
	case "transfer":
		return model.TransactionTypeTransfer, nil
	default:
		return 0, errs.NewValidation("Validation failed",
			errs.FieldError{Key: "type", Message: "The value you selected is not a valid choice.", Code: "INVALID_CHOICE_ERROR"})
	}
}
