// Import side: import-transaction-list. Parses an uploaded CSV per a field
// mapping (+ optional overrides), find-or-creating accounts/categories/payees/
// tags, and creating one transaction per valid row inside a single transaction.
// Row-level failures are caught, counted as skipped, and recorded in the errors
// map (message -> [rowNumbers]); they do not abort the import. Ports
// TransactionListService::importTransactionList + ImportTransactionService::
// importFromCsv.
package transaction

import (
	"context"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/domain/shared/vo"
	domtransaction "github.com/econumo/econumo/internal/domain/transaction"
)

// ImportMapping maps logical fields to CSV column names. Empty string = unmapped.
// amountInflow/amountOutflow enable dual-amount mode (both required together).
type ImportMapping struct {
	Account       string
	Date          string
	Amount        string
	AmountInflow  string
	AmountOutflow string
	Description   string
	Category      string
	Payee         string
	Tag           string
}

// ImportRequest is the decoded import request: the CSV bytes, the mapping, and
// the optional per-import overrides (nil pointer = not provided; PHP treats a
// blank string the same as absent for ids).
type ImportRequest struct {
	File        []byte
	Mapping     ImportMapping
	AccountId   *string
	Date        *string
	CategoryId  *string
	Description *string
	PayeeId     *string
	TagId       *string
}

// ImportResult is the wire result: counts + an errors map (message ->
// [rowNumbers]).
type ImportResult struct {
	Imported int              `json:"imported"`
	Skipped  int              `json:"skipped"`
	Errors   map[string][]int `json:"errors"`
}

// ImportAccount / ImportCategory / ImportPayee / ImportTag are the lightweight
// entity views the importer works with (id + name + owner for the belongs-to
// checks; category carries no type since it's only matched by name).
type ImportAccount struct {
	ID      string
	Name    string
	OwnerID string
}
type ImportNamed struct {
	ID      string
	Name    string
	OwnerID string
}

// Importer is the read/write port the import orchestration drives. It abstracts
// the account/metadata repos + create services so app/transaction stays
// decoupled from those packages. All methods run within the service's
// import-wide transaction.
type Importer interface {
	// AvailableAccounts returns the user's available (own, not deleted) accounts.
	AvailableAccounts(ctx context.Context, userID vo.Id) ([]ImportAccount, error)
	// AccountByID returns an available account by id (nil if not found).
	AccountByID(ctx context.Context, userID vo.Id, id vo.Id) (*ImportAccount, error)
	// CanAddTransaction reports whether the user may add a transaction to the
	// account (ownership, in the single-user reduction).
	CanAddTransaction(ctx context.Context, userID vo.Id, accountID vo.Id) (bool, error)
	// CreateAccount creates a new account (base currency, first/new folder, icon
	// 'wallet', balance 0) and returns its view.
	CreateAccount(ctx context.Context, userID vo.Id, name string) (ImportAccount, error)

	// CategoriesByOwner / PayeesByOwner / TagsByOwner return the owner's entities.
	CategoriesByOwner(ctx context.Context, ownerID vo.Id) ([]ImportNamed, error)
	PayeesByOwner(ctx context.Context, ownerID vo.Id) ([]ImportNamed, error)
	TagsByOwner(ctx context.Context, ownerID vo.Id) ([]ImportNamed, error)
	// CreateCategory creates a category (income type when income==true, else
	// expense; icon 'category'). CreatePayee/CreateTag create by name.
	CreateCategory(ctx context.Context, ownerID vo.Id, name string, income bool) (ImportNamed, error)
	CreatePayee(ctx context.Context, ownerID vo.Id, name string) (ImportNamed, error)
	CreateTag(ctx context.Context, ownerID vo.Id, name string) (ImportNamed, error)

	// SaveTransaction persists a built transaction (no idempotency id).
	SaveTransaction(ctx context.Context, t *domtransaction.Transaction) error
}

// ImportTransactionList runs the CSV import for the user. It returns the result
// with counts + errors; only an infrastructure error (tx failure, override
// resolution failure) returns a non-nil error.
func (s *Service) ImportTransactionList(ctx context.Context, userID vo.Id, req ImportRequest) (*ImportResult, error) {
	result := &ImportResult{Errors: map[string][]int{}}

	if len(req.File) == 0 {
		addImportError(result, "No file provided", 0)
		return result, nil
	}

	overrideAccountID := trimPtr(req.AccountId)
	overrideDateStr := trimPtr(req.Date)

	// Mapping must include account + date (unless overridden).
	if req.Mapping.Account == "" && overrideAccountID == "" {
		addImportError(result, `Mapping must include "account" and "date" fields`, 0)
		return result, nil
	}
	if req.Mapping.Date == "" && overrideDateStr == "" {
		addImportError(result, `Mapping must include "account" and "date" fields`, 0)
		return result, nil
	}

	dualMode := req.Mapping.AmountInflow != "" || req.Mapping.AmountOutflow != ""
	if dualMode && (req.Mapping.AmountInflow == "" || req.Mapping.AmountOutflow == "") {
		addImportError(result, `Mapping must include both "amountInflow" and "amountOutflow" fields when using dual amount mode`, 0)
		return result, nil
	}
	if !dualMode && req.Mapping.Amount == "" {
		addImportError(result, `Mapping must include either "amount" or both "amountInflow" and "amountOutflow"`, 0)
		return result, nil
	}

	header, records, perr := parseCSVRecords(req.File)
	if perr != nil {
		addImportError(result, "Failed to open CSV file", 0)
		return result, nil
	}
	if len(header) == 0 {
		addImportError(result, "CSV file is empty or invalid", 0)
		return result, nil
	}

	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		return s.runImport(ctx, userID, req, overrideAccountID, overrideDateStr, dualMode, header, records, result)
	}); err != nil {
		return nil, err
	}
	return result, nil
}

// runImport performs the in-transaction work: resolve overrides, build the
// find-or-create caches, then process each row. Returns a non-nil error only for
// override-resolution failures that abort the whole import (PHP returns early
// with a single top-level error); those are recorded in the result and a nil
// error is returned to keep the 200 envelope, except true infra errors.
func (s *Service) runImport(ctx context.Context, userID vo.Id, req ImportRequest, overrideAccountID, overrideDateStr string, dualMode bool, header []string, records []map[string]string, result *ImportResult) error {
	imp := s.importer

	accounts, err := imp.AvailableAccounts(ctx, userID)
	if err != nil {
		return err
	}
	accountByName := newNameCache()
	for _, a := range accounts {
		accountByName.put(a.Name, a)
	}

	// Override account (must exist + be accessible).
	var overrideAccount *ImportAccount
	accountOwnerID := userID
	if overrideAccountID != "" {
		oid, perr := vo.ParseId(overrideAccountID)
		if perr != nil {
			addImportError(result, "Account not found for provided accountId", 0)
			return nil
		}
		a, aerr := imp.AccountByID(ctx, userID, oid)
		if aerr != nil {
			return aerr
		}
		if a == nil {
			addImportError(result, "Account not found for provided accountId", 0)
			return nil
		}
		overrideAccount = a
		if oo, oerr := vo.ParseId(a.OwnerID); oerr == nil {
			accountOwnerID = oo
		}
	}

	categories, err := imp.CategoriesByOwner(ctx, accountOwnerID)
	if err != nil {
		return err
	}
	payees, err := imp.PayeesByOwner(ctx, accountOwnerID)
	if err != nil {
		return err
	}
	tags, err := imp.TagsByOwner(ctx, accountOwnerID)
	if err != nil {
		return err
	}
	categoryByName := newNamedCache(categories)
	payeeByName := newNamedCache(payees)
	tagByName := newNamedCache(tags)

	// Override date.
	var overrideDate *time.Time
	if overrideDateStr != "" {
		d, ok := parseImportDate(overrideDateStr)
		if !ok {
			addImportError(result, "Invalid date format '"+overrideDateStr+"'", 0)
			return nil
		}
		overrideDate = &d
	}

	// Override category/payee/tag (resolved by id among owner's entities).
	overrideCategory, ok := resolveOverrideNamed(req.CategoryId, categories)
	if !ok {
		addImportError(result, "Category not found for provided categoryId", 0)
		return nil
	}
	overridePayee, ok := resolveOverrideNamed(req.PayeeId, payees)
	if !ok {
		addImportError(result, "Payee not found for provided payeeId", 0)
		return nil
	}
	overrideTag, ok := resolveOverrideNamed(req.TagId, tags)
	if !ok {
		addImportError(result, "Tag not found for provided tagId", 0)
		return nil
	}
	var overrideDescription *string
	if req.Description != nil {
		d := strings.TrimSpace(*req.Description)
		overrideDescription = &d
	}

	// belongs-to checks when an override account is set.
	if overrideAccount != nil {
		if overrideCategory != nil && overrideCategory.OwnerID != accountOwnerID.String() {
			addImportError(result, "Category does not belong to the account owner", 0)
			return nil
		}
		if overridePayee != nil && overridePayee.OwnerID != accountOwnerID.String() {
			addImportError(result, "Payee does not belong to the account owner", 0)
			return nil
		}
		if overrideTag != nil && overrideTag.OwnerID != accountOwnerID.String() {
			addImportError(result, "Tag does not belong to the account owner", 0)
			return nil
		}
	}

	for i, row := range records {
		rowNumber := i + 2
		if rerr := s.importRow(ctx, userID, accountOwnerID, req, dualMode, row, rowNumber,
			overrideAccount, overrideDate, overrideCategory, overridePayee, overrideTag, overrideDescription,
			accountByName, categoryByName, payeeByName, tagByName, result); rerr != nil {
			// Row-level error: record + skip, continue.
			addImportError(result, rerr.Error(), rowNumber)
			result.Skipped++
		}
	}
	return nil
}

// importRow processes a single CSV row, creating a transaction on success. A
// returned error is a row-level failure (recorded + skipped by the caller); a
// nil error with no transaction created means the row was already skipped
// internally (missing required field) — to mirror PHP's continue, those paths
// record the error + increment skipped here and return nil.
func (s *Service) importRow(
	ctx context.Context, userID, accountOwnerID vo.Id, req ImportRequest, dualMode bool,
	row map[string]string, rowNumber int,
	overrideAccount *ImportAccount, overrideDate *time.Time,
	overrideCategory, overridePayee, overrideTag *ImportNamed, overrideDescription *string,
	accountByName *nameCache, categoryByName, payeeByName, tagByName *nameCache, result *ImportResult,
) error {
	imp := s.importer

	// account
	var account ImportAccount
	if overrideAccount != nil {
		account = *overrideAccount
	} else {
		name := fieldValue(row, req.Mapping.Account)
		if name == "" {
			addImportError(result, "Missing required fields (account or date)", rowNumber)
			result.Skipped++
			return nil
		}
		a, err := s.findOrCreateAccount(ctx, userID, name, accountByName)
		if err != nil {
			return err
		}
		account = a
	}

	// date
	var date time.Time
	if overrideDate != nil {
		date = *overrideDate
	} else {
		dateStr := fieldValue(row, req.Mapping.Date)
		if dateStr == "" {
			addImportError(result, "Missing required fields (account or date)", rowNumber)
			result.Skipped++
			return nil
		}
		d, ok := parseImportDate(dateStr)
		if !ok {
			addImportError(result, "Invalid date format '"+dateStr+"'", rowNumber)
			result.Skipped++
			return nil
		}
		date = d
	}

	// amount (signed; sign decides income vs expense, stored abs).
	amount, ok, aerr := parseRowAmount(req.Mapping, dualMode, row)
	if aerr != "" {
		addImportError(result, aerr, rowNumber)
		result.Skipped++
		return nil
	}
	if !ok {
		addImportError(result, "Invalid amount format", rowNumber)
		result.Skipped++
		return nil
	}

	// description
	description := ""
	if overrideDescription != nil {
		description = *overrideDescription
	} else {
		description = fieldValue(row, req.Mapping.Description)
	}

	income := !amount.IsNegative()

	// category / payee / tag (override or find-or-create by mapped name).
	var categoryID, payeeID, tagID *vo.Id
	if overrideCategory != nil {
		id, _ := vo.ParseId(overrideCategory.ID)
		categoryID = &id
	} else if name := fieldValue(row, req.Mapping.Category); name != "" {
		c, err := s.findOrCreateNamed(ctx, name, categoryByName, func(ctx context.Context) (ImportNamed, error) {
			return imp.CreateCategory(ctx, accountOwnerID, name, income)
		})
		if err != nil {
			return err
		}
		id, _ := vo.ParseId(c.ID)
		categoryID = &id
	}
	if overridePayee != nil {
		id, _ := vo.ParseId(overridePayee.ID)
		payeeID = &id
	} else if name := fieldValue(row, req.Mapping.Payee); name != "" {
		p, err := s.findOrCreateNamed(ctx, name, payeeByName, func(ctx context.Context) (ImportNamed, error) {
			return imp.CreatePayee(ctx, accountOwnerID, name)
		})
		if err != nil {
			return err
		}
		id, _ := vo.ParseId(p.ID)
		payeeID = &id
	}
	if overrideTag != nil {
		id, _ := vo.ParseId(overrideTag.ID)
		tagID = &id
	} else if name := fieldValue(row, req.Mapping.Tag); name != "" {
		tg, err := s.findOrCreateNamed(ctx, name, tagByName, func(ctx context.Context) (ImportNamed, error) {
			return imp.CreateTag(ctx, accountOwnerID, name)
		})
		if err != nil {
			return err
		}
		id, _ := vo.ParseId(tg.ID)
		tagID = &id
	}

	accID, _ := vo.ParseId(account.ID)
	typ := domtransaction.TypeExpense
	if income {
		typ = domtransaction.TypeIncome
	}
	now := s.clock.Now()
	t := domtransaction.New(domtransaction.NewState{
		ID: s.repo.NextIdentity(), UserID: userID, Type: typ, AccountID: accID,
		Amount: amount.Abs().String(), CategoryID: categoryID, PayeeID: payeeID, TagID: tagID,
		Description: description, SpentAt: date, CreatedAt: now, UpdatedAt: now,
	})
	if err := imp.SaveTransaction(ctx, t); err != nil {
		return err
	}
	result.Imported++
	return nil
}

// findOrCreateAccount returns a cached/existing account by case-insensitive name
// (only if the user can add a transaction to it) or creates one.
func (s *Service) findOrCreateAccount(ctx context.Context, userID vo.Id, name string, cache *nameCache) (ImportAccount, error) {
	if a, ok := cache.get(name); ok {
		acct := a.(ImportAccount)
		id, _ := vo.ParseId(acct.ID)
		can, err := s.importer.CanAddTransaction(ctx, userID, id)
		if err != nil {
			return ImportAccount{}, err
		}
		if can {
			return acct, nil
		}
	}
	created, err := s.importer.CreateAccount(ctx, userID, name)
	if err != nil {
		return ImportAccount{}, err
	}
	cache.put(created.Name, created)
	return created, nil
}

// findOrCreateNamed returns a cached/existing named entity by case-insensitive
// name, or creates one via create.
func (s *Service) findOrCreateNamed(ctx context.Context, name string, cache *nameCache, create func(ctx context.Context) (ImportNamed, error)) (ImportNamed, error) {
	if v, ok := cache.get(name); ok {
		return v.(ImportNamed), nil
	}
	created, err := create(ctx)
	if err != nil {
		return ImportNamed{}, err
	}
	cache.put(created.Name, created)
	return created, nil
}

// --- helpers ---

// trimPtr returns the trimmed pointee, or "" when nil/blank (PHP treats a blank
// override the same as absent).
func trimPtr(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

// fieldValue returns the trimmed value of the mapped column ("" when unmapped,
// absent, or blank).
func fieldValue(row map[string]string, column string) string {
	if column == "" {
		return ""
	}
	v, ok := row[column]
	if !ok {
		return ""
	}
	return strings.TrimSpace(v)
}

// resolveOverrideNamed resolves an optional override id among the owner's
// entities. Returns (nil, true) when the id is absent/blank; (entity, true) when
// found; (nil, false) when an id was given but not found.
func resolveOverrideNamed(idPtr *string, list []ImportNamed) (*ImportNamed, bool) {
	id := trimPtr(idPtr)
	if id == "" {
		return nil, true
	}
	for i := range list {
		if list[i].ID == id {
			return &list[i], true
		}
	}
	return nil, false
}

// addImportError appends a row number to the errors map under message (creating
// the bucket if needed). rowNumber 0 means a top-level error with no row.
func addImportError(result *ImportResult, message string, rowNumber int) {
	if _, ok := result.Errors[message]; !ok {
		result.Errors[message] = []int{}
	}
	if rowNumber != 0 {
		result.Errors[message] = append(result.Errors[message], rowNumber)
	}
}

// parseRowAmount extracts the signed amount for a row. Returns (amount, ok,
// errMsg): errMsg non-empty is a specific dual-mode error; ok=false with empty
// errMsg is the generic "invalid amount" path.
func parseRowAmount(m ImportMapping, dualMode bool, row map[string]string) (vo.DecimalNumber, bool, string) {
	if dualMode {
		inflowStr := fieldValue(row, m.AmountInflow)
		outflowStr := fieldValue(row, m.AmountOutflow)
		var inflow, outflow *vo.DecimalNumber
		if inflowStr != "" {
			if v, ok := parseImportAmount(inflowStr); ok {
				inflow = &v
			}
		}
		if outflowStr != "" {
			if v, ok := parseImportAmount(outflowStr); ok {
				outflow = &v
			}
		}
		if inflow != nil && outflow != nil {
			return vo.DecimalNumber{}, false, "Both inflow and outflow specified"
		}
		if inflow == nil && outflow == nil {
			return vo.DecimalNumber{}, false, "No amount specified"
		}
		if inflow != nil {
			return *inflow, true, ""
		}
		// outflow -> negative
		return negateDecimal(*outflow), true, ""
	}

	amountStr := fieldValue(row, m.Amount)
	if amountStr == "" {
		return vo.DecimalNumber{}, false, "Missing amount"
	}
	v, ok := parseImportAmount(amountStr)
	if !ok {
		return vo.DecimalNumber{}, false, ""
	}
	return v, true, ""
}
