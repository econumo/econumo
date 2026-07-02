package transaction

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/vo"
)

var crlfRun = regexp.MustCompile(`[\r\n]+`)

var exportHeaders = []string{
	"transaction_id",
	"account_name",
	"account_currency",
	"category",
	"description",
	"tag",
	"payee",
	"amount",
	"date",
}

// ExportAccount is one accessible account in the export universe: id, name, and
// currency code.
type ExportAccount struct {
	ID           string
	Name         string
	CurrencyCode string
}

// ExportLookup supplies the read-side data the export needs without coupling the
// transaction service to the account/metadata repo packages: the user's
// accessible accounts (own + shared, not deleted) and name resolution for the
// optional category/tag/payee of each transaction. Name lookups return "" when
// the entity is missing.
type ExportLookup interface {
	ExportAccounts(ctx context.Context, userID vo.Id) ([]ExportAccount, error)
	CategoryName(ctx context.Context, id vo.Id) (string, error)
	TagName(ctx context.Context, id vo.Id) (string, error)
	PayeeName(ctx context.Context, id vo.Id) (string, error)
}

// ExportTransactionList builds the CSV rows for the given user, optionally
// restricted to a set of account ids (nil = all accessible accounts). The first
// row is the header.
func (s *Service) ExportTransactionList(ctx context.Context, userID vo.Id, accountIDs []vo.Id) ([][]string, error) {
	// allAccountsById = the user's accessible accounts (own + shared, not
	// deleted), keyed by id. accountsById = that set intersected with the
	// selected ids (or all when no selection).
	accts, err := s.export.ExportAccounts(ctx, userID)
	if err != nil {
		return nil, err
	}
	allAccountsByID := make(map[string]ExportAccount, len(accts))
	for _, a := range accts {
		allAccountsByID[a.ID] = a
	}

	selectedByID := allAccountsByID
	if accountIDs != nil {
		selectedByID = make(map[string]ExportAccount)
		for _, id := range accountIDs {
			if a, ok := allAccountsByID[id.String()]; ok {
				selectedByID[a.ID] = a
			}
		}
	}

	rows := [][]string{exportHeaders}
	if len(selectedByID) == 0 {
		return rows, nil
	}

	// Source transactions = findAvailableForUserId(userId): every transaction
	// whose source OR recipient account is in the accessible (own + shared) set.
	ids := make([]vo.Id, 0, len(allAccountsByID))
	for _, a := range accts {
		id, perr := vo.ParseId(a.ID)
		if perr != nil {
			return nil, perr
		}
		ids = append(ids, id)
	}
	txs, err := s.repo.ListByAccountIDs(ctx, ids, time.Time{}, time.Time{})
	if err != nil {
		return nil, err
	}

	// Category/tag/payee names repeat heavily across an export (a few dozen
	// distinct ids over thousands of rows); resolve each name once per request.
	names := newExportNameCache()
	for _, t := range txs {
		built, berr := s.buildExportRows(ctx, t, selectedByID, allAccountsByID, names)
		if berr != nil {
			return nil, berr
		}
		rows = append(rows, built...)
	}
	return rows, nil
}

// exportNameCache memoizes category/tag/payee name lookups for one export so a
// repeated id is not re-queried per transaction.
type exportNameCache struct {
	categories, tags, payees map[string]string
}

func newExportNameCache() *exportNameCache {
	return &exportNameCache{categories: map[string]string{}, tags: map[string]string{}, payees: map[string]string{}}
}

// cachedName returns the name for id from cache, fetching (and caching) on a miss.
func cachedName(ctx context.Context, cache map[string]string, id vo.Id, fetch func(context.Context, vo.Id) (string, error)) (string, error) {
	key := id.String()
	if v, ok := cache[key]; ok {
		return v, nil
	}
	v, err := fetch(ctx, id)
	if err != nil {
		return "", err
	}
	cache[key] = v
	return v, nil
}

// buildExportRows emits the 0, 1, or 2 CSV rows for one transaction: a row on
// the source account if it is selected, plus a second row on the recipient
// account if this is a transfer whose recipient is selected.
func (s *Service) buildExportRows(ctx context.Context, t *Transaction, selectedByID, allAccountsByID map[string]ExportAccount, names *exportNameCache) ([][]string, error) {
	var rows [][]string
	accountID := t.AccountId().String()

	if src, ok := selectedByID[accountID]; ok {
		description := t.Description()
		if t.Type().IsTransfer() {
			transferNote := "Transfer"
			if rid := t.AccountRecipientId(); rid != nil {
				if recip, ok := allAccountsByID[rid.String()]; ok {
					transferNote = "Transfer of " + formatAmountForDescription(t.Amount()) +
						" " + src.CurrencyCode + " to " + recip.Name
				}
			}
			description = applyTransferNote(transferNote, t.Description())
		}

		category, tag, payee, nerr := s.resolveNames(ctx, t, names)
		if nerr != nil {
			return nil, nerr
		}
		rows = append(rows, s.buildExportRow(
			t, src,
			formatAmount(t.Amount(), t.Type().IsExpense() || t.Type().IsTransfer()),
			category, tag, payee, description,
		))
	}

	if t.Type().IsTransfer() {
		if rid := t.AccountRecipientId(); rid != nil {
			if recip, ok := selectedByID[rid.String()]; ok {
				sourceName := ""
				if src, ok := allAccountsByID[accountID]; ok {
					sourceName = src.Name
				}
				transferNote := "Transfer"
				amt := t.Amount()
				if ar := t.AmountRecipient(); ar != nil {
					amt = *ar
				}
				if sourceName != "" {
					transferNote = "Transfer of " + formatAmountForDescription(amt) +
						" " + recip.CurrencyCode + " from " + sourceName
				}
				description := applyTransferNote(transferNote, t.Description())
				rows = append(rows, s.buildExportRow(
					t, recip, formatAmount(amt, false), "", "", "", description,
				))
			}
		}
	}

	return rows, nil
}

// buildExportRow assembles one CSV row in the fixed column order: id,
// account_name, account_currency, category, description, tag, payee, amount, date.
func (s *Service) buildExportRow(t *Transaction, account ExportAccount, amount, category, tag, payee, description string) []string {
	return []string{
		t.Id().String(),
		sanitizeExportValue(account.Name),
		account.CurrencyCode,
		sanitizeExportValue(category),
		sanitizeExportValue(description),
		sanitizeExportValue(tag),
		sanitizeExportValue(payee),
		amount,
		t.SpentAt().Format(datetime.Layout),
	}
}

// resolveNames resolves the optional category/tag/payee names for a transaction
// (empty when absent or missing).
func (s *Service) resolveNames(ctx context.Context, t *Transaction, names *exportNameCache) (category, tag, payee string, err error) {
	if id := t.CategoryId(); id != nil {
		if category, err = cachedName(ctx, names.categories, *id, s.export.CategoryName); err != nil {
			return "", "", "", err
		}
	}
	if id := t.TagId(); id != nil {
		if tag, err = cachedName(ctx, names.tags, *id, s.export.TagName); err != nil {
			return "", "", "", err
		}
	}
	if id := t.PayeeId(); id != nil {
		if payee, err = cachedName(ctx, names.payees, *id, s.export.PayeeName); err != nil {
			return "", "", "", err
		}
	}
	return category, tag, payee, nil
}

// applyTransferNote combines the auto note with any existing description (empty
// description -> note; else "note [trimmed-desc]").
func applyTransferNote(note, description string) string {
	if strings.TrimSpace(description) == "" {
		return note
	}
	return note + " [" + strings.TrimSpace(description) + "]"
}

// formatAmount returns the normalized decimal string with the sign forced
// (negative=true -> leading '-'; negative=false -> stripped).
func formatAmount(amount string, negative bool) string {
	value := vo.NewDecimal(amount).String()
	if negative {
		if strings.HasPrefix(value, "-") {
			return value
		}
		return "-" + value
	}
	return strings.TrimPrefix(value, "-")
}

// formatAmountForDescription returns the absolute normalized decimal (no sign).
func formatAmountForDescription(amount string) string {
	return strings.TrimPrefix(vo.NewDecimal(amount).String(), "-")
}

// sanitizeExportValue collapses CR/LF runs to a single space and trims.
func sanitizeExportValue(value string) string {
	if value == "" {
		return ""
	}
	value = crlfRun.ReplaceAllString(value, " ")
	return strings.TrimSpace(value)
}
