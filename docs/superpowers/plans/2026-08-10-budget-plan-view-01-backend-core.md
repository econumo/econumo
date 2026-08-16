# Budget Plan View — 01 Backend Core Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make income categories and income envelopes first-class budget elements (two new `budgets_elements.type` values) with side-safe reconciliation, seeding, envelope homogeneity, folder-side enforcement, and a byte-identical `get-budget` — everything the `get-budget-plan` endpoint (plan 02) and the plan-sheet UI (plan 03) will build on.

**Architecture:** The element `type` column gains values 3 (income category) and 4 (income envelope). The single reconciler `syncElements` (`internal/budget/move.go`) is the primary site: it learns both new types so its delete-unseen pass never destroys them, ensures envelopes under their stored side, and updates types in place. Envelope side is stored in the element row's type, set once at creation via a new optional `side` field, and immutable. Folder sides are derived from member element types and enforced at the two folder-assigning writes (`move-element`, `create-envelope`). `get-budget` output stays byte-identical for all existing data; income-sided envelopes and folders are additionally filtered out of it.

**Tech Stack:** Go stdlib + existing repo patterns (no new dependencies). Tests: `go test ./...` harness in `internal/budget/api` (`newHarness`), migration tests in `internal/infra/storage/migrations`, apiparity/mcpparity golden suites.

**Spec:** `docs/superpowers/specs/2026-08-02-budget-plan-view-design.md` (amended 2026-08-10). This plan covers the spec's "Income elements", "Envelopes with income categories", "Folders with content-derived sides", and "Frozen contract" sections. Plan 02 covers "New read: get-budget-plan" + "Queries" + MCP read; plan 03 covers "Frontend".

## Global Constraints

- **Frozen wire contract**: every existing route's apiparity golden must come out byte-identical, with exactly two inspected, intentional exceptions listed in Task 9 (a scenario body edit that provably produces the same bytes, and new scenarios). Never hand-edit a golden.
- **Element type values are frozen once shipped**: income category = `3`, income envelope = `4`; internal aliases `income_category` / `income_envelope`. The wire `type` field stays an int.
- The three original aliases (`envelope`/`category`/`tag`) remain the only valid **wire inputs** to `ElementTypeFromAlias` — drill-down validation output must not change.
- New error codes must be registered in `errs.AllCodes` AND translated in **both** `locales/en.json` and `locales/ru.json` (`internal/test/i18ntest` enforces the pairing; `make go-test` runs it).
- All ids are UUIDs as TEXT; datetimes `"2006-01-02 15:04:05"`; `isArchived` is int 0/1.
- Comments: only non-obvious business logic / frozen-contract rationale (see CLAUDE.md "Comments — write sparingly"). Swag `// @…` blocks are exempt and required on handlers.
- Run tests from the repo root. Smoke tier: `go test ./internal/...`; full gate before finishing: `make go-test` (build + vet + gofmt + OpenAPI-fresh + coverage ≥ 80).
- Branch: work happens on `feature/budget-plan-view`.

---

### Task 1: Income element types in the model

**Files:**
- Modify: `internal/model/budget_valueobject.go` (ElementType block, lines 10–58)
- Modify: `internal/model/budget.go` (add `UpdateType` next to `UpdateCurrency`, ~line 184)
- Test: `internal/model/budget_side_test.go` (create)

**Interfaces:**
- Consumes: nothing new.
- Produces (later tasks rely on these exact names):
  - `model.ElementIncomeCategory ElementType = 3`, `model.ElementIncomeEnvelope ElementType = 4`
  - `func (t ElementType) IsIncomeSide() bool`
  - `func EnvelopeTypeFromSide(side string) (ElementType, error)` — `""`/`"expense"` → `ElementEnvelope`, `"income"` → `ElementIncomeEnvelope`, anything else → validation error on field `side`
  - `func (e *BudgetElement) UpdateType(typ ElementType, now time.Time)`

- [ ] **Step 1: Write the failing tests**

Create `internal/model/budget_side_test.go`:

```go
package model

import "testing"

func TestElementTypeIncomeValues(t *testing.T) {
	if ElementIncomeCategory != 3 || ElementIncomeEnvelope != 4 {
		t.Fatalf("income element type values are frozen: category=3 envelope=4, got %d/%d",
			ElementIncomeCategory, ElementIncomeEnvelope)
	}
	if ElementIncomeCategory.Alias() != "income_category" || ElementIncomeEnvelope.Alias() != "income_envelope" {
		t.Fatalf("aliases: got %q / %q", ElementIncomeCategory.Alias(), ElementIncomeEnvelope.Alias())
	}
}

func TestElementTypeFromAlias_RejectsIncomeAliases(t *testing.T) {
	// The income aliases are internal-only; the wire parser keeps rejecting them
	// so existing drill-down validation output is unchanged.
	for _, alias := range []string{"income_category", "income_envelope"} {
		if _, err := ElementTypeFromAlias(alias); err == nil {
			t.Fatalf("alias %q must stay invalid on the wire", alias)
		}
	}
	for _, alias := range []string{"envelope", "category", "tag"} {
		if _, err := ElementTypeFromAlias(alias); err != nil {
			t.Fatalf("alias %q must stay valid: %v", alias, err)
		}
	}
}

func TestIsIncomeSide(t *testing.T) {
	want := map[ElementType]bool{
		ElementEnvelope: false, ElementCategory: false, ElementTag: false,
		ElementIncomeCategory: true, ElementIncomeEnvelope: true,
	}
	for typ, w := range want {
		if typ.IsIncomeSide() != w {
			t.Errorf("IsIncomeSide(%d)=%v want %v", typ, typ.IsIncomeSide(), w)
		}
	}
}

func TestEnvelopeTypeFromSide(t *testing.T) {
	for side, wantTyp := range map[string]ElementType{
		"": ElementEnvelope, "expense": ElementEnvelope, "income": ElementIncomeEnvelope,
	} {
		got, err := EnvelopeTypeFromSide(side)
		if err != nil || got != wantTyp {
			t.Errorf("EnvelopeTypeFromSide(%q) = %v, %v; want %v, nil", side, got, err, wantTyp)
		}
	}
	if _, err := EnvelopeTypeFromSide("both"); err == nil {
		t.Fatalf("invalid side must fail validation")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/model/ -run 'TestElementType|TestIsIncomeSide|TestEnvelopeTypeFromSide' -v`
Expected: FAIL — `undefined: ElementIncomeCategory`, `undefined: EnvelopeTypeFromSide`.

- [ ] **Step 3: Implement in `internal/model/budget_valueobject.go`**

Extend the const block and alias table (keep the existing comment style):

```go
const (
	ElementEnvelope ElementType = 0
	ElementCategory ElementType = 1
	ElementTag      ElementType = 2
	// Income-side types (plan view). Values are persisted in
	// budgets_elements.type and therefore frozen.
	ElementIncomeCategory ElementType = 3
	ElementIncomeEnvelope ElementType = 4
)
```

```go
var elementAliases = [...]string{
	ElementEnvelope: "envelope", ElementCategory: "category", ElementTag: "tag",
	ElementIncomeCategory: "income_category", ElementIncomeEnvelope: "income_envelope",
}
```

Restrict the wire parser (the income aliases exist for elementKey indexing only):

```go
// ElementTypeFromAlias parses an element type alias. Only the original three
// are valid wire inputs (drill-down requests); the income types are internal,
// so existing validation output is unchanged.
func ElementTypeFromAlias(alias string) (ElementType, error) {
	for v, a := range elementAliases {
		if a == alias && ElementType(v) <= ElementTag {
			return ElementType(v), nil
		}
	}
	return 0, errs.NewValidation("Validation failed", errs.FieldError{
		Key: "type", Message: "BudgetElementType with alias " + alias + " not exists", Code: errs.CodeBudgetInvalidElementTypeAlias,
		Params: map[string]any{"alias": alias},
	})
}
```

Add below the `Int16` method:

```go
// IsIncomeSide reports whether the element belongs to the plan view's income
// area. Tags and the original envelope/category types are expense-side.
func (t ElementType) IsIncomeSide() bool {
	return t == ElementIncomeCategory || t == ElementIncomeEnvelope
}

// EnvelopeTypeFromSide maps a create-envelope side alias to the element type
// that stores it. Absent ("") means expense, keeping the wire contract for
// existing clients.
func EnvelopeTypeFromSide(side string) (ElementType, error) {
	switch side {
	case "", "expense":
		return ElementEnvelope, nil
	case "income":
		return ElementIncomeEnvelope, nil
	}
	return 0, errs.NewValidation("Validation failed", errs.FieldError{
		Key: "side", Message: "The value you selected is not a valid choice.", Code: errs.CodeInvalidChoice,
	})
}
```

In `internal/model/budget.go`, next to `UpdateCurrency` (~line 184):

```go
// UpdateType changes the element's stored type (its side). One row per external
// id (UNIQUE(budget_id, external_id)) means a side change must be an in-place
// update — deleting the row would cascade its limits away.
func (e *BudgetElement) UpdateType(typ ElementType, now time.Time) {
	if e.Type != typ {
		e.Type = typ
		e.UpdatedAt = now
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/model/ -v -run 'TestElementType|TestIsIncomeSide|TestEnvelopeTypeFromSide'`
Expected: PASS. Then `go build ./...` — everything still compiles.

- [ ] **Step 5: Commit**

```bash
git add internal/model/budget_valueobject.go internal/model/budget.go internal/model/budget_side_test.go
git commit -m "feat(budget): income element types (3/4) with side helpers"
```

---

### Task 2: Coded errors + catalogue entries

**Files:**
- Modify: `internal/shared/errs/codes.go` (budget block ~line 37, `AllCodes` ~line 126)
- Modify: `locales/en.json` (the `errors.budget` object)
- Modify: `locales/ru.json` (the `errors.budget` object)

**Interfaces:**
- Produces: `errs.CodeBudgetEnvelopeSideMixed = "budget.envelope_side_mixed"`, `errs.CodeBudgetFolderSideMixed = "budget.folder_side_mixed"` — used by Tasks 5 and 6.
- Frozen English messages (asserted by later api tests): `"An envelope cannot mix income and expense categories"`, `"Income and expense elements cannot share a folder"`.

- [ ] **Step 1: Add the codes, run the i18n guard to see it fail**

In `internal/shared/errs/codes.go`, extend the budget const block:

```go
CodeBudgetEnvelopeSideMixed         = "budget.envelope_side_mixed"
CodeBudgetFolderSideMixed           = "budget.folder_side_mixed"
```

and add both to the `AllCodes` slice next to the other budget codes.

Run: `go test ./internal/test/i18ntest/ -v`
Expected: FAIL — the errs↔catalogue two-way guard reports `errors.budget.envelope_side_mixed` / `errors.budget.folder_side_mixed` missing from the catalogues.

- [ ] **Step 2: Add catalogue entries**

`locales/en.json`, inside `"errors" → "budget"` (keep JSON key order tidy — append after `"transaction_filter_required"`):

```json
"envelope_side_mixed": "An envelope cannot mix income and expense categories",
"folder_side_mixed": "Income and expense elements cannot share a folder"
```

`locales/ru.json`, same location:

```json
"envelope_side_mixed": "Конверт не может смешивать категории доходов и расходов",
"folder_side_mixed": "Доходные и расходные элементы не могут находиться в одной папке"
```

- [ ] **Step 3: Run the guard to verify it passes**

Run: `go test ./internal/test/i18ntest/ ./internal/shared/errs/... -v`
Expected: PASS (key parity en/ru, placeholder parity, AllCodes coverage).

- [ ] **Step 4: Commit**

```bash
git add internal/shared/errs/codes.go locales/en.json locales/ru.json
git commit -m "feat(budget): envelope/folder side-mixing error codes with en/ru catalogue entries"
```

---

### Task 3: `syncElements` learns income types (the reconciler)

This is the load-bearing task. `syncElements` (`internal/budget/move.go:129`) ends by **deleting every element row its walk did not mark `seen`**, and `budgets_elements_limits` has `ON DELETE CASCADE` — today's walk skips income categories (`move.go:199-201`) and ensures every envelope as expense (`move.go:178`), so without this task any income-typed row (and its limits) dies on the first envelope edit or move.

**Files:**
- Modify: `internal/budget/move.go` (`syncElements`, lines 129–263)
- Test: `internal/budget/api/income_elements_test.go` (create)

**Interfaces:**
- Consumes: `model.ElementIncomeCategory`, `model.ElementIncomeEnvelope`, `(*model.BudgetElement).UpdateType` (Task 1).
- Produces: after any element-mutating use case, income categories of participants have `type=3` rows and an envelope keeps the type its element row already had (`0` or `4`). `SetLimit`/`ChangeElementCurrency` self-heal (which call `syncElements`) therefore work for income categories with no further change.

- [ ] **Step 1: Write the failing tests**

Create `internal/budget/api/income_elements_test.go` (package `api_test`, same harness as `element_selfheal_test.go`):

```go
package api_test

import (
	"database/sql"
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// elementTypeAndKey reads the persisted element row for an external id. Takes
// the raw *sql.DB (pass h.db) so it does not depend on the harness type name.
func elementTypeAndKey(t *testing.T, db *sql.DB, budgetID, externalID string) (typ int, sortKey string, found bool) {
	t.Helper()
	err := db.QueryRow(
		`SELECT type, sort_key FROM budgets_elements WHERE budget_id = ? AND external_id = ?`,
		budgetID, externalID,
	).Scan(&typ, &sortKey)
	if err != nil {
		return 0, "", false
	}
	return typ, sortKey, true
}

func limitCount(t *testing.T, db *sql.DB, externalID string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM budgets_elements_limits l JOIN budgets_elements e ON l.element_id = e.id WHERE e.external_id = ?`,
		externalID,
	).Scan(&n); err != nil {
		t.Fatalf("limit count: %v", err)
	}
	return n
}

// TestSetLimit_IncomeCategorySelfHeals: setting a planned income value on an
// income category with no element row must create a type=3 row via the
// syncElements self-heal — and the row plus its limit must SURVIVE a later
// envelope write (the reconciler's delete-unseen pass is the regression here).
func TestSetLimit_IncomeCategorySelfHeals_AndSurvivesSync(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Income Reconciler Budget"))

	const incomeCatID = "cccc2222-0000-7000-8000-0000000000aa"
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Category(fixture.Category{ID: incomeCatID, UserID: seedUserID, Name: "Salary Later", Type: 1, Icon: "payments"})

	st, env := h.do(t, http.MethodPost, "/api/v1/budget/set-limit", tok, map[string]any{
		"budgetId": budgetID1, "elementId": incomeCatID, "period": "2099-01-01", "amount": "3000",
	})
	if st != http.StatusOK {
		t.Fatalf("set-limit on an income category = %d, want 200; body=%s", st, env.raw)
	}
	typ, _, found := elementTypeAndKey(t, h.db, budgetID1, incomeCatID)
	if !found || typ != 3 {
		t.Fatalf("self-healed income element: found=%v type=%d, want type=3", found, typ)
	}

	// Any element-mutating write reruns syncElements. Its delete-unseen pass
	// must keep the income element and its limit.
	const envID = "beee2222-0000-7000-8000-0000000000aa"
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", tok, map[string]any{
		"budgetId": budgetID1, "id": envID, "name": "Sync Env", "icon": "cart",
		"currencyId": usdID, "folderId": nil, "categories": []string{},
	})
	if st != http.StatusOK {
		t.Fatalf("create-envelope = %d; body=%s", st, env.raw)
	}
	typ, _, found = elementTypeAndKey(t, h.db, budgetID1, incomeCatID)
	if !found || typ != 3 {
		t.Fatalf("income element after sync: found=%v type=%d, want it kept with type=3", found, typ)
	}
	if n := limitCount(t, h.db, incomeCatID); n != 1 {
		t.Fatalf("planned income limits after sync = %d, want 1 (cascade delete = data loss)", n)
	}
}

// TestSyncElements_KeepsEnvelopeStoredType: a type=4 (income) envelope element
// must not be re-ensured as expense (duplicate row -> UNIQUE violation) nor
// deleted+recreated. We force the stored type via SQL because create-envelope
// only grows the side field in a later task — this pins the reconciler
// independently of the write path.
func TestSyncElements_KeepsEnvelopeStoredType(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Stored Side Budget"))

	const envID = "beee2222-0000-7000-8000-0000000000ab"
	st, env := h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", tok, map[string]any{
		"budgetId": budgetID1, "id": envID, "name": "Salaries", "icon": "payments",
		"currencyId": usdID, "folderId": nil, "categories": []string{},
	})
	if st != http.StatusOK {
		t.Fatalf("create-envelope = %d; body=%s", st, env.raw)
	}
	if _, err := h.db.Exec(`UPDATE budgets_elements SET type = 4 WHERE budget_id = ? AND external_id = ?`, budgetID1, envID); err != nil {
		t.Fatalf("force income type: %v", err)
	}

	// move-element on an unrelated element reruns syncElements.
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/move-element", tok, map[string]any{
		"budgetId": budgetID1, "id": envID, "folderId": nil, "afterId": nil,
	})
	if st != http.StatusOK {
		t.Fatalf("move-element = %d; body=%s", st, env.raw)
	}
	typ, _, found := elementTypeAndKey(t, h.db, budgetID1, envID)
	if !found || typ != 4 {
		t.Fatalf("envelope stored type after sync: found=%v type=%d, want 4 (side is immutable)", found, typ)
	}
	var rows int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM budgets_elements WHERE budget_id = ? AND external_id = ?`, budgetID1, envID).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("element rows for envelope = %d (err=%v), want exactly 1", rows, err)
	}
}
```

(Task 4 appends tests that need `"strings"` — add it to this file's imports then.)

Note: `h.db` is the harness's raw `*sql.DB` (sqlite — `?` placeholders are correct here; this mirrors `element_selfheal_test.go`'s `fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})` usage). If the harness field has a different name, check `newHarness` in `internal/budget/api/harness_test.go` (or wherever `element_selfheal_test.go` gets `h.db`) and match it.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/budget/api/ -run 'TestSetLimit_IncomeCategorySelfHeals|TestSyncElements_KeepsEnvelopeStoredType' -v`
Expected: FAIL — set-limit returns 400 `"BudgetElement not found"` (income categories are skipped by `syncElements`), and the stored-type test either hits a UNIQUE violation on move (500) or finds `type=0`.

- [ ] **Step 3: Implement in `internal/budget/move.go`**

Rewrite `syncElements`'s indexing + `ensure` + walks. The full replacement for lines 142–238 (everything between the participant-users block and the `assignMissingKeys` call):

```go
	// Index existing elements by "<externalId>-<typeAlias>" AND by external id.
	// The external index is what makes a type (side) change an in-place update:
	// one row per external id (UNIQUE(budget_id, external_id)), and a
	// delete+recreate would cascade the row's limits away — while the recreate
	// would trip the unique constraint, because saves run before the
	// delete-unseen pass below.
	byKey := map[string]*model.BudgetElement{}
	byExternal := map[string]*model.BudgetElement{}
	for _, e := range b.elements {
		byKey[elementKey(e.ExternalID.String(), e.Type)] = e
		byExternal[e.ExternalID.String()] = e
	}
	seen := map[string]bool{}
	created := map[string]*model.BudgetElement{}
	dirty := map[string]*model.BudgetElement{}
	// live marks element keys that belong in the listing: a non-archived
	// participant element that is not an envelope-child category.
	live := map[string]bool{}

	mark := func(e *model.BudgetElement) { dirty[e.ID.String()] = e }

	ensure := func(externalID vo.Id, typ model.ElementType) (*model.BudgetElement, string) {
		if e, ok := byExternal[externalID.String()]; ok && e.Type != typ {
			delete(byKey, elementKey(externalID.String(), e.Type))
			e.UpdateType(typ, now)
			byKey[elementKey(externalID.String(), typ)] = e
			mark(e)
		}
		key := elementKey(externalID.String(), typ)
		seen[key] = true
		if e, ok := byKey[key]; ok {
			return e, key
		}
		// A missing element starts keyless; it is given one below, appended to the
		// end of the no-folder group.
		e := model.NewBudgetElement(s.elements.NextIdentity(), budgetID, externalID, typ, nil, nil, now)
		byKey[key] = e
		byExternal[externalID.String()] = e
		created[key] = e
		return e, key
	}
	forceUnset := func(e *model.BudgetElement) {
		if !e.IsSortKeyUnset() {
			e.UpdateSortKey("", now)
			mark(e)
		}
	}

	// --- envelopes (+ collect child categories) ---
	childCategories := map[string]bool{}
	for _, env := range b.envelopes {
		// The element row stores the envelope's (immutable) side; default to
		// expense only when no row exists yet.
		typ := model.ElementEnvelope
		if e, ok := byExternal[env.ID.String()]; ok && e.Type == model.ElementIncomeEnvelope {
			typ = model.ElementIncomeEnvelope
		}
		e, key := ensure(env.ID, typ)
		if env.IsArchived {
			forceUnset(e)
		} else {
			live[key] = true
		}
		catIDs, cerr := s.envelopes.EnvelopeCategoryIDs(ctx, env.ID)
		if cerr != nil {
			return cerr
		}
		for _, c := range catIDs {
			childCategories[c.String()] = true
		}
	}

	// --- categories (both sides; the type encodes the side) ---
	cats, err := s.metadata.CategoriesByOwners(ctx, userIDs)
	if err != nil {
		return err
	}
	for _, c := range cats {
		typ := model.ElementCategory
		if c.IsIncome {
			typ = model.ElementIncomeCategory
		}
		cid, perr := vo.ParseId(c.ID)
		if perr != nil {
			return perr
		}
		e, key := ensure(cid, typ)
		if childCategories[c.ID] {
			// A category that belongs to an envelope is hidden from the top level:
			// unset key + no folder.
			forceUnset(e)
			if e.FolderID != nil {
				e.UpdateFolder(nil, now)
				mark(e)
			}
		} else if c.IsArchived {
			forceUnset(e)
		} else {
			live[key] = true
		}
	}

	// --- tags ---
	tags, err := s.metadata.TagsByOwners(ctx, userIDs)
	if err != nil {
		return err
	}
	for _, t := range tags {
		tid, perr := vo.ParseId(t.ID)
		if perr != nil {
			return perr
		}
		e, key := ensure(tid, model.ElementTag)
		if t.IsArchived {
			forceUnset(e)
		} else {
			live[key] = true
		}
	}
```

The rest of the function (`assignMissingKeys`, persist, delete-unseen) is unchanged. Also update the function's doc comment: replace "every participant envelope / expense category / tag" with "every participant envelope / category (both sides) / tag", and add one line: "Types are reconciled in place by external id — a side change never deletes a row (limits cascade)."

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/budget/api/ -run 'TestSetLimit_IncomeCategorySelfHeals|TestSyncElements_KeepsEnvelopeStoredType' -v`
Expected: PASS.
Then run the whole budget package + apiparity to prove nothing regressed:
Run: `go test ./internal/budget/... ./internal/test/apiparity/`
Expected: PASS with **zero golden diffs** (income elements exist only in the new tests' budgets).

- [ ] **Step 5: Commit**

```bash
git add internal/budget/move.go internal/budget/api/income_elements_test.go
git commit -m "feat(budget): syncElements reconciles income-typed elements in place"
```

---

### Task 4: Seed income categories at budget creation / accept

**Files:**
- Modify: `internal/budget/create.go` (`seedCategoryElements`, lines 93–128)
- Test: `internal/budget/api/income_elements_test.go` (extend)

**Interfaces:**
- Consumes: `model.ElementIncomeCategory` (Task 1).
- Produces: `seedCategoryElements` seeds expense categories first, then income categories (type=3), continuing one sort-key chain; signature unchanged, so the `AcceptAccess` call site (`internal/budget/accesssvc.go:97`) needs no edit — its `skip` map is keyed by external id and side-agnostic.

- [ ] **Step 1: Write the failing test**

Append to `internal/budget/api/income_elements_test.go`:

```go
// TestCreateBudget_SeedsIncomeCategories: a pre-existing income category gets a
// live type=3 element at budget creation — and get-budget must not show it
// (the frozen contract).
func TestCreateBudget_SeedsIncomeCategories(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	const incomeCatID = "cccc2222-0000-7000-8000-0000000000ab"
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Category(fixture.Category{ID: incomeCatID, UserID: seedUserID, Name: "Wages", Type: 1, Icon: "payments"})

	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Seeded Income Budget"))

	typ, sortKey, found := elementTypeAndKey(t, h.db, budgetID1, incomeCatID)
	if !found {
		t.Fatalf("income category must be seeded at budget creation")
	}
	if typ != 3 {
		t.Errorf("seeded type=%d want 3", typ)
	}
	if sortKey == "" {
		t.Errorf("live income element must carry a sort key")
	}

	st, b := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1, tok, nil)
	if st != http.StatusOK {
		t.Fatalf("get-budget=%d body=%s", st, b.raw)
	}
	if strings.Contains(string(b.Data), incomeCatID) {
		t.Errorf("get-budget must not surface the income category anywhere; body=%s", b.Data)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/budget/api/ -run TestCreateBudget_SeedsIncomeCategories -v`
Expected: FAIL — "income category must be seeded at budget creation" (seeding still skips `IsIncome`).

- [ ] **Step 3: Implement in `internal/budget/create.go`**

Replace `seedCategoryElements`'s loop with a two-pass walk (expense first, then income — the income block continues the same key chain, per the spec):

```go
// seedCategoryElements creates a budget element for each category of the user —
// expense categories first (type=category), then income categories
// (type=income_category), one continuous key chain. Archived categories get the
// unset key. Ids in skip already have an element in the budget (accept after an
// earlier membership) and are left untouched. Returns the key the next element
// follows.
func (s *Service) seedCategoryElements(ctx context.Context, userID, budgetID vo.Id, after sortkey.Key, now time.Time, skip map[vo.Id]bool) (sortkey.Key, error) {
	cats, err := s.metadata.CategoriesByOwners(ctx, []vo.Id{userID})
	if err != nil {
		return after, err
	}
	for _, wantIncome := range []bool{false, true} {
		for _, c := range cats {
			if c.IsIncome != wantIncome {
				continue
			}
			extID, perr := vo.ParseId(c.ID)
			if perr != nil {
				return after, perr
			}
			if skip[extID] {
				continue
			}
			typ := model.ElementCategory
			if c.IsIncome {
				typ = model.ElementIncomeCategory
			}
			key := sortkey.Key("")
			if !c.IsArchived {
				next, kerr := nextSeedKey(after)
				if kerr != nil {
					return after, kerr
				}
				key, after = next, next
			}
			el := model.NewBudgetElement(s.elements.NextIdentity(), budgetID, extID, typ, nil, nil, now)
			el.SetSortKey(key)
			if serr := s.elements.SaveElement(ctx, el); serr != nil {
				return after, serr
			}
		}
	}
	return after, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/budget/... ./internal/test/apiparity/ -count=1`
Expected: PASS, zero golden diffs. (The apiparity fixture's `CatSalary` now gets seeded as an income element in scenarios that call create-budget — the goldens must not change because get-budget never renders income elements; if any golden diff appears here, that is a Task 7 exclusion bug, not a golden to regenerate.)

- [ ] **Step 5: Commit**

```bash
git add internal/budget/create.go internal/budget/api/income_elements_test.go
git commit -m "feat(budget): seed income categories as income-typed elements"
```

---

### Task 5: Folder sides — derivation + cross-side move rejection

**Files:**
- Modify: `internal/budget/move.go` (`MoveElement` ~line 58, add helpers at file end)
- Test: `internal/budget/api/income_elements_test.go` (extend)

**Interfaces:**
- Consumes: `ElementType.IsIncomeSide` (Task 1), `errs.CodeBudgetFolderSideMixed` (Task 2).
- Produces (Task 6 reuses both):
  - `func sideMixed(elements []*model.BudgetElement, folderID vo.Id, typ model.ElementType) bool`
  - `func folderSideMixedErr() error` — validation error, field `folderId`, message `"Income and expense elements cannot share a folder"`, code `errs.CodeBudgetFolderSideMixed`

- [ ] **Step 1: Write the failing test**

Append to `internal/budget/api/income_elements_test.go`:

```go
// TestMoveElement_CrossSideRejected: folder sides are derived from members;
// mixing income and expense elements in one folder is rejected with the coded
// error, and emptying a folder reverts it to neutral.
func TestMoveElement_CrossSideRejected(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	const incomeCatID = "cccc2222-0000-7000-8000-0000000000ac"
	const expenseCatID = "cccc2222-0000-7000-8000-0000000000ad"
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Category(fixture.Category{ID: incomeCatID, UserID: seedUserID, Name: "Wages Move", Type: 1, Icon: "payments"})
	f.Category(fixture.Category{ID: expenseCatID, UserID: seedUserID, Name: "Rent Move", Type: 0, Icon: "home"})
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Folder Sides Budget"))

	const folderID = "bfff2222-0000-7000-8000-0000000000aa"
	h.do(t, http.MethodPost, "/api/v1/budget/create-folder", tok, map[string]any{
		"budgetId": budgetID1, "id": folderID, "name": "Mixed?",
	})

	// Neutral folder accepts an income element.
	st, env := h.do(t, http.MethodPost, "/api/v1/budget/move-element", tok, map[string]any{
		"budgetId": budgetID1, "id": incomeCatID, "folderId": folderID, "afterId": nil,
	})
	if st != http.StatusOK {
		t.Fatalf("income into neutral folder = %d, want 200; body=%s", st, env.raw)
	}

	// Now income-sided: an expense element is rejected with the coded error.
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/move-element", tok, map[string]any{
		"budgetId": budgetID1, "id": expenseCatID, "folderId": folderID, "afterId": nil,
	})
	if st != http.StatusBadRequest {
		t.Fatalf("expense into income folder = %d, want 400; body=%s", st, env.raw)
	}
	if !strings.Contains(string(env.raw), "Income and expense elements cannot share a folder") {
		t.Errorf("want the folder side-mixing message; body=%s", env.raw)
	}

	// Emptying the folder reverts it to neutral: the expense move then succeeds.
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/move-element", tok, map[string]any{
		"budgetId": budgetID1, "id": incomeCatID, "folderId": nil, "afterId": nil,
	})
	if st != http.StatusOK {
		t.Fatalf("income out of folder = %d; body=%s", st, env.raw)
	}
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/move-element", tok, map[string]any{
		"budgetId": budgetID1, "id": expenseCatID, "folderId": folderID, "afterId": nil,
	})
	if st != http.StatusOK {
		t.Fatalf("expense into re-neutraled folder = %d, want 200; body=%s", st, env.raw)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/budget/api/ -run TestMoveElement_CrossSideRejected -v`
Expected: FAIL — the expense move into the income folder returns 200 (no enforcement yet).

- [ ] **Step 3: Implement in `internal/budget/move.go`**

Add the helpers at the end of the file:

```go
// folderSide reports the folder's derived side over its member elements: no
// members = neutral (both false). Archived members count — a folder holding
// only an archived income category is still income-sided.
func folderSide(elements []*model.BudgetElement, folderID vo.Id) (income, expense bool) {
	for _, e := range elements {
		if e.FolderID == nil || !e.FolderID.Equal(folderID) {
			continue
		}
		if e.Type.IsIncomeSide() {
			income = true
		} else {
			expense = true
		}
	}
	return income, expense
}

// sideMixed reports whether placing an element of type typ into the folder
// would mix income and expense sides. Same-side members never conflict, so the
// element being re-placed inside its own folder needs no exclusion.
func sideMixed(elements []*model.BudgetElement, folderID vo.Id, typ model.ElementType) bool {
	income, expense := folderSide(elements, folderID)
	if typ.IsIncomeSide() {
		return expense
	}
	return income
}

func folderSideMixedErr() error {
	return errs.NewValidation("Validation failed", errs.FieldError{
		Key: "folderId", Message: "Income and expense elements cannot share a folder", Code: errs.CodeBudgetFolderSideMixed,
	})
}
```

Add `"github.com/econumo/econumo/internal/shared/errs"` to the file's imports.

In `MoveElement`, after the `moved` resolution loop (after line 56) and before `now := s.clock.Now()`:

```go
	if moved != nil && folderID != nil && sideMixed(b.elements, *folderID, moved.Type) {
		return nil, folderSideMixedErr()
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/budget/... ./internal/test/apiparity/ -count=1`
Expected: PASS, zero golden diffs (existing scenarios only move expense-side elements into expense/neutral folders).

- [ ] **Step 5: Commit**

```bash
git add internal/budget/move.go internal/budget/api/income_elements_test.go
git commit -m "feat(budget): derived folder sides; move-element rejects cross-side placement"
```

---

### Task 6: Envelope side — DTO field, homogeneity validation, side-aware children, MCP param

**Files:**
- Modify: `internal/model/budget_dto.go` (`CreateEnvelopeRequest`, ~line 281)
- Modify: `internal/budget/envelopes.go` (`CreateEnvelope`, `UpdateEnvelope`, `envelopeChildren`, `newEnvelopeElementResult`)
- Modify: `internal/budget/mcp/mcp.go` (`createEnvelopeInput` ~line 43, create_envelope tool ~line 249)
- Test: `internal/budget/api/income_elements_test.go` (extend)

**Interfaces:**
- Consumes: `model.EnvelopeTypeFromSide`, `model.ElementIncomeEnvelope`, `IsIncomeSide` (Task 1); `errs.CodeBudgetEnvelopeSideMixed`, `errs.CodeCategoryNotFound` (Task 2 / existing); `sideMixed`, `folderSideMixedErr` (Task 5).
- Produces:
  - `CreateEnvelopeRequest.Side string` (json `side`, optional: `""`/`"expense"`/`"income"`).
  - `func (s *Service) validateEnvelopeCategories(ctx context.Context, b *budgetAggregate, income bool, categoryIDs []string) error`
  - `envelopeChildren(ctx, b, categoryIDs, income bool)` — side-filtered children (plan 02's builder will need the same side-awareness).
  - `newEnvelopeElementResult(..., typ model.ElementType, ...)` — the result `Type` reflects the stored side.

- [ ] **Step 1: Write the failing tests**

Append to `internal/budget/api/income_elements_test.go`:

```go
// TestCreateEnvelope_IncomeSide covers the envelope-side matrix: income
// envelope with income children OK (children rendered), homogeneity rejections
// both directions, unknown category rejected, invalid side rejected, and
// cross-side folder placement rejected at create time.
func TestCreateEnvelope_IncomeSide(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	const incomeCatID = "cccc2222-0000-7000-8000-0000000000ae"
	const expenseCatID = "cccc2222-0000-7000-8000-0000000000af"
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Category(fixture.Category{ID: incomeCatID, UserID: seedUserID, Name: "Wages Env", Type: 1, Icon: "payments"})
	f.Category(fixture.Category{ID: expenseCatID, UserID: seedUserID, Name: "Rent Env", Type: 0, Icon: "home"})
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Envelope Sides Budget"))

	envBody := func(id, side string, cats []string, folderID any) map[string]any {
		return map[string]any{
			"budgetId": budgetID1, "id": id, "name": "Env " + id[:4], "icon": "cart",
			"currencyId": usdID, "folderId": folderID, "side": side, "categories": cats,
		}
	}

	// Income envelope with an income child: created, child rendered, type=4.
	const incomeEnvID = "beee2222-0000-7000-8000-0000000000b0"
	st, env := h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", tok, envBody(incomeEnvID, "income", []string{incomeCatID}, nil))
	if st != http.StatusOK {
		t.Fatalf("create income envelope = %d; body=%s", st, env.raw)
	}
	if !strings.Contains(string(env.Data), incomeCatID) {
		t.Errorf("income child must be rendered in the result; body=%s", env.Data)
	}
	if typ, _, _ := elementTypeAndKey(t, h.db, budgetID1, incomeEnvID); typ != 4 {
		t.Errorf("income envelope element type=%d want 4", typ)
	}

	// Homogeneity: expense child in an income envelope.
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", tok,
		envBody("beee2222-0000-7000-8000-0000000000b1", "income", []string{expenseCatID}, nil))
	if st != http.StatusBadRequest || !strings.Contains(string(env.raw), "An envelope cannot mix income and expense categories") {
		t.Fatalf("expense child in income envelope: st=%d body=%s", st, env.raw)
	}

	// Homogeneity the other way: income child in a (default expense) envelope.
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", tok,
		envBody("beee2222-0000-7000-8000-0000000000b2", "", []string{incomeCatID}, nil))
	if st != http.StatusBadRequest || !strings.Contains(string(env.raw), "An envelope cannot mix income and expense categories") {
		t.Fatalf("income child in expense envelope: st=%d body=%s", st, env.raw)
	}

	// Existence/ownership: unknown category id.
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", tok,
		envBody("beee2222-0000-7000-8000-0000000000b3", "", []string{"9999aaaa-0000-7000-8000-00000000dead"}, nil))
	if st != http.StatusBadRequest || !strings.Contains(string(env.raw), "Category not found") {
		t.Fatalf("unknown child category: st=%d body=%s", st, env.raw)
	}

	// Invalid side alias.
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", tok,
		envBody("beee2222-0000-7000-8000-0000000000b4", "both", []string{}, nil))
	if st != http.StatusBadRequest {
		t.Fatalf("invalid side: st=%d body=%s", st, env.raw)
	}

	// Cross-side folder placement at create time: put the expense category into
	// a folder, then try to create an income envelope inside that folder.
	const folderID = "bfff2222-0000-7000-8000-0000000000ab"
	h.do(t, http.MethodPost, "/api/v1/budget/create-folder", tok, map[string]any{
		"budgetId": budgetID1, "id": folderID, "name": "Expenses",
	})
	h.do(t, http.MethodPost, "/api/v1/budget/move-element", tok, map[string]any{
		"budgetId": budgetID1, "id": expenseCatID, "folderId": folderID, "afterId": nil,
	})
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", tok,
		envBody("beee2222-0000-7000-8000-0000000000b5", "income", []string{}, folderID))
	if st != http.StatusBadRequest || !strings.Contains(string(env.raw), "Income and expense elements cannot share a folder") {
		t.Fatalf("income envelope into expense folder: st=%d body=%s", st, env.raw)
	}

	// Update keeps the stored side: income children stay valid on update, and an
	// expense child is still rejected (side is immutable, no side field on update).
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/update-envelope", tok, map[string]any{
		"budgetId": budgetID1, "id": incomeEnvID, "name": "Salaries", "icon": "payments",
		"currencyId": usdID, "isArchived": 0, "categories": []string{incomeCatID},
	})
	if st != http.StatusOK {
		t.Fatalf("update income envelope with income child = %d; body=%s", st, env.raw)
	}
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/update-envelope", tok, map[string]any{
		"budgetId": budgetID1, "id": incomeEnvID, "name": "Salaries", "icon": "payments",
		"currencyId": usdID, "isArchived": 0, "categories": []string{expenseCatID},
	})
	if st != http.StatusBadRequest || !strings.Contains(string(env.raw), "An envelope cannot mix income and expense categories") {
		t.Fatalf("update income envelope with expense child: st=%d body=%s", st, env.raw)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/budget/api/ -run TestCreateEnvelope_IncomeSide -v`
Expected: FAIL — the unknown `side` JSON field is ignored today, the envelope is created as expense, no rejection occurs.

- [ ] **Step 3: Implement**

`internal/model/budget_dto.go` — add the field to `CreateEnvelopeRequest` (last field, with the swagger-visible json tag):

```go
	// Side selects the envelope's (immutable) side: "" or "expense" (default),
	// or "income". Income envelopes exist only in the plan view.
	Side string `json:"side"`
```

`internal/budget/envelopes.go`:

(a) New validation helper (place after `envelopeChildren`):

```go
// validateEnvelopeCategories checks every requested child id parses, belongs to
// a budget participant, and matches the envelope's side (the homogeneity rule).
// This is deliberately stricter than the historical behavior, which accepted
// arbitrary ids and hid mismatches at read time.
func (s *Service) validateEnvelopeCategories(ctx context.Context, b *budgetAggregate, income bool, categoryIDs []string) error {
	if len(categoryIDs) == 0 {
		return nil
	}
	userIDs := []vo.Id{b.budget.UserID}
	for _, a := range b.access {
		if a.IsAccepted && a.Role != roleGuest() {
			userIDs = append(userIDs, a.UserID)
		}
	}
	cats, err := s.metadata.CategoriesByOwners(ctx, userIDs)
	if err != nil {
		return err
	}
	byID := map[string]model.CategoryMeta{}
	for _, c := range cats {
		byID[c.ID] = c
	}
	for _, raw := range categoryIDs {
		if _, perr := vo.ParseId(raw); perr != nil {
			return model.ValidateBlank(map[string]string{"categories": ""})
		}
		c, ok := byID[raw]
		if !ok {
			return errs.NewValidation("Validation failed", errs.FieldError{
				Key: "categories", Message: "Category not found", Code: errs.CodeCategoryNotFound,
			})
		}
		if c.IsIncome != income {
			return errs.NewValidation("Validation failed", errs.FieldError{
				Key: "categories", Message: "An envelope cannot mix income and expense categories", Code: errs.CodeBudgetEnvelopeSideMixed,
			})
		}
	}
	return nil
}
```

Add `"github.com/econumo/econumo/internal/shared/errs"` to the imports.

(b) `CreateEnvelope`: after the `ValidateName` call, parse the side:

```go
	envType, err := model.EnvelopeTypeFromSide(req.Side)
	if err != nil {
		return nil, err
	}
```

After the `canUpdate` check, add the two enforcement calls:

```go
	if err := s.validateEnvelopeCategories(ctx, b, envType.IsIncomeSide(), req.Categories); err != nil {
		return nil, err
	}
	if folderID != nil && sideMixed(b.elements, *folderID, envType) {
		return nil, folderSideMixedErr()
	}
```

Change the element construction (line 60) to use the side's type:

```go
		el := model.NewBudgetElement(s.elements.NextIdentity(), budgetID, envelopeID, envType, &curID, folderID, now)
```

And the two result calls at the bottom of `CreateEnvelope`:

```go
	children, err := s.envelopeChildren(ctx, b, req.Categories, envType.IsIncomeSide())
	...
	return &model.CreateEnvelopeResult{Item: newEnvelopeElementResult(envelopeID, req.Name, req.Icon, curID, folderID, 0, false, envType, children)}, nil
```

(c) `UpdateEnvelope`: the stored side comes from the element row already fetched at line 128. Introduce a variable and validate before the category replacement block:

```go
	envType := model.ElementEnvelope
```

(declare next to `elementKeyOfEnvelope`), and inside the `if eerr == nil` block after `folderID = el.FolderID`:

```go
			envType = el.Type
```

Then, immediately after that `if eerr == nil` block (still inside the tx, before "Replace category assignments"):

```go
		if verr := s.validateEnvelopeCategories(txCtx, b, envType.IsIncomeSide(), req.Categories); verr != nil {
			return verr
		}
```

And the result calls:

```go
	children, err := s.envelopeChildren(ctx, b, req.Categories, envType.IsIncomeSide())
	...
	return &model.UpdateEnvelopeResult{Item: newEnvelopeElementResult(
		envelopeID, req.Name, req.Icon, curID, folderID,
		elementIndex(b.elements, folderID, elementKeyOfEnvelope), req.IsArchived == 1, envType, children,
	)}, nil
```

(d) `envelopeChildren` becomes side-aware — change the signature and the filter:

```go
func (s *Service) envelopeChildren(ctx context.Context, b *budgetAggregate, categoryIDs []string, income bool) ([]model.ChildElementResult, error) {
```

and replace the `if c.IsIncome { continue }` filter with:

```go
		if c.IsIncome != income {
			continue // children render only on their own side
		}
```

The child `Type` stays `int(model.ElementCategory.Int16())` for expense children; for income children emit the income type:

```go
	childType := model.ElementCategory
	if income {
		childType = model.ElementIncomeCategory
	}
```

(compute once before the loop; use `Type: int(childType.Int16())` in the result literal).

(e) `newEnvelopeElementResult` gains the type parameter — new signature and the one changed field:

```go
func newEnvelopeElementResult(id vo.Id, name, icon string, currencyID vo.Id, folderID *vo.Id, position int, archived bool, typ model.ElementType, children []model.ChildElementResult) model.ParentElementResult {
```

with `Type: int(typ.Int16()),` in the struct literal.

(f) `internal/budget/mcp/mcp.go` — `createEnvelopeInput` gains (after `FolderID`):

```go
	Side        string   `json:"side,omitempty" jsonschema:"envelope side: expense (default) or income; income envelopes appear only in the plan view"`
```

and the `CreateEnvelope` call inside the create_envelope tool adds `Side: in.Side,` after `FolderId`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/budget/... -count=1 -v -run TestCreateEnvelope_IncomeSide`
Expected: PASS.
Then: `go test ./internal/budget/... ./internal/test/apiparity/ ./internal/test/mcpparity/ -count=1`
Expected: apiparity FAILS on the `budget_structure_writes` scenario — its `update-envelope` call sends `categories: [CatSalary]` (an income category) and now gets the homogeneity 400. **Do not regenerate yet** — Task 9 fixes the scenario deliberately. mcpparity fails on the tools/list golden (new `side` param) — also deferred to Task 9. Everything else must pass.

- [ ] **Step 5: Commit**

```bash
git add internal/model/budget_dto.go internal/budget/envelopes.go internal/budget/mcp/mcp.go internal/budget/api/income_elements_test.go
git commit -m "feat(budget): stored immutable envelope side with homogeneity validation"
```

---

### Task 7: get-budget exclusions — income envelopes and income-sided folders

**Files:**
- Modify: `internal/budget/builder_structure_build.go` (`buildStructure`, lines 48–110)
- Test: `internal/budget/api/income_elements_test.go` (extend)

**Interfaces:**
- Consumes: `ElementType.IsIncomeSide`, `model.ElementIncomeEnvelope` (Task 1).
- Produces: `get-budget` never emits an income-sided envelope, and folders whose members include an income-typed element drop out of `structure.folders`. Everything else is byte-identical.

- [ ] **Step 1: Write the failing test**

Append to `internal/budget/api/income_elements_test.go`:

```go
// TestGetBudget_ExcludesIncomeEnvelopesAndFolders: an income envelope never
// renders in get-budget, an income-sided folder disappears from the folder
// list, and a re-neutraled folder comes back.
func TestGetBudget_ExcludesIncomeEnvelopesAndFolders(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	const incomeCatID = "cccc2222-0000-7000-8000-0000000000b6"
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Category(fixture.Category{ID: incomeCatID, UserID: seedUserID, Name: "Wages GB", Type: 1, Icon: "payments"})
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "GB Exclusion Budget"))

	const incomeEnvID = "beee2222-0000-7000-8000-0000000000b7"
	st, env := h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", tok, map[string]any{
		"budgetId": budgetID1, "id": incomeEnvID, "name": "Salaries GB", "icon": "payments",
		"currencyId": usdID, "folderId": nil, "side": "income", "categories": []string{incomeCatID},
	})
	if st != http.StatusOK {
		t.Fatalf("create income envelope = %d; body=%s", st, env.raw)
	}

	const folderID = "bfff2222-0000-7000-8000-0000000000ac"
	h.do(t, http.MethodPost, "/api/v1/budget/create-folder", tok, map[string]any{
		"budgetId": budgetID1, "id": folderID, "name": "Income Folder",
	})
	// Neutral folder: visible.
	st, b := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1, tok, nil)
	if st != http.StatusOK {
		t.Fatalf("get-budget=%d body=%s", st, b.raw)
	}
	if !strings.Contains(string(b.Data), folderID) {
		t.Fatalf("neutral folder must be visible; body=%s", b.Data)
	}
	if strings.Contains(string(b.Data), incomeEnvID) || strings.Contains(string(b.Data), incomeCatID) {
		t.Fatalf("income envelope/category must not render in get-budget; body=%s", b.Data)
	}

	// Give the folder an income member: it disappears from get-budget.
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/move-element", tok, map[string]any{
		"budgetId": budgetID1, "id": incomeEnvID, "folderId": folderID, "afterId": nil,
	})
	if st != http.StatusOK {
		t.Fatalf("move income envelope into folder = %d; body=%s", st, env.raw)
	}
	_, b = h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1, tok, nil)
	if strings.Contains(string(b.Data), folderID) {
		t.Fatalf("income-sided folder must be filtered from get-budget; body=%s", b.Data)
	}

	// Empty it again: back to neutral, visible again.
	h.do(t, http.MethodPost, "/api/v1/budget/move-element", tok, map[string]any{
		"budgetId": budgetID1, "id": incomeEnvID, "folderId": nil, "afterId": nil,
	})
	_, b = h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1, tok, nil)
	if !strings.Contains(string(b.Data), folderID) {
		t.Fatalf("re-neutraled folder must reappear; body=%s", b.Data)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/budget/api/ -run TestGetBudget_ExcludesIncomeEnvelopes -v`
Expected: FAIL — the income envelope renders (with empty children) and the income-sided folder stays listed.

- [ ] **Step 3: Implement in `internal/budget/builder_structure_build.go`**

At the top of `buildStructure`, right after `options := s.elementOptions(b)`, derive the exclusion sets:

```go
	// Income-sided envelopes and folders exist only in the plan view; the
	// budget view's wire contract is frozen without them.
	incomeEnvelopes := map[string]bool{}
	incomeFolders := map[string]bool{}
	for _, e := range b.elements {
		if e.Type == model.ElementIncomeEnvelope {
			incomeEnvelopes[e.ExternalID.String()] = true
		}
		if e.Type.IsIncomeSide() && e.FolderID != nil {
			incomeFolders[e.FolderID.String()] = true
		}
	}
```

Change the folders loop (lines 53–57) to filter and keep the dense wire position:

```go
	for _, fl := range sorted {
		if incomeFolders[fl.ID.String()] {
			continue
		}
		folders = append(folders, model.BudgetFolderResult{Id: fl.ID.String(), Name: fl.Name, Position: len(folders)})
	}
```

Guard the envelope loop (line 75):

```go
	for _, env := range b.envelopes {
		if incomeEnvelopes[env.ID.String()] {
			continue
		}
		...
```

No other builder needs a change: `buildFilters` already keeps `f.categories`/`f.tags` expense-side, and every limits/spending lookup is keyed `<externalId>-<typeAlias>`, so income-typed rows match nothing.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/budget/... -count=1`
Expected: PASS (apiparity still red only on the two known Task 9 items — verify with `go test ./internal/test/apiparity/ -count=1 2>&1 | tail -20` that the only failing scenario is `budget_structure_writes`).

- [ ] **Step 5: Commit**

```bash
git add internal/budget/builder_structure_build.go internal/budget/api/income_elements_test.go
git commit -m "feat(budget): get-budget filters income envelopes and income-sided folders"
```

---

### Task 8: Dirty-data cleanup migration

Envelope children were never validated, so production rows in `budgets_envelopes_categories` may reference income categories — stored but invisible. Every pre-existing envelope is expense-sided, so those links are removed once; without this, the plan view would surface them and the homogeneity rule would reject edits to envelopes the user never knowingly mixed.

**Files:**
- Create: `internal/infra/storage/migrations/sqlite/20260811000000.sql`
- Create: `internal/infra/storage/migrations/pgsql/20260811000000.sql`
- Test: `internal/infra/storage/migrations/migration_20260811_test.go` (create; follows the `runUpTo`/`runAll` pattern of `migration_20260803_test.go`)

**Interfaces:**
- Consumes: `migrations.SQLite()`, `migrate.Run` (existing test helpers pattern).
- Produces: post-migration, no `budgets_envelopes_categories` row references a `categories.type = 1` row.

- [ ] **Step 1: Write the failing test**

Create `internal/infra/storage/migrations/migration_20260811_test.go`:

```go
package migrations_test

// Verifies the 20260811000000 migration: income-category links silently stored
// on (expense) envelopes are removed; expense links are untouched.

import (
	"testing"
)

const envelopeSideCleanupVersion = "20260811000000"

func TestMigration20260811_RemovesIncomeEnvelopeChildren(t *testing.T) {
	db := runUpTo(t, "envelope_side_cleanup", envelopeSideCleanupVersion)

	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	mustExec(`INSERT INTO users (id, identifier, email, name, password, salt, avatar, created_at, updated_at)
	          VALUES ('u1', 'u1', 'u@e.test', 'U', 'x', '', 'face:sky', '2026-01-01 00:00:00', '2026-01-01 00:00:00')`)
	mustExec(`INSERT INTO currencies (id, code, name, symbol, fraction_digits, created_at, updated_at)
	          VALUES ('cur1', 'USD', 'Dollar', '$', 2, '2026-01-01 00:00:00', '2026-01-01 00:00:00')`)
	mustExec(`INSERT INTO categories (id, user_id, name, position, type, is_archived, created_at, updated_at)
	          VALUES ('cat-exp', 'u1', 'Food', 0, 0, 0, '2026-01-01 00:00:00', '2026-01-01 00:00:00')`)
	mustExec(`INSERT INTO categories (id, user_id, name, position, type, is_archived, created_at, updated_at)
	          VALUES ('cat-inc', 'u1', 'Salary', 1, 1, 0, '2026-01-01 00:00:00', '2026-01-01 00:00:00')`)
	mustExec(`INSERT INTO budgets (id, user_id, currency_id, name, started_at, created_at, updated_at)
	          VALUES ('b1', 'u1', 'cur1', 'B', '2026-01-01 00:00:00', '2026-01-01 00:00:00', '2026-01-01 00:00:00')`)
	mustExec(`INSERT INTO budgets_envelopes (id, budget_id, name, icon, is_archived, created_at, updated_at)
	          VALUES ('env1', 'b1', 'Env', 'cart', 0, '2026-01-01 00:00:00', '2026-01-01 00:00:00')`)
	mustExec(`INSERT INTO budgets_envelopes_categories (envelope_id, category_id) VALUES ('env1', 'cat-exp')`)
	mustExec(`INSERT INTO budgets_envelopes_categories (envelope_id, category_id) VALUES ('env1', 'cat-inc')`)

	runAll(t, db)

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM budgets_envelopes_categories WHERE category_id = 'cat-inc'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("income child links after migration = %d (err=%v), want 0", n, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM budgets_envelopes_categories WHERE category_id = 'cat-exp'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("expense child links after migration = %d (err=%v), want 1 (untouched)", n, err)
	}
}
```

**Column-name check before running:** the INSERT column lists above are best-effort — verify each against the actual schema (`internal/infra/storage/migrations/sqlite/20260101000000.sql` and later migrations, e.g. whether `users` still requires `salt`, the exact `budgets_envelopes*` table/column names, whether `categories` has an `icon` column that is NOT NULL). Adjust the INSERTs to satisfy NOT NULL constraints — the test's substance is the two link rows and the two counts, not the scaffolding rows. The `migration_20260803_test.go` file in the same package contains working INSERTs to crib from.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/infra/storage/migrations/ -run TestMigration20260811 -v`
Expected: FAIL — `runUpTo` cannot find version `20260811000000` (or `runAll` leaves both links: count = 1 for `cat-inc`).

- [ ] **Step 3: Write the migration (both engines, identical SQL)**

`internal/infra/storage/migrations/sqlite/20260811000000.sql` and `internal/infra/storage/migrations/pgsql/20260811000000.sql`:

```sql
-- Envelope children were never validated on write, so income-category links
-- could be stored (invisible: reads filter them). Envelope sides become real
-- with the plan view, and every pre-existing envelope is expense-sided, so the
-- never-visible income links are removed. Nothing observable changes.
DELETE FROM budgets_envelopes_categories
WHERE category_id IN (SELECT id FROM categories WHERE type = 1);
```

(If the pgsql link table name differs, mirror whatever `internal/infra/storage/migrations/pgsql/20241103192302.sql` calls it.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/infra/storage/migrations/ -run TestMigration20260811 -v`
Expected: PASS.
Then the full smoke tier to prove boot-time migration application still works: `go test ./internal/... -count=1 2>&1 | tail -5` (apiparity's known Task 9 failure aside).

- [ ] **Step 5: Commit**

```bash
git add internal/infra/storage/migrations/sqlite/20260811000000.sql internal/infra/storage/migrations/pgsql/20260811000000.sql internal/infra/storage/migrations/migration_20260811_test.go
git commit -m "feat(budget): migration removes never-visible income children from envelopes"
```

---

### Task 9: Parity catalogues, goldens, OpenAPI docs, full gate

Two intentional, inspected golden events — everything else stays byte-identical:

1. `budget_structure_writes`'s `update-envelope` call currently sends `categories: [CatSalary]` (an income category, silently ignored until now). The call switches to `[]` — the **response bytes are provably identical** (the income child was already filtered out of the rendered children), so this golden must NOT diff after regeneration; if it does, stop and investigate.
2. New scenarios/goldens are added (income elements + homogeneity/folder backstops), and mcpparity's `tools/list` golden gains the `side` param.

**Files:**
- Modify: `internal/test/apiparity/catalogue_budget_structure.go` (line 30–32)
- Create: `internal/test/apiparity/catalogue_budget_income.go`
- Modify: golden files under `internal/test/apiparity/testdata/golden/` (regenerated, inspected)
- Modify: golden files under `internal/test/mcpparity/testdata/` (regenerated, inspected)
- Modify: `docs/` OpenAPI artifacts via `make swagger` (CreateEnvelopeRequest gained `side`)

**Interfaces:**
- Consumes: everything from Tasks 1–8.
- Produces: the scenario-count guard grows by one scenario; the frozen-contract proof (`get-budget` goldens unchanged) is on record.

- [ ] **Step 1: Fix the update-envelope scenario body**

In `internal/test/apiparity/catalogue_budget_structure.go`, change the `update-envelope` call:

```go
			{Label: "update-envelope", Method: "POST", Path: "/api/v1/budget/update-envelope", Auth: "owner",
				Body: map[string]any{"budgetId": Budget, "id": Envelope1, "name": "Envelope 2", "icon": "cart",
					// CatSalary (income) was silently ignored here for years; envelope
					// homogeneity now rejects it. An empty set produces byte-identical
					// response bytes (the income child never rendered).
					"currencyId": USD, "isArchived": 0, "categories": []string{}}},
```

- [ ] **Step 2: Add the income scenario**

Create `internal/test/apiparity/catalogue_budget_income.go`:

```go
package apiparity

// Income-element scenario: seeding, the income envelope lifecycle, both coded
// side backstops, planned income via set-limit — and the frozen-contract proof
// that get-budget shows none of it. The fixture's CatSalary (income) is seeded
// as a type=3 element by create-budget.

func init() {
	register(Scenario{Name: "budget_income_elements", Calls: func() []Call {
		const (
			incomeBudget   = "b0000000-0000-0000-0000-0000000000ee"
			expenseFolder  = "bf000000-0000-0000-0000-0000000000ee"
			incomeEnvelope = "be000000-0000-0000-0000-0000000000ee"
		)
		return []Call{
			{Label: "create-budget", Method: "POST", Path: "/api/v1/budget/create-budget", Auth: "owner",
				Body: map[string]any{"id": incomeBudget, "name": "Income Plan", "currencyId": USD, "startDate": "2024-04-01"}},
			{Label: "get-budget-baseline", Method: "GET", Path: "/api/v1/budget/get-budget?id=" + incomeBudget + "&date=2024-04-15", Auth: "owner"},
			{Label: "create-folder", Method: "POST", Path: "/api/v1/budget/create-folder", Auth: "owner",
				Body: map[string]any{"budgetId": incomeBudget, "id": expenseFolder, "name": "Living"}},
			// CatFood makes the folder expense-sided.
			{Label: "seed-expense-folder", Method: "POST", Path: "/api/v1/budget/move-element", Auth: "owner",
				Body: map[string]any{"budgetId": incomeBudget, "id": CatFood, "folderId": expenseFolder, "afterId": nil}},
			// The behavior change on record: an income child in a default (expense)
			// envelope is now rejected instead of silently stored.
			{Label: "err:expense-envelope-income-child", Method: "POST", Path: "/api/v1/budget/create-envelope", Auth: "owner",
				Body: map[string]any{"budgetId": incomeBudget, "id": incomeEnvelope, "name": "Wrong Side", "icon": "cart",
					"currencyId": USD, "folderId": nil, "categories": []string{CatSalary}}},
			{Label: "create-income-envelope", Method: "POST", Path: "/api/v1/budget/create-envelope", Auth: "owner",
				Body: map[string]any{"budgetId": incomeBudget, "id": incomeEnvelope, "name": "Salaries", "icon": "payments",
					"currencyId": USD, "folderId": nil, "side": "income", "categories": []string{CatSalary}}},
			{Label: "err:income-envelope-expense-child", Method: "POST", Path: "/api/v1/budget/update-envelope", Auth: "owner",
				Body: map[string]any{"budgetId": incomeBudget, "id": incomeEnvelope, "name": "Salaries", "icon": "payments",
					"currencyId": USD, "isArchived": 0, "categories": []string{CatFood}}},
			{Label: "err:income-into-expense-folder", Method: "POST", Path: "/api/v1/budget/move-element", Auth: "owner",
				Body: map[string]any{"budgetId": incomeBudget, "id": incomeEnvelope, "folderId": expenseFolder, "afterId": nil}},
			// Planned income is a plain limit on the income envelope.
			{Label: "set-income-limit", Method: "POST", Path: "/api/v1/budget/set-limit", Auth: "owner",
				Body: map[string]any{"budgetId": incomeBudget, "elementId": incomeEnvelope, "period": "2024-05-01", "amount": "1500"}},
			// Frozen-contract proof: none of the income structure or its limit is
			// visible in the budget view.
			{Label: "get-budget-after", Method: "GET", Path: "/api/v1/budget/get-budget?id=" + incomeBudget + "&date=2024-05-15", Auth: "owner"},
		}
	}})
}
```

(Mirror the exact `register`/`Scenario`/`Call` field names from `catalogue_budget_structure.go`; if error-labeled calls need a specific label convention for the guard, follow whatever `catalogue_negative.go` uses — the `err:` prefix above copies `catalogue_ratelimit.go`.)

- [ ] **Step 3: Regenerate and INSPECT the goldens**

```bash
UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ -count=1
UPDATE_GOLDEN=1 go test ./internal/test/mcpparity/ -count=1
git status --short
git diff internal/test/apiparity/testdata/ internal/test/mcpparity/testdata/ | head -200
```

Inspection checklist — every point must hold before staging:
- **Zero diffs** in any pre-existing apiparity golden, including `budget_structure_writes` (the `[]`-categories edit is byte-neutral) and every `get-budget` golden.
- New goldens exist only for `budget_income_elements`; its `err:` calls carry the two frozen messages and `"code"` values matching `budget.envelope_side_mixed` / `budget.folder_side_mixed` semantics (the envelope carries the message + `errors` map; codes are int on the wire — expect the standard 400 envelope shape).
- `get-budget-baseline` and `get-budget-after` differ **only** by the folder/CatFood placement and the requested date — no income envelope id, no `CatSalary`, no `1500` anywhere in either.
- mcpparity: the only diff is the added `side` property in `create_envelope`'s input schema.

- [ ] **Step 4: Regenerate OpenAPI docs and run the full gate**

```bash
make swagger
make go-test
```

Expected: PASS, including the docs-freshness check, the apiparity guard (scenario count grew, nothing orphaned), i18ntest, and the coverage gate.

- [ ] **Step 5: Commit**

```bash
git add internal/test/apiparity/ internal/test/mcpparity/ docs/
git commit -m "test(budget): income-element parity scenario; envelope side in MCP schema + OpenAPI"
```

- [ ] **Step 6: PostgreSQL parity (if a local PG is available, else CI)**

Run: `make test-repo-pgsql` (or rely on the CI regression job).
Expected: PASS — the reconciler/seeding/migration changes contain no engine-specific SQL except the new migration, which is identical text on both engines.

---

## Ordering & dependencies

Tasks must run in order 1 → 9. Task 3 (reconciler) MUST land before Task 4 (seeding) and Task 6 (envelope side): seeded or side-typed rows written before the reconciler learns the types would be destroyed or trip the UNIQUE constraint on the next write — the exact failure modes documented in the spec's "Reconciliation is the primary implementation site" section.

## Out of scope for this plan

- The `get-budget-plan` endpoint, its three grouped-by-month queries, opening balances, per-month rates, DTOs, REST + MCP edges, and their parity suites — plan 02.
- All frontend work (mode toggle, sheet, keyboard, totals, density, analytics, i18n UI strings) — plan 03. The i18n **error** strings for the two new codes are in this plan (Task 2) because the i18ntest guard would fail without them.
- Lazily-backfilled income elements order by element id (the `assignMissingKeys` tail-append), not category order — acceptable for v1; freshly created budgets seed in category order.
