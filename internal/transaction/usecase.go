package transaction

import (
	"context"
	"sort"
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
	labels   LabelOwnership
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
	labels LabelOwnership,
	tx port.TxRunner,
	ops port.OperationGuard,
	clock port.Clock,
) *Service {
	return &Service{repo: repo, accounts: accounts, grants: grants, visible: visible, users: users, export: export, importer: importer, labels: labels, tx: tx, ops: ops, clock: clock}
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
		return &errs.ValidationError{Msg: notAvailableMsg, MsgCode: notAvailableCode(notAvailableMsg)}
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
	return &errs.ValidationError{Msg: notAvailableMsg, MsgCode: notAvailableCode(notAvailableMsg)}
}

// notAvailableCode maps checkWriteAccess's two frozen notAvailableMsg literals
// to their catalogue codes.
func notAvailableCode(msg string) string {
	if msg == "transaction.transaction.not_available" {
		return errs.CodeTransactionItemNotAvailable
	}
	return errs.CodeTransactionAccountNotAvailable
}

// checkReferences authorizes the non-source references a create/update carries
// (the source account is already checked by the caller). Without this a valid
// foreign UUID would be enough to touch another user's data:
//   - a transfer's recipient account needs the SAME write access as the source,
//     else a caller could inject a leg into a stranger's account (its balance is
//     SUM(amount_recipient) over that account id);
//   - an optional category/payee/tag must belong to the CALLER or to the OWNER
//     of the account the transaction is on. On a shared account the SPA
//     categorizes with the account owner's entities (its picker filters to the
//     account owner), so a caller-only check would reject a legitimate
//     co-sharer's transaction; a truly foreign (unconnected) id is still
//     rejected.
//   - when rawLabelIDs is non-nil, the ids it points to are resolved and
//     written into st.LabelIDs for a non-transfer (a transfer never carries
//     labels; st.LabelIDs stays whatever the caller already set — buildState
//     never sets it, so create/update start from nil). rawLabelIDs itself
//     being nil means "leave st.LabelIDs as the caller already set it": bulk
//     re-classification touches only category/payee/tag and must not zero an
//     existing label set, so it passes nil after seeding st.LabelIDs with the
//     transaction's current labels; create/update always pass a non-nil
//     pointer (even to an empty/nil slice) since their wire contract is a full
//     replace. Unlike category/payee/tag, a label must belong to the ACCOUNT
//     OWNER specifically, not the caller: reporting labels classify the
//     owner's books, so a connected co-sharer classifies with the owner's
//     labels exactly as they do with the owner's categories/tags/payees above.
func (s *Service) checkReferences(ctx context.Context, userID vo.Id, st *model.NewState, rawLabelIDs *[]string) error {
	if st.AccountRecipID != nil {
		if err := s.checkWriteAccess(ctx, userID, *st.AccountRecipID, "account.account.not_available"); err != nil {
			return err
		}
	}
	// The account owner (== userID for an own account) whose entities are also
	// acceptable references. Write access to st.AccountID is already verified by
	// the caller, so the lookup resolves.
	ownerID, err := s.accounts.AccountOwner(ctx, st.AccountID)
	if err != nil {
		return &errs.ValidationError{Msg: "account.account.not_available", MsgCode: errs.CodeTransactionAccountNotAvailable}
	}
	if st.CategoryID != nil {
		if err := s.requireAvailableEntity(ctx, userID, ownerID, *st.CategoryID, s.importer.CategoriesByOwner); err != nil {
			return err
		}
	}
	if st.PayeeID != nil {
		if err := s.requireAvailableEntity(ctx, userID, ownerID, *st.PayeeID, s.importer.PayeesByOwner); err != nil {
			return err
		}
	}
	if st.TagID != nil {
		if err := s.requireAvailableEntity(ctx, userID, ownerID, *st.TagID, s.importer.TagsByOwner); err != nil {
			return err
		}
	}
	if !st.Type.IsTransfer() && rawLabelIDs != nil {
		ids, lerr := s.resolveLabels(ctx, ownerID, *rawLabelIDs)
		if lerr != nil {
			return lerr
		}
		st.LabelIDs = ids
	}
	return nil
}

// resolveLabels parses, dedupes, and authorizes the wire labelIds for a
// non-transfer transaction: each must parse as a valid id, exist, and be
// owned by ownerID (the transaction's account owner) — else the frozen
// item-not-available validation error, same as an unavailable category/
// payee/tag. Deduped ids are sorted by string so an immediate create/update
// response shows the same order a later read does (LabelsByTransactionIDs
// orders by label_id for cross-engine stability; sorting here keeps the two
// views consistent without a redundant DB round trip on the write path).
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

// requireAvailableEntity confirms id belongs to the caller or to the account
// owner (each list is owner-scoped, so membership IS the ownership check). A
// foreign or unknown id yields the frozen item-not-available validation error.
func (s *Service) requireAvailableEntity(ctx context.Context, callerID, accountOwnerID, id vo.Id, list func(context.Context, vo.Id) ([]model.ImportNamed, error)) error {
	if ok, err := ownsEntity(ctx, callerID, id, list); err != nil {
		return err
	} else if ok {
		return nil
	}
	if !accountOwnerID.Equal(callerID) {
		if ok, err := ownsEntity(ctx, accountOwnerID, id, list); err != nil {
			return err
		} else if ok {
			return nil
		}
	}
	return &errs.ValidationError{Msg: "transaction.transaction.not_available", MsgCode: errs.CodeTransactionItemNotAvailable}
}

// ownsEntity reports whether id is among ownerID's owner-scoped entities.
func ownsEntity(ctx context.Context, ownerID, id vo.Id, list func(context.Context, vo.Id) ([]model.ImportNamed, error)) (bool, error) {
	items, err := list(ctx, ownerID)
	if err != nil {
		return false, err
	}
	for _, it := range items {
		if it.ID == id.String() {
			return true, nil
		}
	}
	return false, nil
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
// amountRecipient falls back to amount when nil. labelIDs is the wire-shaped
// label id list already resolved by the caller (create/update pass the
// in-memory entity's own LabelIDs — set moments earlier in the same tx, so it
// is authoritative without a re-read; delete passes ids fetched BEFORE the
// row and its transactions_labels rows are removed, since a post-delete
// LabelsByTransactionIDs would come back empty due to the cascade). The list
// endpoint uses buildResult directly with a per-request author cache to avoid
// an N+1 (see GetTransactionList).
func (s *Service) toResult(ctx context.Context, t *model.Transaction, labelIDs []string) (model.TransactionResult, error) {
	author, err := s.users.GetOwner(ctx, t.UserID.String())
	if err != nil {
		return model.TransactionResult{}, err
	}
	return s.buildResult(t, model.UserResult{Id: author.ID, Avatar: author.Avatar, Name: author.Name}, labelIDs), nil
}

// labelIDStrings converts a resolved label id slice to its wire string form
// (nil-safe: a transfer or a never-classified transaction has none).
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

// labelIDsFromStrings parses an already-persisted label id list back into
// vo.Id (the DB values are always well-formed, but ParseId's signature is
// honored rather than assumed). Used to seed a preserved label set into a
// model.NewState when a use case (bulk re-classification) must not disturb
// the existing labels.
func labelIDsFromStrings(ids []string) ([]vo.Id, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	out := make([]vo.Id, len(ids))
	for i, s := range ids {
		id, err := vo.ParseId(s)
		if err != nil {
			return nil, err
		}
		out[i] = id
	}
	return out, nil
}

// buildResult assembles the wire DTO from an already-resolved author (no DB
// access) and an already-resolved label id list, so callers control author
// resolution/caching and label-loading strategy (single lookup vs. batch).
func (s *Service) buildResult(t *model.Transaction, author model.UserResult, labelIDs []string) model.TransactionResult {
	amountRecipient := t.Amount
	if ar := t.AmountRecipient; ar != nil {
		amountRecipient = *ar
	}
	// Always a list, never null, so clients never need a nil check.
	labelIds := []string{}
	labelIds = append(labelIds, labelIDs...)
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
	var recurringID *string
	if v := t.RecurringID; v != nil {
		s := v.String()
		recurringID = &s
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
		RecurringId:        recurringID,
		LabelIds:           labelIds,
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
// keeps an optional category/payee/tag, dropping recipient. amount is
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
		// A non-transfer may have no category. Treat an empty string as absent:
		// the removed blank guard used to reject it before it could reach the
		// reference lookup, so passing "" through would turn into a spurious
		// "category not found".
		if categoryID != nil && *categoryID != "" {
			cid, err := vo.ParseId(*categoryID)
			if err != nil {
				return st, err
			}
			st.CategoryID = &cid
		}
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
		return &errs.ValidationError{Msg: "account.account.not_available", MsgCode: errs.CodeTransactionAccountNotAvailable}
	}
	if srcCur.Equal(dstCur) {
		amount := st.Amount
		st.AmountRecipient = &amount
		return nil
	}
	if st.AmountRecipient == nil {
		return errs.NewValidation("Validation failed",
			errs.FieldError{Key: "amountRecipient", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
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
			errs.FieldError{Key: "type", Message: "The value you selected is not a valid choice.", Code: errs.CodeInvalidChoice})
	}
}
