package transaction

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Clock supplies the current time.
type Clock interface {
	Now() time.Time
}

// TxRunner is the transaction boundary the service owns.
type TxRunner interface {
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// OperationGuard is the shared idempotency guard (create-transaction has an
// OperationId).
type OperationGuard interface {
	Claim(ctx context.Context, id vo.Id, now time.Time) (already bool, err error)
	MarkHandled(ctx context.Context, id vo.Id, now time.Time) error
}

// AuthorView is the minimal author shape the result embeds.
type AuthorView struct {
	ID     string
	Name   string
	Avatar string
}

// UserLookup resolves the author (id, name, avatar).
type UserLookup interface {
	GetOwner(ctx context.Context, userID string) (AuthorView, error)
}

// AccountResolver answers ownership/existence questions about an account and
// supplies the account-list embed. The account module's service satisfies the
// list method; ownership is answered by a small repo lookup (AccountOwner).
type AccountResolver interface {
	// AccountOwner returns the owner user id of an account (for the access
	// check). Missing -> *errs.NotFoundError.
	AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error)
	// AccountListForUser returns the user's available accounts in the wire shape
	// (reverse order), for the create/update/delete result embed.
	AccountListForUser(ctx context.Context, userID vo.Id) ([]AccountResult, error)
}

// VisibleAccounts supplies the set of account ids whose transactions a user may
// list (own + shared, minus deleted + hidden-folder). The account module
// provides this.
type VisibleAccounts interface {
	VisibleAccountIDs(ctx context.Context, userID vo.Id) ([]vo.Id, error)
}

// AccountGrants reports whether a connected (non-owner) user holds an
// admin-or-user shared-access grant on an account, for the write-access
// check. Backed by the connection module's AccountAccess repository.
type AccountGrants interface {
	HasWriteGrant(ctx context.Context, accountID, userID vo.Id) (bool, error)
}

// Service is the transaction write-side orchestrator.
type Service struct {
	repo     Repository
	accounts AccountResolver
	grants   AccountGrants
	visible  VisibleAccounts
	users    UserLookup
	export   ExportLookup
	importer Importer
	tx       TxRunner
	ops      OperationGuard
	clock    Clock
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
	tx TxRunner,
	ops OperationGuard,
	clock Clock,
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
func (s *Service) toResult(ctx context.Context, t *Transaction) (TransactionResult, error) {
	author, err := s.users.GetOwner(ctx, t.UserId().String())
	if err != nil {
		return TransactionResult{}, err
	}
	return s.buildResult(t, AuthorResult{Id: author.ID, Avatar: author.Avatar, Name: author.Name}), nil
}

// buildResult assembles the wire DTO from an already-resolved author (no DB
// access), so callers control author resolution / caching.
func (s *Service) buildResult(t *Transaction, author AuthorResult) TransactionResult {
	amountRecipient := t.Amount()
	if ar := t.AmountRecipient(); ar != nil {
		amountRecipient = *ar
	}
	var recipID, catID, payeeID, tagID *string
	if v := t.AccountRecipientId(); v != nil {
		s := v.String()
		recipID = &s
	}
	if v := t.CategoryId(); v != nil {
		s := v.String()
		catID = &s
	}
	if v := t.PayeeId(); v != nil {
		s := v.String()
		payeeID = &s
	}
	if v := t.TagId(); v != nil {
		s := v.String()
		tagID = &s
	}
	return TransactionResult{
		Id:                 t.Id().String(),
		Author:             author,
		Type:               t.Type().Alias(),
		AccountId:          t.AccountId().String(),
		AccountRecipientId: recipID,
		Amount:             vo.NewDecimal(t.Amount()).String(),
		AmountRecipient:    strPtrDecimal(amountRecipient),
		CategoryId:         catID,
		Description:        t.Description(),
		PayeeId:            payeeID,
		TagId:              tagID,
		Date:               t.SpentAt().Format(datetime.Layout),
	}
}

// strPtrDecimal normalizes a decimal string and returns a pointer to it.
func strPtrDecimal(v string) *string {
	n := vo.NewDecimal(v).String()
	return &n
}

// accountListEmbed builds the account-list embed for the create/update/delete
// results (the full reversed list with balances).
func (s *Service) accountListEmbed(ctx context.Context, userID vo.Id) ([]AccountResult, error) {
	return s.accounts.AccountListForUser(ctx, userID)
}

// buildState converts the request's primitive fields into a domain NewState,
// applying the type-dependent field rules: a transfer keeps recipient
// account+amount and drops category/payee/tag; a non-transfer requires a
// category and keeps payee/tag, dropping recipient. amount is normalized.
func buildState(
	id, userID vo.Id, typ Type, accountID vo.Id, amount string,
	amountRecipient, accountRecipientID, categoryID, payeeID, tagID *string,
	description string, spentAt, now time.Time,
) (NewState, error) {
	st := NewState{
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

// parseType maps the wire alias to the domain Type.
func parseType(alias string) (Type, error) {
	switch alias {
	case "expense":
		return TypeExpense, nil
	case "income":
		return TypeIncome, nil
	case "transfer":
		return TypeTransfer, nil
	default:
		return 0, errs.NewValidation("Validation failed",
			errs.FieldError{Key: "type", Message: "The value you selected is not a valid choice.", Code: "INVALID_CHOICE_ERROR"})
	}
}
