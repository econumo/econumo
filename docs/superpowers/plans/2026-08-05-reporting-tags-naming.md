# Reporting Tags: Naming, Dialog, Uniqueness, Budget Folder — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the user-facing word "label" with a single noun ("tag", qualified as budget/reporting where the kinds must be distinguished), teach the distinction with a purpose-phrased radio at creation, enforce one case-insensitive name namespace across both kinds, and reshape the budget's flat labels block into an ephemeral collapsed "Reporting tags" folder whose tags expand to a category breakdown.

**Architecture:** Backend entity naming (`labels` table, `internal/label/`, `labelIds` on the wire) is deliberately untouched — this is a copy, dialog, validation, and rendering change layered on the existing branch. Cross-kind uniqueness is a cross-feature lookup, so it follows the codebase's dependency rule: each feature declares a consumer-side port, and `internal/server` wires a `glue_` adapter over the other feature's service. The budget folder adds one grouping level (`category_id`) to the existing `CountSpendingByLabel` query and a second assembly level in the builder, reusing the two-level rollup the budget-tag path already performs.

**Tech Stack:** Go 1.x (stdlib `net/http`, sqlc, SQLite + PostgreSQL), React 19 + Vite + TypeScript, TanStack Query, Zustand, Tailwind 4 / shadcn, vitest, react-i18next.

**Spec:** [2026-08-05-reporting-tags-naming-design.md](../specs/2026-08-05-reporting-tags-naming-design.md)

## Global Constraints

- **Backend naming is frozen.** Do NOT rename the `labels` table, `internal/label/`, `LabelSpendResult`, `labelIds`, `labelsSeparator`, or any MCP/analytics key. Only user-facing strings change.
- **Copy rule:** the word "label"/"Label" must not appear in any user-facing catalogue *value* in any of the 11 languages. Catalogue *keys* under `classifications.labels.*` stay.
- **Qualifiers, not a second noun:** "Budget tags" / "Reporting tags" (en); «Теги для бюджета» / «Теги для отчётов» (ru). Never «метки».
- **11 languages, always together:** `locales/{de,en,es,fr,it,nl,pl,pt,ru,uk,zh}.json` — `en` is the reference catalogue among them; every key added or removed must land in all of them or `i18ntest` fails.
- **Golden files are never hand-edited.** Regenerate with `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/` (or `./internal/test/mcpparity/`) and inspect the diff.
- **Wire contract additions only.** `LabelSpendResult.children` is additive; no existing JSON field changes name or type.
- **Test floor:** `make go-test` enforces `GO_COVER_MIN` (default 80). Do not lower it.
- **Every user-facing action fires analytics** per CLAUDE.md — but this plan adds no new user actions, so no new `METRICS` keys. Do not add any (`metrics-coverage.test.ts` fails on unfired keys).
- **Comments:** only for non-obvious business logic or frozen-contract rationale. No comments restating a symbol's name. Never reference the removed PHP implementation.
- **Commit style:** `type(scope): summary`, ending with the `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>` trailer.

## File Structure

**Backend — cross-kind uniqueness (Tasks 1–3)**
- `internal/tag/ports.go` — gains `LabelNames` port (consumer-side interface).
- `internal/tag/usecase.go:153-167` — `ensureNameUnique` goes case-insensitive + cross-kind.
- `internal/label/ports.go` — gains `TagNames` port.
- `internal/label/usecase.go:184-198` — same change, mirrored.
- `internal/server/glue_classification_names.go` — **new**; both adapters, wired in `BuildAPI`.
- `internal/shared/errs/codes.go` — `CodeLabelAlreadyExists` removed.

**Backend — budget folder (Tasks 4–6)**
- `internal/model/budget_view.go` — `LabelSpendingRow` gains `CategoryID *string`.
- `internal/model/budget_dto.go:95-106` — `LabelSpendResult` gains `Children`.
- `internal/budget/repo/read.go:415+` — `CountSpendingByLabel` groups by category; two new drill-down methods.
- `internal/budget/builder_structure_build.go:307-325` — two-level rollup.
- `internal/budget/txlist.go:39-45,77+` — relaxed guard + two new switch cases.

**Frontend (Tasks 7–10)**
- `web/src/lib/classificationKind.ts` — single accent colour.
- `web/src/features/classifications/TagDialog.tsx` — purpose radio.
- `web/src/features/classifications/TagsPage.tsx:61-72` — qualified section captions.
- `web/src/features/budgets/BudgetTable.tsx:264-344` — `LabelsSection` → `ReportingTagsFolder`.
- `web/src/api/dto/budget.ts` — `LabelSpendDto.children`.

**Catalogues (Task 11)**
- `locales/*.json` × 11.

---

### Task 1: Case-insensitive same-kind uniqueness

Move both features' existing `ensureNameUnique` from exact to case-insensitive comparison. This is a standalone behavior change and lands first so the cross-kind work in Task 2 builds on a settled comparison.

**Files:**
- Modify: `internal/tag/usecase.go:153-167`
- Modify: `internal/label/usecase.go:184-198`
- Test: `internal/tag/create_test.go`, `internal/label/create_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `ensureNameUnique(ctx context.Context, userID vo.Id, name string, exceptID vo.Id) error` on both `*tag.Service` and `*label.Service` — unchanged signature, case-insensitive semantics. Task 2 extends these same methods.

- [ ] **Step 1: Write the failing tests**

Append to `internal/tag/create_test.go`:

```go
func TestCreateTagRejectsCaseVariantName(t *testing.T) {
	env := newTagEnv(t)
	ctx := context.Background()

	_, err := env.svc.CreateTag(ctx, env.userID, model.CreateTagRequest{
		Id: vo.NewId().String(), Name: "Trip",
	})
	if err != nil {
		t.Fatalf("first create: %v", err)
	}

	_, err = env.svc.CreateTag(ctx, env.userID, model.CreateTagRequest{
		Id: vo.NewId().String(), Name: "trip",
	})
	if err == nil {
		t.Fatal("want case-variant name rejected, got nil error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.MsgCode != errs.CodeTagAlreadyExists {
		t.Fatalf("want CodeTagAlreadyExists, got %#v", err)
	}
}
```

Append the mirror to `internal/label/create_test.go`, using `env.svc.CreateLabel`, `model.CreateLabelRequest`, and `errs.CodeLabelAlreadyExists` (still present at this point — Task 3 removes it).

> **`newTagEnv` / `newLabelEnv` do not exist** — they stand in for whatever fixture the neighbouring tests in each file already use to build a service plus a user (check the top of `internal/tag/create_test.go`). Reuse that; do not invent a new fixture. The same substitution applies everywhere these names appear in this plan.

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/tag/ ./internal/label/ -run 'CaseVariantName' -v
```

Expected: FAIL — both creates succeed, so the test hits `t.Fatal("want case-variant name rejected, got nil error")`.

- [ ] **Step 3: Make the comparison case-insensitive**

In `internal/tag/usecase.go`, replace the body of `ensureNameUnique`:

```go
func (s *Service) ensureNameUnique(ctx context.Context, userID vo.Id, name string, exceptID vo.Id) error {
	tags, err := s.repo.ListByOwner(ctx, userID)
	if err != nil {
		return err
	}
	for _, t := range tags {
		// Case-insensitive: one name means one tag regardless of case, matching
		// how CSV import already resolves names.
		if strings.EqualFold(t.Name, name) && !t.ID.Equal(exceptID) {
			return &errs.ValidationError{Msg: "Tag already exists.", MsgCode: errs.CodeTagAlreadyExists}
		}
	}
	return nil
}
```

Add `"strings"` to that file's import block if absent. Apply the identical change in `internal/label/usecase.go`, keeping its own error (`"Label already exists."`, `errs.CodeLabelAlreadyExists`) for now.

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/tag/ ./internal/label/ -v
```

Expected: PASS, including the pre-existing tests in both packages.

- [ ] **Step 5: Commit**

```bash
git add internal/tag/usecase.go internal/tag/create_test.go internal/label/usecase.go internal/label/create_test.go
git commit -m "$(cat <<'EOF'
feat(tag): compare names case-insensitively

One name means one tag regardless of case, matching how CSV import
already resolves names.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Cross-kind uniqueness via ports and glue

A budget tag and a reporting tag may no longer share a name. Each feature declares a consumer-side port for the other's names; `internal/server` wires both. Features never import each other (`archtest` enforces this).

**Files:**
- Modify: `internal/tag/ports.go`, `internal/tag/usecase.go`, `internal/tag/service.go` (constructor)
- Modify: `internal/label/ports.go`, `internal/label/usecase.go`, `internal/label/service.go` (constructor)
- Create: `internal/server/glue_classification_names.go`
- Modify: `internal/server/server.go` (the `BuildAPI` wiring)
- Test: `internal/tag/create_test.go`, `internal/label/create_test.go`, `internal/server/glue_classification_names_test.go`

**Interfaces:**
- Consumes: `ensureNameUnique` from Task 1.
- Produces:
  - `type LabelNames interface { NamesByOwner(ctx context.Context, ownerID vo.Id) ([]string, error) }` in package `tag`.
  - `type TagNames interface { NamesByOwner(ctx context.Context, ownerID vo.Id) ([]string, error) }` in package `label`.
  - Both `NewService` constructors take the port as a new **final** parameter.
  - `server.labelNamesAdapter` / `server.tagNamesAdapter` implement them.

- [ ] **Step 1: Write the failing tests**

Append to `internal/tag/create_test.go`:

```go
type stubNames struct{ names []string }

func (s stubNames) NamesByOwner(context.Context, vo.Id) ([]string, error) { return s.names, nil }

func TestCreateTagRejectsNameTakenByLabel(t *testing.T) {
	env := newTagEnv(t)
	env.svc.SetLabelNames(stubNames{names: []string{"Trip"}})
	ctx := context.Background()

	_, err := env.svc.CreateTag(ctx, env.userID, model.CreateTagRequest{
		Id: vo.NewId().String(), Name: "trip",
	})
	if err == nil {
		t.Fatal("want name-taken-by-label rejected, got nil error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.MsgCode != errs.CodeTagAlreadyExists {
		t.Fatalf("want CodeTagAlreadyExists, got %#v", err)
	}
}
```

Append the mirror to `internal/label/create_test.go` (`SetTagNames`, `CreateLabel`, expecting `errs.CodeTagAlreadyExists` — Task 3 collapses both kinds onto the tag code, and this test is written for that end state; it will be adjusted in Task 3 if it lands red here, so if `CodeLabelAlreadyExists` is still what the label path returns, assert that for now and Task 3 flips it).

> `SetLabelNames`/`SetTagNames` are test-only setters added in Step 3 so existing fixtures need no constructor churn. If the package already has a fixture that builds the service via `NewService`, prefer passing the stub there and drop the setter.

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/tag/ ./internal/label/ -run 'NameTakenBy' -v
```

Expected: FAIL to compile — `env.svc.SetLabelNames` undefined.

- [ ] **Step 3: Declare the ports and use them**

Append to `internal/tag/ports.go`:

```go
// LabelNames exposes the reporting-tag names of one owner. Both kinds share a
// single user-facing namespace ("tag"), so a name taken by either blocks the
// other; the tag feature cannot import the label feature to check.
type LabelNames interface {
	NamesByOwner(ctx context.Context, ownerID vo.Id) ([]string, error)
}
```

Append the mirror to `internal/label/ports.go`:

```go
// TagNames exposes the budgeting-tag names of one owner. See tag.LabelNames.
type TagNames interface {
	NamesByOwner(ctx context.Context, ownerID vo.Id) ([]string, error)
}
```

In `internal/tag/service.go`, add the field to `Service`, accept it as the final `NewService` parameter, and add the test-only setter:

```go
func (s *Service) SetLabelNames(n LabelNames) { s.labelNames = n }
```

Extend `internal/tag/usecase.go`'s `ensureNameUnique` to consult it:

```go
func (s *Service) ensureNameUnique(ctx context.Context, userID vo.Id, name string, exceptID vo.Id) error {
	tags, err := s.repo.ListByOwner(ctx, userID)
	if err != nil {
		return err
	}
	for _, t := range tags {
		if strings.EqualFold(t.Name, name) && !t.ID.Equal(exceptID) {
			return &errs.ValidationError{Msg: "Tag already exists.", MsgCode: errs.CodeTagAlreadyExists}
		}
	}
	// The other kind shares the namespace. exceptID never matches here: ids are
	// unique across both tables, so a rename can never collide with itself.
	if s.labelNames == nil {
		return nil
	}
	names, err := s.labelNames.NamesByOwner(ctx, userID)
	if err != nil {
		return err
	}
	for _, n := range names {
		if strings.EqualFold(n, name) {
			return &errs.ValidationError{Msg: "Tag already exists.", MsgCode: errs.CodeTagAlreadyExists}
		}
	}
	return nil
}
```

Mirror all of it in `internal/label/service.go` + `internal/label/usecase.go` (`tagNames`, `SetTagNames`), keeping the label error code for now.

- [ ] **Step 4: Write the glue adapters**

Create `internal/server/glue_classification_names.go`:

```go
package server

import (
	"context"

	"github.com/econumo/econumo/internal/label"
	"github.com/econumo/econumo/internal/tag"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Both classification kinds share one user-facing namespace, so each feature's
// uniqueness check must see the other's names. Neither may import the other.
type labelNamesAdapter struct{ svc *label.Service }

func (a labelNamesAdapter) NamesByOwner(ctx context.Context, ownerID vo.Id) ([]string, error) {
	return a.svc.NamesByOwner(ctx, ownerID)
}

type tagNamesAdapter struct{ svc *tag.Service }

func (a tagNamesAdapter) NamesByOwner(ctx context.Context, ownerID vo.Id) ([]string, error) {
	return a.svc.NamesByOwner(ctx, ownerID)
}
```

Add the exported `NamesByOwner` to both services (each reads its own repo — `ListByOwner`, map to names). Wire in `BuildAPI` after both services exist:

```go
tagSvc.SetLabelNames(labelNamesAdapter{svc: labelSvc})
labelSvc.SetTagNames(tagNamesAdapter{svc: tagSvc})
```

> The two services are mutually dependent, so they cannot both take the port in their constructor. Setter wiring at composition time is the simplest break in the cycle; keep it in `BuildAPI` only.

- [ ] **Step 5: Write the glue test**

Create `internal/server/glue_classification_names_test.go` asserting that a tag created through the real `BuildAPI` handler blocks a same-name label create and vice versa, using the existing server-test fixture style in `glue_budget_test.go`.

- [ ] **Step 6: Run the tests**

```bash
go test ./internal/tag/ ./internal/label/ ./internal/server/ ./internal/test/archtest/ -v
```

Expected: PASS. `archtest` must stay green — it proves no feature imports another.

- [ ] **Step 7: Commit**

```bash
git add internal/tag internal/label internal/server
git commit -m "$(cat <<'EOF'
feat(tag): one name namespace across both classification kinds

Budget and reporting tags are one concept to the user, so a name taken
by either blocks the other. Features cannot import each other, so each
declares a names port wired by server glue.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Collapse the duplicate-name error onto one code

With one namespace there is one error. `CodeLabelAlreadyExists` and its 11 catalogue entries go.

**Files:**
- Modify: `internal/shared/errs/codes.go`, `internal/label/usecase.go`
- Modify: `locales/*.json` × 11 (remove `errors.label.already_exists`)
- Test: `internal/label/create_test.go`, `internal/test/i18ntest/`

**Interfaces:**
- Consumes: Task 2's cross-kind check.
- Produces: the label path now returns `errs.CodeTagAlreadyExists` with message `"Tag already exists."`.

- [ ] **Step 1: Point the label path at the tag code**

In `internal/label/usecase.go`, both rejection sites become:

```go
return &errs.ValidationError{Msg: "Tag already exists.", MsgCode: errs.CodeTagAlreadyExists}
```

- [ ] **Step 2: Remove the dead code and catalogue entries**

Delete `CodeLabelAlreadyExists` from `internal/shared/errs/codes.go` (both the constant and its `AllCodes` entry). Remove `errors.label.already_exists` from all 11 `locales/*.json`.

- [ ] **Step 3: Update the label tests**

In `internal/label/create_test.go`, every assertion expecting `CodeLabelAlreadyExists` becomes `CodeTagAlreadyExists`.

- [ ] **Step 4: Run the guards**

```bash
go test ./internal/label/ ./internal/test/i18ntest/ ./internal/shared/errs/ -v
```

Expected: PASS. `i18ntest`'s two-way `errs.AllCodes` ↔ `errors.*` coverage proves the code and all 11 catalogue entries were removed together.

- [ ] **Step 5: Regenerate affected goldens and inspect**

```bash
UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ && git diff --stat internal/test/apiparity/testdata/golden/
```

Expected: only duplicate-name scenarios change, message `"Label already exists."` → `"Tag already exists."`. **Read the diff.** Any other change means something unintended moved.

- [ ] **Step 6: Commit**

```bash
git add internal/shared/errs internal/label locales internal/test/apiparity/testdata
git commit -m "$(cat <<'EOF'
refactor(errs): one duplicate-name error for both kinds

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Group label spend by category

`CountSpendingByLabel` gains `category_id` to its grouping — the data the folder's second level needs.

**Files:**
- Modify: `internal/model/budget_view.go` (`LabelSpendingRow`)
- Modify: `internal/budget/repo/read.go:415-480`
- Test: `internal/budget/repo/read_labels_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `LabelSpendingRow{ LabelID, CurrencyID, Amount string; CategoryID *string }` — nil `CategoryID` means the transaction had no category. `CountSpendingByLabel(ctx, accountIDs []vo.Id, start, end time.Time) ([]model.LabelSpendingRow, error)` — signature unchanged, now one row per `(label, category, currency)`.

- [ ] **Step 1: Write the failing test**

Append to `internal/budget/repo/read_labels_test.go` a test that seeds one label on two transactions in different categories plus one with no category, then asserts three rows come back with the expected `CategoryID` values (two non-nil, one nil) and that the amounts are per-category, not summed.

Follow the seeding style already in that file; use `dbtest.New(t)` and `db.Rebind(query)` for any raw SQL.

- [ ] **Step 2: Run to verify it fails**

```bash
go test ./internal/budget/repo/ -run 'ByLabel' -v
```

Expected: FAIL — one aggregated row per label instead of one per category.

- [ ] **Step 3: Add the grouping**

In `internal/model/budget_view.go`, add `CategoryID *string` to `LabelSpendingRow`. In `internal/budget/repo/read.go`, extend the `sel` constant to select and group by `t.category_id`:

```go
const sel = "SELECT SUM(t.amount) as amount, tl.label_id, t.category_id, a.currency_id " +
	"FROM transactions t " +
	"JOIN transactions_labels tl ON tl.transaction_id = t.id " +
	"LEFT JOIN accounts a ON t.account_id = a.id AND a.id IN (%s) " +
	"WHERE t.type = 0 AND t.spent_at >= %s AND t.spent_at < %s " +
	"GROUP BY tl.label_id, t.category_id, a.currency_id ORDER BY tl.label_id, t.category_id, a.currency_id"
```

Add `categoryID *string` to both engine branches' `rows.Scan` (between `labelID` and `currencyID`, matching the SELECT order) and set it on the emitted row. Keep the existing float-vs-NUMERIC amount handling exactly as it is.

- [ ] **Step 4: Run both engines**

```bash
go test ./internal/budget/repo/ -run 'ByLabel' -v
DBTEST_ENGINE=pgsql go test -tags enginecompare ./internal/budget/repo/ -run 'ByLabel' -v
```

Expected: PASS on both. (The pgsql run needs `DATABASE_TEST_PGSQL_URL` or the compose stack; if unavailable, note it and let Task 6's full run cover it.)

- [ ] **Step 5: Commit**

```bash
git add internal/model/budget_view.go internal/budget/repo
git commit -m "$(cat <<'EOF'
feat(budget): group reporting-tag spend by category

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: Two-level rollup in the builder

Turn the per-category rows into per-tag totals with category children.

**Files:**
- Modify: `internal/model/budget_dto.go:95-106`
- Modify: `internal/budget/builder_structure_build.go:160-200,307-325`
- Test: `internal/budget/builder_labels_test.go` (new or existing label builder test file)

**Interfaces:**
- Consumes: Task 4's `LabelSpendingRow.CategoryID`.
- Produces: `LabelSpendResult` gains `Children []ChildElementResult \`json:"children"\`` — always non-nil (`[]` when empty), each child carrying `Id, Type, Name, Icon, IsArchived, Spent, BudgetSpent, OwnerUserId`. Category children with zero spend are omitted; a nil `CategoryID` becomes one child using the shared uncategorized id.

- [ ] **Step 1: Write the failing test**

A builder test asserting: a reporting tag with spend in two categories emits two children whose amounts sum to the parent's total; a transaction with no category emits an uncategorized child; a tag with zero spend is absent entirely; and children are omitted when their own spend is zero.

- [ ] **Step 2: Run to verify it fails**

```bash
go test ./internal/budget/ -run 'Label' -v
```

Expected: FAIL — `LabelSpendResult` has no `Children` field.

- [ ] **Step 3: Add the field and the rollup**

Add to `internal/model/budget_dto.go`:

```go
	Children    []ChildElementResult `json:"children"`
```

In `builder_structure_build.go`, extend the label accumulation (around line 175) to key spend by `(labelID, categoryID)` as well as by label, registering each category's amount for bulk conversion under a distinct key — mirroring the `%s_spent_%s` parent/child key convention the element path uses. Then in the emit loop (line 307):

```go
	labels := []model.LabelSpendResult{}
	for labelID, meta := range f.labels {
		spent := get(fmt.Sprintf("label-spent_%s", labelID))
		if spent.IsZero() {
			continue
		}
		// A tag's children sum to its own total; tags still overlap with EACH
		// OTHER, which is why the block carries the overlap note.
		children := []model.ChildElementResult{}
		for categoryID, sub := range f.labelCategories[labelID] {
			subSpent := get(fmt.Sprintf("label-spent_%s_%s", labelID, categoryID))
			if subSpent.IsZero() {
				continue
			}
			children = append(children, model.ChildElementResult{
				Id: categoryID, Type: int(model.ElementCategory.Int16()),
				Name: sub.Name, Icon: sub.Icon, IsArchived: boolToInt(sub.IsArchived),
				Spent: subSpent.String(), BudgetSpent: subSpent.String(),
				OwnerUserId: sub.OwnerID,
			})
		}
		sort.Slice(children, func(i, j int) bool { return children[i].Id < children[j].Id })
		labels = append(labels, model.LabelSpendResult{
			Id: labelID, Name: meta.Name, Icon: meta.Icon,
			IsArchived: boolToInt(meta.IsArchived), Spent: spent.String(),
			OwnerUserId: meta.OwnerID, Children: children,
		})
	}
```

Populate `f.labelCategories` (a `map[string]map[string]categoryMeta`) alongside `f.labels` in the filter build, resolving category display metadata the same way the element path does, and using the existing uncategorized id constant for the nil-category bucket.

- [ ] **Step 4: Run the tests**

```bash
go test ./internal/budget/ -v
```

Expected: PASS.

- [ ] **Step 5: Regenerate budget goldens and inspect**

```bash
UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ && git diff internal/test/apiparity/testdata/golden/ | head -60
```

Expected: label entries gain `"children": [...]`. **Read the diff** — no element totals may move.

- [ ] **Step 6: Commit**

```bash
git add internal/model/budget_dto.go internal/budget internal/test/apiparity/testdata
git commit -m "$(cat <<'EOF'
feat(budget): break reporting-tag spend down by category

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Relax the txlist guard for label+category drill-down

`labelId` is currently mutually exclusive with every other selector. Permit exactly `labelId + categoryId` and `labelId + uncategorized`; keep rejecting `labelId + tagId` and `labelId + envelopeId`.

**Files:**
- Modify: `internal/budget/txlist.go:39-45` (guard), `:77+` (switch)
- Modify: `internal/budget/repo/read.go` (two new methods)
- Modify: `internal/budget/repository.go` (ReadModel interface)
- Test: `internal/budget/txlist_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces:
  - `BudgetTransactionsByLabelAndCategory(ctx context.Context, labelID, categoryID vo.Id, accountIDs []vo.Id, start, end time.Time) ([]model.BudgetTransactionRow, error)`
  - `BudgetTransactionsByLabelUncategorized(ctx context.Context, labelID vo.Id, accountIDs []vo.Id, start, end time.Time) ([]model.BudgetTransactionRow, error)`

- [ ] **Step 1: Write the failing tests**

Four cases in `internal/budget/txlist_test.go`: `labelId + categoryId` returns only that label's transactions in that category; `labelId + uncategorized` returns only its category-less ones; `labelId + tagId` still fails with `CodeBudgetTransactionFilterRequired`; `labelId + envelopeId` likewise.

- [ ] **Step 2: Run to verify they fail**

```bash
go test ./internal/budget/ -run 'TxList.*Label' -v
```

Expected: FAIL — the first two are rejected by the guard.

- [ ] **Step 3: Narrow the guard**

Replace the mutual-exclusion block at `txlist.go:39-45`:

```go
	// A reporting tag narrows by category (the folder's child rows drill down to
	// exactly that pair), but pairing it with another SELECTOR — a budgeting tag
	// or an envelope — is ambiguous about which one narrows.
	if req.LabelId != nil && strings.TrimSpace(*req.LabelId) != "" &&
		((req.TagId != nil && strings.TrimSpace(*req.TagId) != "") ||
			(req.EnvelopeId != nil && strings.TrimSpace(*req.EnvelopeId) != "")) {
		return nil, &errs.ValidationError{Msg: "Validation failed", MsgCode: errs.CodeBudgetTransactionFilterRequired}
	}
```

The `uncategorized + categoryId` guard above it stays untouched.

- [ ] **Step 4: Add the switch cases**

The existing `case lbl != "":` must be **preceded** by the two narrower cases, since Go's `switch` takes the first match:

```go
	case lbl != "" && cat != "":
		labelID, perr := vo.ParseId(lbl)
		if perr != nil {
			return nil, model.ValidateBlank(map[string]string{"labelId": ""})
		}
		categoryID, perr := vo.ParseId(cat)
		if perr != nil {
			return nil, model.ValidateBlank(map[string]string{"categoryId": ""})
		}
		rows, err = s.read.BudgetTransactionsByLabelAndCategory(ctx, labelID, categoryID, f.includedAccountIDs, periodStart, periodEnd)
	case lbl != "" && req.Uncategorized:
		labelID, perr := vo.ParseId(lbl)
		if perr != nil {
			return nil, model.ValidateBlank(map[string]string{"labelId": ""})
		}
		rows, err = s.read.BudgetTransactionsByLabelUncategorized(ctx, labelID, f.includedAccountIDs, periodStart, periodEnd)
```

- [ ] **Step 5: Add the repo methods**

In `internal/budget/repo/read.go`, mirror `BudgetTransactionsByLabel` twice, adding `AND t.category_id = ?` and `AND t.category_id IS NULL` respectively. Keep the per-engine placeholder and datetime handling identical to the method you are mirroring. Declare both on the `ReadModel` interface in `internal/budget/repository.go`.

- [ ] **Step 6: Run the tests**

```bash
go test ./internal/budget/... -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/budget
git commit -m "$(cat <<'EOF'
feat(budget): drill down into a reporting tag's category

Pairing a reporting tag with a category is unambiguous, unlike pairing
it with another selector, which stays rejected.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: Single accent colour

**Files:**
- Modify: `web/src/lib/classificationKind.ts:12-16`
- Test: `web/src/lib/classificationKind.test.ts`

**Interfaces:**
- Consumes: nothing.
- Produces: `kindAccentClass(kind: ClassificationKind): string` — same signature, same value for both kinds.

- [ ] **Step 1: Update the test**

Replace the per-kind colour assertions in `web/src/lib/classificationKind.test.ts` with:

```ts
it('uses one accent colour for both kinds', () => {
  expect(kindAccentClass('tag')).toBe(kindAccentClass('label'))
})
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd web && pnpm test classificationKind
```

Expected: FAIL — sky vs violet.

- [ ] **Step 3: Collapse the colour**

```ts
/** Kept as a function (not a constant) so a future icon/colour picker can
 *  reintroduce per-row variation without re-threading every call site. */
export function kindAccentClass(_kind: ClassificationKind): string {
  return 'text-sky-600 dark:text-sky-400'
}
```

- [ ] **Step 4: Run the suite**

```bash
cd web && pnpm test classificationKind && pnpm lint
```

Expected: PASS. If oxlint flags the unused parameter, keep the leading underscore.

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/classificationKind.ts web/src/lib/classificationKind.test.ts
git commit -m "$(cat <<'EOF'
feat(web): one accent colour for both classification kinds

The two icons already distinguish the kinds; a single colour reads
calmer in a dense budget table.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 8: Purpose-phrased create dialog

Replace the compact segmented control with two stacked radio options that explain the purposes, and disable the choice on edit.

**Files:**
- Modify: `web/src/features/classifications/TagDialog.tsx:100-115`
- Test: `web/src/features/classifications/TagDialog.test.tsx`

**Interfaces:**
- Consumes: Task 7's `kindAccentClass`.
- Produces: no exported API change. New catalogue keys consumed here are defined in Task 11: `classifications.tags.forms.tag.kind.tag_option`, `.tag_hint`, `.label_option`, `.label_hint`, `.locked_note`.

- [ ] **Step 1: Write the failing tests**

In `web/src/features/classifications/TagDialog.test.tsx`:

```tsx
it('offers both purposes with their explanations when creating', () => {
  renderDialog({ item: null })
  expect(screen.getByRole('radio', { name: /Budget money for this tag/ })).toBeInTheDocument()
  expect(screen.getByText(/One per transaction/)).toBeInTheDocument()
  expect(screen.getByRole('radio', { name: /Label transactions for reporting/ })).toBeInTheDocument()
  expect(screen.getByText(/Several per transaction/)).toBeInTheDocument()
})

it('locks the purpose when editing', () => {
  renderDialog({ item: { id: 'x', name: 'Trip', kind: 'tag', icon: 'tag' } })
  expect(screen.queryByRole('radiogroup')).not.toBeInTheDocument()
  expect(screen.getByTestId('kind-locked-note')).toBeInTheDocument()
})
```

Use whatever `renderDialog` helper the file already defines.

- [ ] **Step 2: Run to verify they fail**

```bash
cd web && pnpm test TagDialog
```

Expected: FAIL — options currently render as bare "Tag"/"Label", and edit renders no note.

- [ ] **Step 3: Rebuild the control**

Replace the `isNew ? (...) : null` block:

```tsx
        {isNew ? (
          <div className="flex flex-col gap-1.5" role="radiogroup">
            {CLASSIFICATION_KINDS.map((option) => (
              <button
                key={option}
                type="button"
                role="radio"
                aria-checked={kind === option}
                aria-label={t(`classifications.tags.forms.tag.kind.${option}_option`)}
                className={`rounded-md border px-3 py-2 text-left ${kind === option ? 'border-primary bg-accent' : 'hover:bg-accent/50'}`}
                onClick={() => setKind(option)}
              >
                <span className="block text-sm font-medium">
                  {t(`classifications.tags.forms.tag.kind.${option}_option`)}
                </span>
                <span className="block text-xs text-muted-foreground">
                  {t(`classifications.tags.forms.tag.kind.${option}_hint`)}
                </span>
              </button>
            ))}
          </div>
        ) : (
          <p className="text-xs text-muted-foreground" data-testid="kind-locked-note">
            {t('classifications.tags.forms.tag.kind.locked_note')}
          </p>
        )}
```

Wrap the icon + this block in `flex flex-col gap-3` (rather than the current `flex items-center gap-3`) so the stacked options are not squeezed beside the icon; keep `data-testid="kind-icon"` on the icon.

- [ ] **Step 4: Run the suite**

```bash
cd web && pnpm test TagDialog && pnpm lint
```

Expected: PASS. (These tests need Task 11's catalogue keys — if run before Task 11, they fail on missing translations. Either land Task 11 first or accept a red step here; the plan orders Task 11 last because it is mechanical, so **run this step's suite again after Task 11**.)

- [ ] **Step 5: Commit**

```bash
git add web/src/features/classifications/TagDialog.tsx web/src/features/classifications/TagDialog.test.tsx
git commit -m "$(cat <<'EOF'
feat(web): phrase the tag purpose choice by what it does

The kind names taught nothing; the purposes do. Locked on edit, since
there is no conversion path.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 9: Qualified section captions

**Files:**
- Modify: `web/src/features/classifications/TagsPage.tsx:61-72`
- Test: `web/src/features/classifications/TagsPage.test.tsx:85-86`

**Interfaces:**
- Consumes: Task 11's `classifications.tags.pages.settings.section_budget` / `.section_reporting`, and the repurposed `.info` / `.create_tag`.
- Produces: no API change.

- [ ] **Step 1: Update the test**

`TagsPage.test.tsx:85-86` currently asserts `screen.getAllByText('Tags')` has length 2. Replace:

```tsx
  // the page title is 'Tags'; the two sections qualify the purposes
  expect(screen.getAllByText('Tags')).toHaveLength(1)
  expect(screen.getByText('Budget tags')).toBeInTheDocument()
  expect(screen.getByText('Reporting tags')).toBeInTheDocument()
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd web && pnpm test TagsPage
```

Expected: FAIL — two "Tags" nodes, no "Budget tags".

- [ ] **Step 3: Point at the new keys**

In `TagsPage.tsx`:

```tsx
        info={t('classifications.tags.pages.settings.info')}
        createLabel={t('classifications.tags.pages.settings.create_tag')}
```

and:

```tsx
        sections={[
          { label: t('classifications.tags.pages.settings.section_budget'), match: (row) => row.kind === 'tag' },
          { label: t('classifications.tags.pages.settings.section_reporting'), match: (row) => row.kind === 'label' },
        ]}
```

- [ ] **Step 4: Run the suite** (after Task 11, for the same reason as Task 8)

```bash
cd web && pnpm test TagsPage
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/classifications/TagsPage.tsx web/src/features/classifications/TagsPage.test.tsx
git commit -m "$(cat <<'EOF'
feat(web): qualify the two tag sections by purpose

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 10: Reporting-tags folder in the budget table

Replace the flat `LabelsSection` with a collapsed-by-default folder whose tag rows expand to category children.

**Files:**
- Modify: `web/src/api/dto/budget.ts` (`LabelSpendDto`)
- Modify: `web/src/features/budgets/BudgetTable.tsx:264-344,383-393`
- Modify: `web/src/features/budgets/BudgetTransactionsDialog.tsx` (params)
- Test: `web/src/features/budgets/BudgetTable.test.tsx`

**Interfaces:**
- Consumes: Task 5's `children` on the wire; Task 6's `labelId + categoryId` endpoint support.
- Produces: `LabelSpendDto` gains `children: BudgetChildElementDto[]`; `BudgetTransactionsParams` carries `labelId` and `categoryId` together.

- [ ] **Step 1: Write the failing tests**

```tsx
it('collapses the reporting tags folder by default', () => {
  renderTable(budgetWithLabels)
  expect(screen.getByText('Reporting tags')).toBeInTheDocument()
  expect(screen.queryByText('kid-A')).not.toBeInTheDocument()
})

it('reveals reporting tags when the folder is expanded, still collapsed', async () => {
  renderTable(budgetWithLabels)
  await userEvent.click(screen.getByText('Reporting tags'))
  expect(screen.getByText('kid-A')).toBeInTheDocument()
  expect(screen.queryByText('Groceries')).not.toBeInTheDocument()
})

it('reveals a tag\'s categories when that tag is expanded', async () => {
  renderTable(budgetWithLabels)
  await userEvent.click(screen.getByText('Reporting tags'))
  await userEvent.click(screen.getByText('kid-A'))
  expect(screen.getByText('Groceries')).toBeInTheDocument()
})

it('drills down to tag and category together', async () => {
  const onSpentClick = vi.fn()
  renderTable(budgetWithLabels, { onSpentClick })
  await userEvent.click(screen.getByText('Reporting tags'))
  await userEvent.click(screen.getByText('kid-A'))
  await userEvent.click(screen.getByRole('button', { name: /transactions Groceries/ }))
  expect(onSpentClick).toHaveBeenCalledWith(
    expect.objectContaining({ id: 'cat-groceries', parent: expect.objectContaining({ id: 'label-kid-a' }) }),
  )
})
```

Extend the test fixture with a label carrying `children`.

- [ ] **Step 2: Run to verify they fail**

```bash
cd web && pnpm test BudgetTable
```

Expected: FAIL — the flat section renders every label immediately.

- [ ] **Step 3: Add `children` to the DTO**

In `web/src/api/dto/budget.ts`, add `children: BudgetChildElementDto[]` to `LabelSpendDto` (reusing whatever the element children type is called).

- [ ] **Step 4: Rebuild the section as a folder**

Rework `LabelsSection` (rename to `ReportingTagsFolder`) so it renders as a `Collapsible` section matching the real-folder markup already in this file, defaulting closed, with the heading `t('budgets.page.budget.structure.labels.heading')` and the overlap note **inside** the expanded content. Each label becomes a row that is itself collapsible when `label.children.length > 0`, reusing the chevron/`EntityIcon` treatment from `ElementRow` (lines 141–172) and rendering children with the same `pl-8 sm:pl-12` child-row markup (lines 226–255).

Persist both levels in `useBudgetPeriodStore` via `toggleElement`, keying the folder as `__reporting_tags__` and each tag by its own id — these ids never collide with element ids, which are UUIDs.

Child click targets must carry the parent:

```tsx
onClick={() => onLabelClick({ id: child.id, type: child.type, name: child.name, icon: child.icon, currencyId: null, parent: { id: label.id, type: 'label' } })}
```

At the call site (line ~388) keep the placement (after Uncategorized, before Archive) and the `labels.length > 0` guard.

- [ ] **Step 5: Send both params**

In `BudgetTransactionsDialog.tsx`, when a target has a `parent` of type `'label'`, send `labelId: parent.id` together with `categoryId: target.id` (or `uncategorized: true` when the child is the uncategorized bucket).

- [ ] **Step 6: Run the suite**

```bash
cd web && pnpm test BudgetTable BudgetTransactionsDialog && pnpm lint
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add web/src/api/dto/budget.ts web/src/features/budgets
git commit -m "$(cat <<'EOF'
feat(web): fold reporting tags into an expandable folder

Collapsed by default, each tag opening to its category breakdown, so
"where did this go" is answerable without leaving the budget.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 11: Catalogue rewrite across 11 languages

**Files:**
- Modify: `locales/{de,en,es,fr,it,nl,pl,pt,ru,uk,zh}.json`
- Test: `internal/test/i18ntest/`, `web/src/**` vitest suite

**Interfaces:**
- Consumes: key names referenced by Tasks 8, 9, 10.
- Produces: the final catalogue state.

- [ ] **Step 1: Add, repurpose, and remove keys**

**Add** under `classifications.tags.forms.tag.kind`: `tag_option`, `tag_hint`, `label_option`, `label_hint`, `locked_note`.
**Add** under `classifications.tags.pages.settings`: `section_budget`, `section_reporting`.
**Repurpose** `classifications.tags.pages.settings.info` (two-purpose text) and `.create_tag` ("Create tag").
**Remove** `classifications.tags.pages.settings.tags_and_labels_info`, `.create_tag_or_label`, and `classifications.tags.forms.tag.kind.legend` (the new control needs no legend).

English values:

| Key | Value |
|---|---|
| `kind.tag_option` | `Budget money for this tag` |
| `kind.tag_hint` | `Set a limit and track it in your budget. One per transaction.` |
| `kind.label_option` | `Label transactions for reporting` |
| `kind.label_hint` | `See where money went without affecting your budget. Several per transaction.` |
| `kind.locked_note` | `A tag's purpose can't be changed after it's created.` |
| `settings.section_budget` | `Budget tags` |
| `settings.section_reporting` | `Reporting tags` |
| `settings.info` | `Budget tags group spending inside your budget — mark every trip expense with one, for example, and see the breakdown right in the budget. Reporting tags are for reporting only: a transaction can carry several, and they never affect your budget.` |
| `settings.create_tag` | `Create tag` |

Russian:

| Key | Value |
|---|---|
| `kind.tag_option` | `Планировать бюджет по этому тегу` |
| `kind.tag_hint` | `Задайте лимит и следите за ним в бюджете. Один на транзакцию.` |
| `kind.label_option` | `Отмечать транзакции для отчётов` |
| `kind.label_hint` | `Смотрите, куда ушли деньги, не влияя на бюджет. Несколько на транзакцию.` |
| `kind.locked_note` | `Назначение тега нельзя изменить после создания.` |
| `settings.section_budget` | `Теги для бюджета` |
| `settings.section_reporting` | `Теги для отчётов` |
| `settings.create_tag` | `Создать тег` |

Translate the same set for the remaining nine languages, following each catalogue's existing register.

- [ ] **Step 2: Rewrite every remaining "label" value**

Per the spec's wording table, update in all 11 languages:

- `budgets.page.budget.structure.labels.heading` → "Reporting tags"
- `budgets.page.budget.structure.labels.overlap_note` → "…several reporting tags…"
- `accounts.page.preview_transaction_modal.tags.label` → "Tags"
- `accounts.page.preview_transaction_modal.label.label` → "Reporting tag"
- `transactions.import_csv.fields.tag` → "Budget tag"; `.fields.labels` → "Reporting tags"
- `transactions.import_csv.labels_separator.button` / `.title` → "…tag separator" / "Tag separator"
- `transactions.import_csv.new_labels_count` → "This import will create {count} new tag | …new tags"
- `errors.label.name_length` / `errors.label.list_empty` → "Tag name must be…" / "Tags list is empty"
- `classifications.labels.*` values → "…tag" wording (`modals.edit.header` → "Edit tag", `modals.create.header` → "New tag", `modals.delete.title` → "Delete tag?", `forms.label.name.validation.invalid_name` → "Tag name must be 3-64 characters", `pages.settings.header` → "Reporting tags")
- `onboarding.steps.*` and `budgets.page.budget.structure.empty_folder.note` — leave as-is; they say "tags", already correct.

- [ ] **Step 3: Verify no "label" survives in user-facing values**

```bash
python3 - <<'PY'
import json, glob, re
bad = []
for path in sorted(glob.glob('locales/*.json')):
    def walk(d, p=''):
        for k, v in d.items():
            key = f'{p}.{k}' if p else k
            if isinstance(v, dict):
                walk(v, key)
            elif re.search(r'\blabels?\b', str(v), re.I) and not key.endswith('.label'):
                bad.append((path, key, v))
    walk(json.load(open(path)))
for b in bad:
    print(b)
print('CLEAN' if not bad else f'{len(bad)} remaining')
PY
```

Expected: `CLEAN`. (The `.label` key suffix is the shadcn form-field convention — a key name, not user-facing text.)

- [ ] **Step 4: Run every guard**

```bash
go test ./internal/test/i18ntest/ -v
cd web && pnpm test && pnpm lint
```

Expected: PASS — key parity across 11 languages, placeholder-set parity, `t()`-call coverage, and the Task 8/9 suites now green.

- [ ] **Step 5: Commit**

```bash
git add locales
git commit -m "$(cat <<'EOF'
feat(i18n): say "tag" everywhere, qualified by purpose

"Label" named the mechanism and taught nothing; the product now has one
noun, qualified as budget/reporting where the kinds must be told apart.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 12: CSV import surfaces a cross-kind collision as a row error

The spec requires that a CSV whose `labels` cell names an existing *budget* tag fails that row rather than silently creating a twin. Import already routes through `imp.CreateLabel` (`internal/transaction/import.go:381`), which reaches the service path Task 2 changed — so the behavior should already hold. This task **proves** it and pins it against regression.

**Files:**
- Test: `internal/transaction/import_test.go`
- Modify (only if the test proves it necessary): `internal/transaction/import.go:375-395`

**Interfaces:**
- Consumes: Task 2's cross-kind check, Task 3's `CodeTagAlreadyExists`.
- Produces: no API change.

- [ ] **Step 1: Write the test**

Append to `internal/transaction/import_test.go` a case that seeds a **budget tag** named `Trip` for the owner, then imports a CSV whose `labels` column contains `trip` (lower-case, exercising Task 1's case-insensitivity too), and asserts the row is reported as a **row-level error** — not silently skipped, and not creating a reporting tag.

Follow the existing import-test style in that file for building the CSV payload and reading back per-row errors.

```go
	// The name belongs to a budget tag; both kinds share one namespace, so the
	// row must fail rather than quietly creating a same-named reporting tag.
	if len(result.Errors) == 0 {
		t.Fatal("want a row error for the cross-kind name collision, got none")
	}
```

- [ ] **Step 2: Run it**

```bash
go test ./internal/transaction/ -run 'Import.*Label' -v
```

Expected: PASS if the error already propagates as a row error. If it FAILS because the error aborts the whole import instead of failing one row, wrap the `findOrCreateNamed` call at `import.go:380` so a validation error becomes a row error, matching how the tag path at `:335` already handles its failures — then re-run.

- [ ] **Step 3: Confirm no label was created**

Extend the test to assert the owner's label list is unchanged after the failed import, so a partial write cannot slip through.

```bash
go test ./internal/transaction/ -v
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/transaction
git commit -m "$(cat <<'EOF'
test(transaction): pin cross-kind name collisions as row errors

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 13: Full-suite verification

**Files:** none modified unless a failure demands it.

- [ ] **Step 1: Backend smoke tier**

```bash
make go-test
```

Expected: PASS, coverage ≥ `GO_COVER_MIN`.

- [ ] **Step 2: Full tier including PostgreSQL and the frontend**

```bash
make test
```

Expected: PASS — includes `enginecompare` (byte-identical SQLite vs PostgreSQL, covering the new category grouping and both drill-down queries) and the web suite.

- [ ] **Step 3: Confirm goldens are committed and intentional**

```bash
git status --short internal/test/apiparity/testdata internal/test/mcpparity/testdata
```

Expected: clean. Anything unstaged means a golden moved after its task's commit — inspect before adding.

- [ ] **Step 4: Commit any remaining golden churn**

```bash
git add internal/test
git commit -m "$(cat <<'EOF'
test: refresh goldens for the reporting-tags rename

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>
EOF
)"
```

Skip if Step 3 was clean.
