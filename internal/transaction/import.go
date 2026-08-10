// Import side: import-transaction-list. Parses an uploaded CSV per a field
// mapping (+ optional overrides), find-or-creating accounts/categories/payees/
// tags, and creating one transaction per valid row inside a single transaction.
// Row-level failures are caught, counted as skipped, and recorded in the errors
// map (message -> [rowNumbers]); they do not abort the import.
package transaction

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// maxLabelsPerImportRow caps how many distinct labels one mapped cell may
// resolve to. A mapped column that isn't really a label list (e.g.
// description) would otherwise mass-create junk labels — one cell can split
// into many values, a bigger blast radius than the single-valued
// category/payee/tag columns. Exceeding it is a row-level error rather than a
// silent truncation, so the mis-mapping is visible instead of hidden.
const maxLabelsPerImportRow = 10

// ImportTransactionList runs the CSV import for the user. It returns the result
// with counts + errors; only an infrastructure error (tx failure, override
// resolution failure) returns a non-nil error.
func (s *Service) ImportTransactionList(ctx context.Context, userID vo.Id, req model.ImportRequest) (*model.ImportResult, error) {
	result := &model.ImportResult{Errors: map[string][]int{}}

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
// find-or-create caches, then process each row. Override-resolution failures
// abort the whole import with a single top-level error recorded in the result,
// returning nil to keep the 200 envelope; only true infra errors return non-nil.
func (s *Service) runImport(ctx context.Context, userID vo.Id, req model.ImportRequest, overrideAccountID, overrideDateStr string, dualMode bool, header []string, records []map[string]string, result *model.ImportResult) error {
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
	var overrideAccount *model.ImportAccount
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
	labels, err := imp.LabelsByOwner(ctx, accountOwnerID)
	if err != nil {
		return err
	}
	categoryByName := newNamedCache(categories)
	payeeByName := newNamedCache(payees)
	tagByName := newNamedCache(tags)
	// labelCaches is keyed by owner id, seeded with the outer accountOwnerID's
	// cache (the common case: every row lands on the same owner). A row-mapped
	// account name (no accountId override) can resolve to a DIFFERENT owner's
	// shared account per row -- unlike category/payee/tag, a label is
	// owner-only (see checkReferences in usecase.go), so reusing this owner's
	// cache for that row would create/attach the label under the wrong owner,
	// leaving the account owner unable to resolve it afterwards (a caller-owned
	// label fails the owner-only belongs-to check on the next
	// update-transaction). importRow re-derives the label owner per row from
	// the resolved account and looks up (or lazily builds) that owner's cache
	// here instead. category/payee/tag intentionally keep using the single
	// outer accountOwnerID below: their belongs-to check accepts either the
	// caller or the account owner, so they already tolerate this case.
	labelCaches := map[string]*nameCache{accountOwnerID.String(): newNamedCache(labels)}

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
	overrideLabelIDs, ok, labelsTooMany := resolveOverrideLabelIDs(req.LabelIds, labels)
	if !ok {
		if labelsTooMany {
			addImportError(result, fmt.Sprintf("Too many labelIds provided, exceeding the maximum of %d", maxLabelsPerImportRow), 0)
		} else {
			addImportError(result, "Label not found for provided labelIds", 0)
		}
		return nil
	}
	labelsSeparator := req.LabelsSeparator
	if labelsSeparator == "" {
		labelsSeparator = ";"
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
			overrideAccount, overrideDate, overrideCategory, overridePayee, overrideTag, overrideLabelIDs, overrideDescription, labelsSeparator,
			accountByName, categoryByName, payeeByName, tagByName, labelCaches, result); rerr != nil {
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
// internally (missing required field) — those paths record the error + increment
// skipped here and return nil so the import continues to the next row.
func (s *Service) importRow(
	ctx context.Context, userID, accountOwnerID vo.Id, req model.ImportRequest, dualMode bool,
	row map[string]string, rowNumber int,
	overrideAccount *model.ImportAccount, overrideDate *time.Time,
	overrideCategory, overridePayee, overrideTag *model.ImportNamed, overrideLabelIDs []vo.Id, overrideDescription *string,
	labelsSeparator string,
	accountByName *nameCache, categoryByName, payeeByName, tagByName *nameCache, labelCaches map[string]*nameCache, result *model.ImportResult,
) error {
	imp := s.importer

	// account
	var account model.ImportAccount
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
		c, err := s.findOrCreateNamed(ctx, name, categoryByName, func(ctx context.Context) (model.ImportNamed, error) {
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
		p, err := s.findOrCreateNamed(ctx, name, payeeByName, func(ctx context.Context) (model.ImportNamed, error) {
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
		tg, err := s.findOrCreateNamed(ctx, name, tagByName, func(ctx context.Context) (model.ImportNamed, error) {
			return imp.CreateTag(ctx, accountOwnerID, name)
		})
		if err != nil {
			return err
		}
		id, _ := vo.ParseId(tg.ID)
		tagID = &id
	}

	// labels (override applies to every row; otherwise the mapped cell is
	// split on the caller's separator, trimmed/deduped/blanks-dropped, and
	// each piece is find-or-created — mirroring category/payee/tag above, but
	// multi-valued). A label must belong to the ACCOUNT OWNER, so it is
	// resolved against labelOwnerID -- the resolved account's actual owner,
	// re-derived per row -- rather than the outer accountOwnerID that
	// category/payee/tag still use: a row-mapped account name (no accountId
	// override) can land on a shared account owned by someone other than the
	// caller, and reusing the caller's owner there would attach a
	// caller-owned label the account owner can never resolve afterwards.
	labelOwnerID := accountOwnerID
	if oo, operr := vo.ParseId(account.OwnerID); operr == nil {
		labelOwnerID = oo
	}
	var labelIDs []vo.Id
	if overrideLabelIDs != nil {
		if !labelOwnerID.Equal(accountOwnerID) {
			return errs.NewValidation("labelIds override does not apply: the resolved account belongs to a different owner")
		}
		labelIDs = overrideLabelIDs
	} else if req.Mapping.Labels != "" {
		names := splitLabelCell(fieldValue(row, req.Mapping.Labels), labelsSeparator)
		// names is already deduped (case-insensitively) by splitLabelCell, so
		// the cap counts distinct labels, not raw split pieces - "Kid A;kid a"
		// is one label and must not count as two against the limit.
		if len(names) > maxLabelsPerImportRow {
			return errs.NewValidation(fmt.Sprintf("Row has %d labels, exceeding the maximum of %d", len(names), maxLabelsPerImportRow))
		}
		if len(names) > 0 {
			labelByName, cerr := s.labelCacheForOwner(ctx, labelOwnerID, labelCaches)
			if cerr != nil {
				return cerr
			}
			labelIDs = make([]vo.Id, 0, len(names))
			for _, name := range names {
				lb, err := s.findOrCreateNamed(ctx, name, labelByName, func(ctx context.Context) (model.ImportNamed, error) {
					id, cerr := imp.CreateLabel(ctx, labelOwnerID, name)
					if cerr != nil {
						return model.ImportNamed{}, cerr
					}
					return model.ImportNamed{ID: id.String(), Name: name, OwnerID: labelOwnerID.String()}, nil
				})
				if err != nil {
					return err
				}
				id, _ := vo.ParseId(lb.ID)
				labelIDs = append(labelIDs, id)
			}
		}
	}

	accID, _ := vo.ParseId(account.ID)
	typ := model.TransactionTypeExpense
	if income {
		typ = model.TransactionTypeIncome
	}
	now := s.clock.Now()
	t := model.New(model.NewState{
		ID: s.repo.NextIdentity(), UserID: userID, Type: typ, AccountID: accID,
		Amount: amount.Abs().String(), CategoryID: categoryID, PayeeID: payeeID, TagID: tagID, LabelIDs: labelIDs,
		Description: description, SpentAt: date, CreatedAt: now, UpdatedAt: now,
	})
	if err := imp.SaveTransaction(ctx, t); err != nil {
		return err
	}
	// t is a brand-new row (NextIdentity), so ReplaceLabels's DELETE can never
	// match; skip the round trip entirely when there is nothing to attach
	// (the common case on an import with no labels mapping).
	if len(t.LabelIDs) > 0 {
		if err := s.repo.ReplaceLabels(ctx, t.ID, t.LabelIDs); err != nil {
			return err
		}
	}
	result.Imported++
	return nil
}

// findOrCreateAccount returns a cached/existing account by case-insensitive name
// (only if the user can add a transaction to it) or creates one.
func (s *Service) findOrCreateAccount(ctx context.Context, userID vo.Id, name string, cache *nameCache) (model.ImportAccount, error) {
	if a, ok := cache.get(name); ok {
		acct := a.(model.ImportAccount)
		id, _ := vo.ParseId(acct.ID)
		can, err := s.importer.CanAddTransaction(ctx, userID, id)
		if err != nil {
			return model.ImportAccount{}, err
		}
		if can {
			return acct, nil
		}
	}
	created, err := s.importer.CreateAccount(ctx, userID, name)
	if err != nil {
		return model.ImportAccount{}, err
	}
	cache.put(created.Name, created)
	return created, nil
}

// findOrCreateNamed returns a cached/existing named entity by case-insensitive
// name, or creates one via create.
func (s *Service) findOrCreateNamed(ctx context.Context, name string, cache *nameCache, create func(ctx context.Context) (model.ImportNamed, error)) (model.ImportNamed, error) {
	if v, ok := cache.get(name); ok {
		return v.(model.ImportNamed), nil
	}
	created, err := create(ctx)
	if err != nil {
		return model.ImportNamed{}, err
	}
	cache.put(created.Name, created)
	return created, nil
}

// labelCacheForOwner returns the cached name->label lookup for ownerID,
// fetching and caching it on first use. Owners besides the outer
// accountOwnerID only show up when a row-mapped account name resolves to a
// shared account (see the label section of importRow), so this is a cheap
// no-op map lookup for every import that never touches a second owner.
func (s *Service) labelCacheForOwner(ctx context.Context, ownerID vo.Id, caches map[string]*nameCache) (*nameCache, error) {
	key := ownerID.String()
	if c, ok := caches[key]; ok {
		return c, nil
	}
	labels, err := s.importer.LabelsByOwner(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	c := newNamedCache(labels)
	caches[key] = c
	return c, nil
}

// trimPtr returns the trimmed pointee, or "" when nil/blank (a blank override is
// treated the same as absent).
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
func resolveOverrideNamed(idPtr *string, list []model.ImportNamed) (*model.ImportNamed, bool) {
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

// resolveOverrideLabelIDs parses the comma-joined labelIds override against
// the pre-fetched owner-scoped label list: membership in that list IS the
// belongs-to-the-account-owner check, the same way a single tagId override is
// authorized against the owner-scoped tags list. Duplicate ids collapse to
// one (first occurrence wins). The deduped count is capped at
// maxLabelsPerImportRow: this override applies to EVERY imported row (unlike
// the per-row mapped-cell cap, which only ever bounds one row), so it is a
// top-level input worth bounding at the same width rather than letting a
// bad value fan out into per-row work for the whole file - closer in spirit
// to the per-row cap than to the 50-per-transaction ceiling
// resolveLabels/write side enforces downstream. Returns (nil, true) when
// idsCSV is absent/blank (a blank override is treated as absent, like every
// other override id); (ids, true) when every piece resolves within the cap;
// (nil, false, tooMany=false) when any piece is not found; (nil, false,
// tooMany=true) when the deduped count exceeds the cap - both are top-level
// errors, matching how a bad tagId override behaves, but distinguished so
// the caller can report the right message.
func resolveOverrideLabelIDs(idsCSV *string, list []model.ImportNamed) (ids []vo.Id, ok bool, tooMany bool) {
	csv := trimPtr(idsCSV)
	if csv == "" {
		return nil, true, false
	}
	pieces := strings.Split(csv, ",")
	seen := make(map[string]struct{}, len(pieces))
	out := make([]vo.Id, 0, len(pieces))
	for _, p := range pieces {
		raw := strings.TrimSpace(p)
		if raw == "" {
			continue
		}
		found := false
		for i := range list {
			if list[i].ID == raw {
				id, err := vo.ParseId(list[i].ID)
				if err != nil {
					return nil, false, false
				}
				if _, dup := seen[id.String()]; !dup {
					seen[id.String()] = struct{}{}
					out = append(out, id)
				}
				found = true
				break
			}
		}
		if !found {
			return nil, false, false
		}
	}
	if len(out) == 0 {
		// Every piece was blank (e.g. idsCSV == ","): treat exactly like an
		// absent override, not an explicit "no labels", so a mapped labels
		// column still resolves per row instead of being silently suppressed.
		return nil, true, false
	}
	if len(out) > maxLabelsPerImportRow {
		return nil, false, true
	}
	return out, true, false
}

// addImportError appends a row number to the errors map under message (creating
// the bucket if needed). rowNumber 0 means a top-level error with no row.
func addImportError(result *model.ImportResult, message string, rowNumber int) {
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
func parseRowAmount(m model.ImportMapping, dualMode bool, row map[string]string) (vo.DecimalNumber, bool, string) {
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
