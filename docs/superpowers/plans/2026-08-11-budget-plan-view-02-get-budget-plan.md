# Budget Plan View — 02 get-budget-plan Backend Read Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The `GET /api/v1/budget/get-budget-plan` read (REST + MCP): one multi-month response carrying every plan-sheet row — income and expense, folders, envelopes with children, two synthetic Uncategorized rows — with per-month `{actual, planned}` cells, opening balances, and per-month currency rates, in O(1) transaction queries regardless of window size.

**Architecture:** Three new grouped-by-month queries land in the hand-built budget read repo (`SpendingByMonth`, `IncomeByMonth`, `LimitsByMonth`); a new `builder_plan.go` mirrors the existing `buildStructure` walk but emits per-month cell arrays, converting all amounts in ONE `BulkConvert` call (its `ConvertItem.PeriodStart` already selects per-month rates). The use case (`plan.go`) reuses `requireBudget`/`buildFilters`/`buildMeta`/`buildAverageRates`; `buildFilters` additionally exposes the participants' income categories. The REST handler is a hand-written GET (query params), the MCP tool mirrors it. No existing wire byte changes anywhere.

**Tech Stack:** Go stdlib + existing repo patterns (no new dependencies). Tests: the `internal/budget/api` harness (`newHarness`), repo integration tests (`internal/budget/repo`, sqlite + `make test-repo-pgsql` rerun), apiparity/mcpparity golden suites.

**Spec:** `docs/superpowers/specs/2026-08-02-budget-plan-view-design.md` (amended 2026-08-10) — sections "New read: get-budget-plan" and "Queries", plus the MCP bullet under "Testing". Plan 01 (backend core: income element types, reconciler, envelope sides, folder sides, get-budget exclusions) is already merged on this branch. Plan 03 covers all frontend work.

## Global Constraints

- **Frozen wire contract**: every pre-existing apiparity/mcpparity golden must come out byte-identical. The only golden events are ADDITIVE: the new `budget_plan` REST scenario, the new `budget_plan` MCP scenario, and mcpparity's `tools/list` gaining the `get_budget_plan` tool. Never hand-edit a golden.
- **No new i18n catalogue keys.** The repo has ELEVEN locale catalogues (not just en/ru — check `ls locales/`), and every new key must land in all of them (guard-enforced). This plan needs none: the `months` validation reuses the existing `errs.CodeInvalidChoice` ("The value you selected is not a valid choice.") and blank-field validation reuses `errs.CodeIsBlank`. Do not invent a new error code.
- Element type values (frozen, persisted): envelope=0, category=1, tag=2, income category=3, income envelope=4. Aliases via `ElementType.Alias()`; `elementKey(id, typ)` = `"<id>-<alias>"`.
- All ids UUID TEXT; datetimes `"2006-01-02 15:04:05"` (`datetime.Layout`); date-only `"2006-01-02"` (`datetime.DateLayout`); `isArchived` int 0/1; amounts decimal strings; empty planned cell = `""` (not null).
- SQLite date binds in hand-built SQL MUST go through `sqliteDatetime(t)` (a bound `time.Time` drops first-of-month rows — see the existing comments in `internal/budget/repo/read.go`).
- Month bucketing SQL: sqlite `strftime('%Y-%m-01', col)`, pgsql `to_char(col, 'YYYY-MM') || '-01'` — both produce the TEXT `"YYYY-MM-01"`, byte-identical across engines.
- Comments: only non-obvious business logic / frozen-contract rationale (CLAUDE.md "Comments — write sparingly"). Swag `// @…` blocks are exempt and required on handlers.
- Run tests from the repo root. Smoke per task: `go test ./internal/budget/... -count=1`; full gate at the end: `make go-test` (build + vet + gofmt + OpenAPI-fresh + coverage ≥ 80).
- Go toolchain: if `go` is not on PATH, `export PATH=$HOME/.local/go/bin:$PATH` (go1.26.5 lives in `~/.local/go`).
- Branch: work happens on `feature/budget-plan-view`.
- **Known intermediate failure**: from Task 1 (route registered) until Task 6 (scenario added), `go test ./internal/test/apiparity/` fails its route-coverage guard for `get-budget-plan`, and from Task 5 until Task 6 mcpparity fails on the `tools/list` golden. Both are expected and resolved in Task 6; everything else must stay green after every task.

---

### Task 1: Plan DTOs + use-case skeleton + REST edge

The endpoint exists end-to-end after this task — meta, months, openingBalances, per-month currencyRates, and folders are real; `structure.elements` is `[]` until Tasks 3–4.

**Files:**
- Modify: `internal/model/budget_dto.go` (append after `GetBudgetListResult`, ~line 206)
- Create: `internal/budget/plan.go`
- Create: `internal/budget/builder_plan.go`
- Modify: `internal/budget/api/budget.go` (add the `GetBudgetPlan` handler after `GetBudgetList`)
- Modify: `internal/budget/api/routes.go` (one route line)
- Modify: `internal/budget/accounts.go` (~line 183, stale doc name) and `internal/budget/repository.go` (~lines 56 and 74, stale doc name)
- Test: `internal/budget/api/plan_test.go` (create)

**Interfaces:**
- Consumes: `requireBudget`, `buildMeta`, `buildFilters`, `buildAverageRates`, `parsePeriodDate`, `localMonth`, `sortBudgetFolders`, `sumBalances`, `s.read.AccountsBalancesOnDate` — all existing.
- Produces (later tasks and plan 03 rely on these exact names):
  - DTOs: `model.GetBudgetPlanRequest{Id, From, Months string}`, `model.PlanCellResult{Actual, Planned string}`, `model.PlanChildCellResult{Actual string}`, `model.PlanChildResult`, `model.PlanElementResult`, `model.OpeningBalanceResult{CurrencyId, Amount string}`, `model.PlanMonthRatesResult{Period string, Rates []AverageCurrencyRateResult}`, `model.PlanStructureResult{Folders, Elements}`, `model.BudgetPlanResult`, `model.GetBudgetPlanResult{Item BudgetPlanResult}`.
  - `func (s *Service) GetBudgetPlan(ctx context.Context, userID vo.Id, req model.GetBudgetPlanRequest) (*model.GetBudgetPlanResult, error)`
  - `func (s *Service) BuildBudgetPlan(ctx context.Context, userID vo.Id, b *budgetAggregate, from time.Time, months int) (model.BudgetPlanResult, error)`
  - `func (s *Service) buildPlanStructure(ctx context.Context, b *budgetAggregate, f filters, monthsList []time.Time) (model.PlanStructureResult, error)` — Tasks 3–4 replace its body.
  - Route: `GET /api/v1/budget/get-budget-plan` (query `id`, `from`, `months`).

- [ ] **Step 1: Write the failing tests**

Create `internal/budget/api/plan_test.go`:

```go
package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// decEq compares two decimal strings as normalized values (sqlite float
// rendering vs stored text differ in trailing digits, never in value).
func decEq(a, b string) bool {
	return vo.NewDecimal(a).Sub(vo.NewDecimal(b)).IsZero()
}

func getPlan(t *testing.T, h *harness, tok, query string) (int, envelope) {
	t.Helper()
	return h.do(t, http.MethodGet, "/api/v1/budget/get-budget-plan?"+query, tok, nil)
}

func planItem(t *testing.T, env envelope) model.BudgetPlanResult {
	t.Helper()
	res := mustUnmarshal[model.GetBudgetPlanResult](t, env.Data)
	return res.Item
}

// TestGetBudgetPlan_SkeletonShape: months list, meta, openingBalances,
// per-month currencyRates and folders — the response frame Tasks 3-4 fill.
func TestGetBudgetPlan_SkeletonShape(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": budgetID1, "name": "Plan Budget", "currencyId": usdID, "startDate": "2024-04-01"})

	const folderID2 = "bfff2222-0000-7000-8000-0000000000b0"
	h.do(t, http.MethodPost, "/api/v1/budget/create-folder", tok, map[string]any{
		"budgetId": budgetID1, "id": folderID2, "name": "Bills",
	})

	st, env := getPlan(t, h, tok, "id="+budgetID1+"&from=2024-04-15&months=3")
	if st != http.StatusOK {
		t.Fatalf("get-budget-plan = %d; body=%s", st, env.raw)
	}
	item := planItem(t, env)

	// from snaps to first-of-month; months are ordered date strings.
	wantMonths := []string{"2024-04-01", "2024-05-01", "2024-06-01"}
	if len(item.Months) != 3 || item.Months[0] != wantMonths[0] || item.Months[1] != wantMonths[1] || item.Months[2] != wantMonths[2] {
		t.Fatalf("months = %v, want %v", item.Months, wantMonths)
	}
	if item.Meta.Id != budgetID1 {
		t.Errorf("meta.id = %q", item.Meta.Id)
	}
	// One included USD account with no transactions before the window: one
	// opening balance row, zero-valued.
	if len(item.OpeningBalances) != 1 || item.OpeningBalances[0].CurrencyId != usdID || !decEq(item.OpeningBalances[0].Amount, "0") {
		t.Errorf("openingBalances = %+v", item.OpeningBalances)
	}
	if len(item.CurrencyRates) != 3 {
		t.Fatalf("currencyRates length = %d, want 3", len(item.CurrencyRates))
	}
	for i, cr := range item.CurrencyRates {
		if cr.Period != wantMonths[i] {
			t.Errorf("currencyRates[%d].period = %q want %q", i, cr.Period, wantMonths[i])
		}
		if cr.Rates == nil {
			t.Errorf("currencyRates[%d].rates must be [] not null", i)
		}
	}
	if len(item.Structure.Folders) != 1 || item.Structure.Folders[0].Id != folderID2 || item.Structure.Folders[0].Position != 0 {
		t.Errorf("folders = %+v", item.Structure.Folders)
	}
	if item.Structure.Elements == nil {
		t.Errorf("structure.elements must be [] not null")
	}
}

// TestGetBudgetPlan_Validation: id blank; months non-integer / out of 1..24
// rejected with the frozen invalid-choice message; empty months defaults to 12.
func TestGetBudgetPlan_Validation(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Plan Bounds Budget"))

	st, env := getPlan(t, h, tok, "from=2024-04-01&months=3")
	if st != http.StatusBadRequest {
		t.Fatalf("missing id = %d; body=%s", st, env.raw)
	}
	if msgs := env.errorsMap()["id"]; len(msgs) == 0 || msgs[0] != "This value should not be blank." {
		t.Errorf("id error = %v", env.errorsMap())
	}

	for _, months := range []string{"0", "25", "-1", "junk", "1.5"} {
		st, env = getPlan(t, h, tok, "id="+budgetID1+"&months="+months)
		if st != http.StatusBadRequest {
			t.Fatalf("months=%s -> %d, want 400; body=%s", months, st, env.raw)
		}
		if msgs := env.errorsMap()["months"]; len(msgs) == 0 || msgs[0] != "The value you selected is not a valid choice." {
			t.Errorf("months=%s error = %v", months, env.errorsMap())
		}
	}

	st, env = getPlan(t, h, tok, "id="+budgetID1)
	if st != http.StatusOK {
		t.Fatalf("default window = %d; body=%s", st, env.raw)
	}
	if item := planItem(t, env); len(item.Months) != 12 {
		t.Errorf("default months = %d, want 12", len(item.Months))
	}
}

// TestGetBudgetPlan_RequiresMembership: a non-member gets the standard 403.
func TestGetBudgetPlan_RequiresMembership(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Private Budget"))

	const strangerID = "99992222-0000-7000-8000-0000000000aa"
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.User(fixture.User{ID: strangerID, Email: "stranger@example.test", Name: "Stranger", Password: "pw"})

	st, env := getPlan(t, h, strangerID, "id="+budgetID1+"&months=3")
	if st != http.StatusForbidden {
		t.Fatalf("stranger = %d, want 403; body=%s", st, env.raw)
	}
	_ = json.RawMessage(env.raw)
}
```

Note: `h.token(t)` returns the seed user id (authstub: the bearer token IS the user id), so the stranger's token is just `strangerID`. If `fixture.User` requires more fields, crib from `permissions_test.go`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/budget/api/ -run TestGetBudgetPlan -v`
Expected: FAIL to compile — `undefined: model.GetBudgetPlanResult` (and the route 404s once types exist).

- [ ] **Step 3: Add the DTOs**

Append to `internal/model/budget_dto.go` (after `GetBudgetListResult`):

```go
// GetBudgetPlanRequest selects a budget + a month window for the plan read.
// From and Months arrive as raw query strings; the service parses them.
type GetBudgetPlanRequest struct {
	Id     string `json:"id"`
	From   string `json:"from"`
	Months string `json:"months"`
}

func (r GetBudgetPlanRequest) Validate() error {
	return ValidateBlank(map[string]string{"id": r.Id})
}

// PlanCellResult is one (element, month) plan cell. Planned is "" when no
// limit row exists for that month — the frozen empty-cell encoding.
type PlanCellResult struct {
	Actual  string `json:"actual"`
	Planned string `json:"planned"`
}

// PlanChildCellResult is one (envelope child, month) cell — actual only;
// planned lives on the parent, exactly like budgeted on the budget page.
type PlanChildCellResult struct {
	Actual string `json:"actual"`
}

// PlanChildResult is a category nested under an envelope in the plan sheet.
// Cells align index-for-index with BudgetPlanResult.Months.
type PlanChildResult struct {
	Id          string                `json:"id"`
	Type        int                   `json:"type"`
	Name        string                `json:"name"`
	Icon        string                `json:"icon"`
	IsArchived  int                   `json:"isArchived"`
	OwnerUserId string                `json:"ownerUserId"`
	Cells       []PlanChildCellResult `json:"cells"`
}

// PlanElementResult is one plan-sheet row (income and expense alike; the
// element type encodes the side). Cells align with BudgetPlanResult.Months.
type PlanElementResult struct {
	Id          string            `json:"id"`
	Type        int               `json:"type"`
	Name        string            `json:"name"`
	Icon        string            `json:"icon"`
	CurrencyId  string            `json:"currencyId"`
	IsArchived  int               `json:"isArchived"`
	FolderId    *string           `json:"folderId"`
	Position    int               `json:"position"`
	OwnerUserId *string           `json:"ownerUserId"`
	Cells       []PlanCellResult  `json:"cells"`
	Children    []PlanChildResult `json:"children"`
}

// OpeningBalanceResult is one currency's real account balance at the window
// start (transactions dated <= months[0], the same bound as the budget page's
// startBalance) — the client-side Balance row's seed.
type OpeningBalanceResult struct {
	CurrencyId string `json:"currencyId"`
	Amount     string `json:"amount"`
}

// PlanMonthRatesResult is one window month's average currency rates. Period is
// the REQUESTED month; each rate row reports its own snapped period, exactly
// as the budget page's currencyRates block does.
type PlanMonthRatesResult struct {
	Period string                      `json:"period"`
	Rates  []AverageCurrencyRateResult `json:"rates"`
}

// PlanStructureResult is all folders (income-, expense-sided and neutral) +
// all plan rows.
type PlanStructureResult struct {
	Folders  []BudgetFolderResult `json:"folders"`
	Elements []PlanElementResult  `json:"elements"`
}

// BudgetPlanResult is the full get-budget-plan shape.
type BudgetPlanResult struct {
	Meta            MetaResult             `json:"meta"`
	Months          []string               `json:"months"`
	OpeningBalances []OpeningBalanceResult `json:"openingBalances"`
	CurrencyRates   []PlanMonthRatesResult `json:"currencyRates"`
	Structure       PlanStructureResult    `json:"structure"`
}

// GetBudgetPlanResult is {item: BudgetPlanResult}.
type GetBudgetPlanResult struct {
	Item BudgetPlanResult `json:"item"`
}
```

- [ ] **Step 4: The use case (`internal/budget/plan.go`)**

```go
package budget

import (
	"context"
	"strconv"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

const (
	planMonthsDefault = 12
	planMonthsMax     = 24
)

// GetBudgetPlan returns the multi-month plan sheet for [from, from+months).
// Readable by any accepted member (same visibility as get-budget). `from`
// snaps to first-of-month with the same tolerant fallback as get-budget's
// date param; `months` is strict (see parsePlanMonths).
func (s *Service) GetBudgetPlan(ctx context.Context, userID vo.Id, req model.GetBudgetPlanRequest) (*model.GetBudgetPlanResult, error) {
	budgetID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"id": ""})
	}
	from, err := parsePeriodDate(req.From, localMonth(s.clock.Now(), reqctx.Location(ctx)))
	if err != nil {
		return nil, err
	}
	months, err := parsePlanMonths(req.Months)
	if err != nil {
		return nil, err
	}
	b, err := s.requireBudget(ctx, userID, budgetID)
	if err != nil {
		return nil, err
	}
	result, err := s.BuildBudgetPlan(ctx, userID, b, from, months)
	if err != nil {
		return nil, err
	}
	return &model.GetBudgetPlanResult{Item: result}, nil
}

// parsePlanMonths parses the months query param: empty defaults to 12. Unlike
// the tolerant date fallback, a malformed or out-of-range (1..24) value is a
// hard validation error — silently rendering a different window size than the
// sheet asked for would read as data loss.
func parsePlanMonths(raw string) (int, error) {
	if raw == "" {
		return planMonthsDefault, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > planMonthsMax {
		return 0, errs.NewValidation("Validation failed", errs.FieldError{
			Key: "months", Message: "The value you selected is not a valid choice.", Code: errs.CodeInvalidChoice,
		})
	}
	return n, nil
}
```

- [ ] **Step 5: The builder skeleton (`internal/budget/builder_plan.go`)**

```go
package budget

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/vo"
)

// BuildBudgetPlan assembles the plan sheet: window months, opening balances,
// per-month rates, and the row structure. Unlike BuildBudget it never walks
// month-by-month over transactions — the by-month repo queries return the
// whole window in one pass each (the endpoint stays O(1) queries in the
// window size; only the rate lookups are per-month, and those read the small
// rates table).
func (s *Service) BuildBudgetPlan(ctx context.Context, userID vo.Id, b *budgetAggregate, from time.Time, months int) (model.BudgetPlanResult, error) {
	windowEnd := from.AddDate(0, months, 0)

	meta, err := s.buildMeta(ctx, b)
	if err != nil {
		return model.BudgetPlanResult{}, err
	}
	f, err := s.buildFilters(ctx, userID, b, from, windowEnd)
	if err != nil {
		return model.BudgetPlanResult{}, err
	}

	monthsList := make([]time.Time, months)
	monthStrs := make([]string, months)
	for i := range monthsList {
		m := from.AddDate(0, i, 0)
		monthsList[i] = m
		monthStrs[i] = m.Format(datetime.DateLayout)
	}

	opening, err := s.buildOpeningBalances(ctx, b.budget.CurrencyID, f, from)
	if err != nil {
		return model.BudgetPlanResult{}, err
	}

	rates := make([]model.PlanMonthRatesResult, 0, months)
	for i, m := range monthsList {
		monthRates, rerr := s.buildAverageRates(ctx, m, m.AddDate(0, 1, 0))
		if rerr != nil {
			return model.BudgetPlanResult{}, rerr
		}
		rates = append(rates, model.PlanMonthRatesResult{Period: monthStrs[i], Rates: monthRates})
	}

	structure, err := s.buildPlanStructure(ctx, b, f, monthsList)
	if err != nil {
		return model.BudgetPlanResult{}, err
	}

	return model.BudgetPlanResult{
		Meta:            meta,
		Months:          monthStrs,
		OpeningBalances: opening,
		CurrencyRates:   rates,
		Structure:       structure,
	}, nil
}

// buildOpeningBalances sums per-currency balances of the included accounts as
// of the window start, ordered budget currency first then discovery order —
// the same per-currency ordering rule as the budget page's balances block.
func (s *Service) buildOpeningBalances(ctx context.Context, budgetCurrencyID vo.Id, f filters, from time.Time) ([]model.OpeningBalanceResult, error) {
	rows, err := s.read.AccountsBalancesOnDate(ctx, f.includedAccountIDs, from)
	if err != nil {
		return nil, err
	}
	ordered := make([]vo.Id, 0, len(f.currencyIDs))
	for _, c := range f.currencyIDs {
		if c.Equal(budgetCurrencyID) {
			ordered = append(ordered, c)
			break
		}
	}
	for _, c := range f.currencyIDs {
		if !c.Equal(budgetCurrencyID) {
			ordered = append(ordered, c)
		}
	}
	out := make([]model.OpeningBalanceResult, 0, len(ordered))
	for _, cid := range ordered {
		out = append(out, model.OpeningBalanceResult{
			CurrencyId: cid.String(),
			Amount:     sumBalances(rows, cid.String()).String(),
		})
	}
	return out, nil
}

// buildPlanStructure emits all folders plus every plan row. Tasks 3-4 fill the
// element walk; the skeleton returns folders only.
func (s *Service) buildPlanStructure(ctx context.Context, b *budgetAggregate, f filters, monthsList []time.Time) (model.PlanStructureResult, error) {
	sorted := append([]*model.BudgetFolder(nil), b.folders...)
	sortBudgetFolders(sorted)
	folders := make([]model.BudgetFolderResult, 0, len(sorted))
	for i, fl := range sorted {
		folders = append(folders, model.BudgetFolderResult{Id: fl.ID.String(), Name: fl.Name, Position: i})
	}
	return model.PlanStructureResult{Folders: folders, Elements: []model.PlanElementResult{}}, nil
}
```

(`ctx` and `monthsList` are unused until Task 3 — Go tolerates unused function params, no underscore dance needed.)

- [ ] **Step 6: The REST edge**

`internal/budget/api/budget.go`, after `GetBudgetList`:

```go
// GetBudgetPlan handles GET /api/v1/budget/get-budget-plan.
//
// @Summary  Multi-month budget plan sheet
// @Tags     Budget
// @Produce  json
// @Param    id     query string  true  "Budget id"
// @Param    from   query string  false "Window start (Y-m-d, snapped to first of month; defaults to the current month)"
// @Param    months query integer false "Window length in months (1-24, default 12)"
// @Success  200 {object} apidoc.JsonResponseOk{data=model.GetBudgetPlanResult}
// @Failure  400 {object} apidoc.JsonResponseError
// @Failure  401 {object} apidoc.JsonResponseUnauthorized
// @Failure  500 {object} apidoc.JsonResponseException
// @Security Bearer
// @Router   /api/v1/budget/get-budget-plan [get]
func (h *Handlers) GetBudgetPlan(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	req := model.GetBudgetPlanRequest{Id: q.Get("id"), From: q.Get("from"), Months: q.Get("months")}
	res, err := h.svc.GetBudgetPlan(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.OK(w, res)
}
```

`internal/budget/api/routes.go`, after the `get-budget-list` line:

```go
		mux.Handle("GET /api/v1/budget/get-budget-plan", auth(h.GetBudgetPlan))
```

- [ ] **Step 7: Fix the three stale doc references (plan-01 carry-over)**

The reconciler was renamed `restoreElementsOrder` → `syncElements`; three doc comments still use the old name. Pure comment edits:

- `internal/budget/accounts.go` (~line 183, `getElementSelfHeal` doc): "maintained by restoreElementsOrder" → "maintained by syncElements".
- `internal/budget/repository.go` (~line 56, `EnvelopeStore` doc): "and restoreElementsOrder (move.go)" → "and syncElements (move.go)".
- `internal/budget/repository.go` (~line 74, `ElementStore` doc): "MoveElementList/shiftElements/restoreElementsOrder (move.go)" → "MoveElement/syncElements (move.go)" (match whatever function names actually exist in move.go — verify with grep before writing).

- [ ] **Step 8: Run tests to verify they pass**

Run: `go test ./internal/budget/... -count=1 -run TestGetBudgetPlan -v`
Expected: PASS.
Then: `go test ./internal/budget/... -count=1 && go build ./...`
Expected: PASS. (`go test ./internal/test/apiparity/` now fails its route-coverage guard for the new route — the expected intermediate failure resolved in Task 6; everything else green.)

- [ ] **Step 9: Commit**

```bash
git add internal/model/budget_dto.go internal/budget/plan.go internal/budget/builder_plan.go internal/budget/api/budget.go internal/budget/api/routes.go internal/budget/api/plan_test.go internal/budget/accounts.go internal/budget/repository.go
git commit -m "feat(budget): get-budget-plan read skeleton — window, opening balances, per-month rates"
```

---

### Task 2: The three grouped-by-month repo queries

**Files:**
- Modify: `internal/model/budget_view.go` (three row types, after `SummarizedLimitRow`)
- Modify: `internal/budget/readmodel.go` (three interface methods, after `SummarizedLimits`)
- Modify: `internal/budget/repo/read.go` (three implementations, after `SummarizedLimits`)
- Test: `internal/budget/repo/read_plan_test.go` (create)

**Interfaces:**
- Consumes: the existing `ph`/`phFrom`/`idArgs`/`sqliteDatetime`/`itoa` helpers in `read.go`.
- Produces (Task 3/4 build on these exact shapes):
  - `model.MonthlySpendingRow{Month string, CategoryID *string, TagID *string, CurrencyID string, Amount string}`
  - `model.MonthlyIncomeRow{Month string, CategoryID *string, CurrencyID string, Amount string}`
  - `model.MonthlyLimitRow{ExternalID string, Type int16, Month string, Amount string}`
  - `SpendingByMonth(ctx, categoryIDs, accountIDs []vo.Id, from, to time.Time) ([]model.MonthlySpendingRow, error)`
  - `IncomeByMonth(ctx, accountIDs []vo.Id, from, to time.Time) ([]model.MonthlyIncomeRow, error)`
  - `LimitsByMonth(ctx, budgetID vo.Id, from, to time.Time) ([]model.MonthlyLimitRow, error)`

- [ ] **Step 1: Write the failing test**

Create `internal/budget/repo/read_plan_test.go`. Follow the setup pattern of `read_integration_test.go` in the same package (dbtest.New + fixture builder + `NewReadRepo`); crib its seeding helpers verbatim. The substance:

```go
package repo_test

// (imports and db/fixture setup exactly as read_integration_test.go does —
// dbtest.New(t) selects the engine by DBTEST_ENGINE, so `make test-repo-pgsql`
// reruns this file against PostgreSQL unchanged.)

// Seed (fixed ids, explicit SpentAt values — never wall-clock):
//   user u1, USD account acc1
//   categories: catFood (expense, Type 0), catSalary (income, Type 1)
//   tag tagTrip
//   transactions on acc1:
//     expense  40.00  catFood            2024-04-10 10:00:00
//     expense  60.00  catFood            2024-05-02 09:00:00   (2nd row, same category, next month)
//     expense  15.00  catFood + tagTrip  2024-05-20 12:00:00   (tag-carrying)
//     expense   5.00  no category        2024-05-21 12:00:00   (uncategorized)
//     expense  99.00  catFood            2024-07-01 00:00:00   (OUTSIDE window [Apr..Jul))
//     income 1000.00  catSalary          2024-04-05 08:00:00
//     income   50.00  no category        2024-05-06 08:00:00
//   budget b1 (USD) + elements: elFood (external catFood, type 1),
//     elSalary (external catSalary, type 3)
//   limits: elFood 2024-04-01 -> 100, elFood 2024-05-01 -> 120,
//     elSalary 2024-05-01 -> 1500, elFood 2024-07-01 -> 999 (outside window)

func TestSpendingByMonth(t *testing.T) {
	// window [2024-04-01, 2024-07-01)
	rows, err := readRepo.SpendingByMonth(ctx, []vo.Id{catFoodID}, []vo.Id{acc1ID}, apr, jul)
	// Assert (order-independent — collect into a map keyed month|cat|tag):
	//   {2024-04-01, catFood, no tag}  = 40
	//   {2024-05-01, catFood, no tag}  = 60
	//   {2024-05-01, catFood, tagTrip} = 15   (tag bucketing preserved)
	//   {2024-05-01, nil cat, no tag}  =  5   (NULL category kept)
	//   and NO 2024-07 row (window bound respected, boundary row excluded)
	// Amounts via the normalized-decimal comparison (vo.NewDecimal(...).Sub(...).IsZero()),
	// never string equality — sqlite float rendering differs from pgsql NUMERIC text.
}

func TestIncomeByMonth(t *testing.T) {
	rows, err := readRepo.IncomeByMonth(ctx, []vo.Id{acc1ID}, apr, jul)
	// Assert: {2024-04-01, catSalary} = 1000; {2024-05-01, nil} = 50; nothing else.
}

func TestLimitsByMonth(t *testing.T) {
	rows, err := readRepo.LimitsByMonth(ctx, b1ID, apr, jul)
	// Assert: (catFood, type 1, 2024-04-01)=100, (catFood, 1, 2024-05-01)=120,
	// (catSalary, 3, 2024-05-01)=1500; the 2024-07 limit absent.
}

func TestPlanQueries_EmptyAccountSet(t *testing.T) {
	// Empty accountIDs must short-circuit to (nil, nil) on BOTH engines — an
	// empty IN () is a pgsql syntax error (same guard as CountSpending).
	rows, err := readRepo.SpendingByMonth(ctx, nil, nil, apr, jul)
	// nil, nil; same for IncomeByMonth.
}
```

Write these as real tests (the sketch above pins the cases; the scaffolding mirrors `read_integration_test.go` — including `db.Rebind` for any raw verification SQL and the fixture's `SpentAt` string form `"2024-04-10 10:00:00"`).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/budget/repo/ -run 'TestSpendingByMonth|TestIncomeByMonth|TestLimitsByMonth|TestPlanQueries' -v`
Expected: FAIL to compile — `undefined: SpendingByMonth` (interface + repo methods missing).

- [ ] **Step 3: Add the row types and interface methods**

`internal/model/budget_view.go`, after `SummarizedLimitRow`:

```go
// MonthlySpendingRow is one (month, category?, tag?, currency) expense total
// for the plan sheet. Month is the first-of-month TEXT "YYYY-MM-01", rendered
// identically by both engines so the wire output stays byte-identical.
type MonthlySpendingRow struct {
	Month      string
	CategoryID *string
	TagID      *string
	CurrencyID string
	Amount     string
}

// MonthlyIncomeRow is one (month, category?, currency) income total.
// CategoryID nil = uncategorized income.
type MonthlyIncomeRow struct {
	Month      string
	CategoryID *string
	CurrencyID string
	Amount     string
}

// MonthlyLimitRow is one element's limit in one month (the plan sheet's
// planned cells), keyed by the element's external id + stored type.
type MonthlyLimitRow struct {
	ExternalID string
	Type       int16
	Month      string
	Amount     string
}
```

`internal/budget/readmodel.go`, after `SummarizedLimits`:

```go
	// SpendingByMonth: expense spending per (month, category/tag, currency)
	// over [from, to) — the plan sheet's actual-expense cells in ONE query for
	// the whole window. Bucketing matches CountSpending (rows outside the
	// category set are dropped except NULL-category rows; a tag_id row keeps
	// its tag).
	SpendingByMonth(ctx context.Context, categoryIDs, accountIDs []vo.Id, from, to time.Time) ([]model.MonthlySpendingRow, error)
	// IncomeByMonth: income (type=1) per (month, category, currency) over
	// [from, to) on the given accounts. Deliberately NO category filter: the
	// plan builder files rows whose category is not a participant income
	// category — and NULL-category rows — under the income-side Uncategorized
	// row, so no income ever silently vanishes from the Balance projection.
	IncomeByMonth(ctx context.Context, accountIDs []vo.Id, from, to time.Time) ([]model.MonthlyIncomeRow, error)
	// LimitsByMonth: every element limit of the budget in [from, to) as
	// (external_id, type, month, amount) — the plan sheet's planned cells.
	LimitsByMonth(ctx context.Context, budgetID vo.Id, from, to time.Time) ([]model.MonthlyLimitRow, error)
```

- [ ] **Step 4: Implement in `internal/budget/repo/read.go`**

Place after `SummarizedLimits`. The month expression is the ONLY new SQL idea; everything else (LEFT JOIN with the IN inside the ON, catWhere IN-or-NULL, date binds, float-vs-NUMERIC scan) is copied from `CountSpending`/`SummarizedLimits` — copy it faithfully.

```go
// planMonthExpr renders a datetime column as its first-of-month TEXT
// "YYYY-MM-01" — the same bytes on both engines, so grouped months compare
// and serialize identically.
func (r *ReadRepo) planMonthExpr(col string) string {
	if r.driver == "postgresql" {
		return "to_char(" + col + ", 'YYYY-MM') || '-01'"
	}
	return "strftime('%Y-%m-01', " + col + ")"
}

// SpendingByMonth implements ReadModel.
func (r *ReadRepo) SpendingByMonth(ctx context.Context, categoryIDs, accountIDs []vo.Id, from, to time.Time) ([]model.MonthlySpendingRow, error) {
	// Empty IN () is a no-op on SQLite but a syntax error on PostgreSQL (same
	// guard as CountSpending); an empty category set is NOT a short-circuit —
	// the NULL-category rows still have to come back.
	if len(accountIDs) == 0 {
		return nil, nil
	}
	catArgs := idArgs(categoryIDs)
	accArgs := idArgs(accountIDs)
	month := r.planMonthExpr("t.spent_at")
	var sql string
	var args []any
	if r.driver == "postgresql" {
		accIn := r.ph(1, len(accArgs))
		catWhere := "t.category_id IS NULL"
		if len(catArgs) > 0 {
			catWhere = "(t.category_id IN (" + r.ph(1+len(accArgs), len(catArgs)) + ") OR t.category_id IS NULL)"
		}
		dStart := "$" + itoa(1+len(accArgs)+len(catArgs))
		dEnd := "$" + itoa(2+len(accArgs)+len(catArgs))
		sql = "SELECT " + month + " as month, SUM(t.amount) as amount, t.category_id, t.tag_id, a.currency_id FROM transactions t LEFT JOIN accounts a ON t.account_id = a.id AND a.id IN (" + accIn + ") WHERE t.type = 0 AND " + catWhere + " AND t.spent_at >= " + dStart + " AND t.spent_at < " + dEnd + " GROUP BY month, t.category_id, t.tag_id, a.currency_id"
		args = append(args, accArgs...)
		args = append(args, catArgs...)
		args = append(args, from, to)
	} else {
		accIn := r.ph(1, len(accArgs))
		catWhere := "t.category_id IS NULL"
		if len(catArgs) > 0 {
			catWhere = "(t.category_id IN (" + r.ph(1, len(catArgs)) + ") OR t.category_id IS NULL)"
		}
		sql = "SELECT " + month + " as month, SUM(t.amount) as amount, t.category_id, t.tag_id, a.currency_id FROM transactions t LEFT JOIN accounts a ON t.account_id = a.id AND a.id IN (" + accIn + ") WHERE t.type = 0 AND " + catWhere + " AND t.spent_at >= ? AND t.spent_at < ? GROUP BY month, t.category_id, t.tag_id, a.currency_id"
		args = append(args, accArgs...)
		args = append(args, catArgs...)
		// See sqliteDatetime: a time.Time bound drops the first-of-month row.
		args = append(args, sqliteDatetime(from), sqliteDatetime(to))
	}
	rows, err := r.db(ctx).QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.MonthlySpendingRow
	for rows.Next() {
		var monthStr string
		var categoryID, tagID, currencyID *string
		// SQLite's SUM is float (format 'f',8); PostgreSQL's is exact NUMERIC
		// text — same split as CountSpending.
		a := "0"
		if r.driver == "postgresql" {
			var amount *string
			if err := rows.Scan(&monthStr, &amount, &categoryID, &tagID, &currencyID); err != nil {
				return nil, err
			}
			if currencyID == nil {
				continue
			}
			if amount != nil {
				a = *amount
			}
		} else {
			var amount *float64
			if err := rows.Scan(&monthStr, &amount, &categoryID, &tagID, &currencyID); err != nil {
				return nil, err
			}
			if currencyID == nil {
				continue
			}
			if amount != nil {
				a = strconv.FormatFloat(*amount, 'f', 8, 64)
			}
		}
		out = append(out, model.MonthlySpendingRow{Month: monthStr, CategoryID: categoryID, TagID: tagID, CurrencyID: *currencyID, Amount: a})
	}
	return out, rows.Err()
}

// IncomeByMonth implements ReadModel.
func (r *ReadRepo) IncomeByMonth(ctx context.Context, accountIDs []vo.Id, from, to time.Time) ([]model.MonthlyIncomeRow, error) {
	if len(accountIDs) == 0 {
		return nil, nil
	}
	accArgs := idArgs(accountIDs)
	month := r.planMonthExpr("t.spent_at")
	var sql string
	var args []any
	if r.driver == "postgresql" {
		accIn := r.ph(1, len(accArgs))
		dStart := "$" + itoa(1+len(accArgs))
		dEnd := "$" + itoa(2+len(accArgs))
		sql = "SELECT " + month + " as month, SUM(t.amount) as amount, t.category_id, a.currency_id FROM transactions t LEFT JOIN accounts a ON t.account_id = a.id AND a.id IN (" + accIn + ") WHERE t.type = 1 AND t.spent_at >= " + dStart + " AND t.spent_at < " + dEnd + " GROUP BY month, t.category_id, a.currency_id"
		args = append(args, accArgs...)
		args = append(args, from, to)
	} else {
		accIn := r.ph(1, len(accArgs))
		sql = "SELECT " + month + " as month, SUM(t.amount) as amount, t.category_id, a.currency_id FROM transactions t LEFT JOIN accounts a ON t.account_id = a.id AND a.id IN (" + accIn + ") WHERE t.type = 1 AND t.spent_at >= ? AND t.spent_at < ? GROUP BY month, t.category_id, a.currency_id"
		args = append(args, accArgs...)
		// See sqliteDatetime: a time.Time bound drops the first-of-month row.
		args = append(args, sqliteDatetime(from), sqliteDatetime(to))
	}
	rows, err := r.db(ctx).QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.MonthlyIncomeRow
	for rows.Next() {
		var monthStr string
		var categoryID, currencyID *string
		a := "0"
		if r.driver == "postgresql" {
			var amount *string
			if err := rows.Scan(&monthStr, &amount, &categoryID, &currencyID); err != nil {
				return nil, err
			}
			if currencyID == nil {
				continue
			}
			if amount != nil {
				a = *amount
			}
		} else {
			var amount *float64
			if err := rows.Scan(&monthStr, &amount, &categoryID, &currencyID); err != nil {
				return nil, err
			}
			if currencyID == nil {
				continue
			}
			if amount != nil {
				a = strconv.FormatFloat(*amount, 'f', 8, 64)
			}
		}
		out = append(out, model.MonthlyIncomeRow{Month: monthStr, CategoryID: categoryID, CurrencyID: *currencyID, Amount: a})
	}
	return out, rows.Err()
}

// LimitsByMonth implements ReadModel. SUM + GROUP BY is one row per
// (element, month) anyway — (element_id, period) is unique — but keeps the
// scan on the proven SummarizedLimits float/NUMERIC split.
func (r *ReadRepo) LimitsByMonth(ctx context.Context, budgetID vo.Id, from, to time.Time) ([]model.MonthlyLimitRow, error) {
	month := r.planMonthExpr("l.period")
	var sql string
	var pStart, pEnd any = from, to
	if r.driver == "postgresql" {
		sql = "SELECT e.external_id, e.type, " + month + " as month, SUM(l.amount) as amount FROM budgets_elements_limits l JOIN budgets_elements e ON e.id = l.element_id WHERE e.budget_id = $1 AND l.period >= $2 AND l.period < $3 GROUP BY e.external_id, e.type, month"
	} else {
		sql = "SELECT e.external_id, e.type, " + month + " as month, SUM(l.amount) as amount FROM budgets_elements_limits l JOIN budgets_elements e ON e.id = l.element_id WHERE e.budget_id = ? AND l.period >= ? AND l.period < ? GROUP BY e.external_id, e.type, month"
		// See sqliteDatetime: the period bounds must bind as 'Y-m-d H:i:s' text.
		pStart, pEnd = sqliteDatetime(from), sqliteDatetime(to)
	}
	rows, err := r.db(ctx).QueryContext(ctx, sql, budgetID.String(), pStart, pEnd)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.MonthlyLimitRow
	for rows.Next() {
		var row model.MonthlyLimitRow
		if r.driver == "postgresql" {
			var amount *string
			if err := rows.Scan(&row.ExternalID, &row.Type, &row.Month, &amount); err != nil {
				return nil, err
			}
			if amount != nil {
				row.Amount = *amount
			} else {
				row.Amount = "0"
			}
		} else {
			var amount *float64
			if err := rows.Scan(&row.ExternalID, &row.Type, &row.Month, &amount); err != nil {
				return nil, err
			}
			if amount != nil {
				row.Amount = strconv.FormatFloat(*amount, 'f', 8, 64)
			} else {
				row.Amount = "0"
			}
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
```

PostgreSQL note: `GROUP BY month` (the output alias) is legal in PostgreSQL and SQLite; if the pgsql run rejects it, substitute the full expression in the GROUP BY.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/budget/... -count=1`
Expected: PASS.
If a local PostgreSQL is available (or via the compose stack): `make test-repo-pgsql` — the same file reruns against pgsql. Otherwise defer to Task 6's full gate.

- [ ] **Step 6: Commit**

```bash
git add internal/model/budget_view.go internal/budget/readmodel.go internal/budget/repo/read.go internal/budget/repo/read_plan_test.go
git commit -m "feat(budget): grouped-by-month spending/income/limit queries for the plan read"
```

---

### Task 3: Expense-side plan rows with cells

`buildPlanStructure` grows the real element walk for the EXPENSE side: envelopes (with children), tags (leaf rows), standalone categories, and the expense Uncategorized row. Income-typed elements are skipped in this task (Task 4 adds them) — the walk must already branch on side so Task 4 slots in.

**Files:**
- Modify: `internal/budget/builder_plan.go`
- Test: `internal/budget/api/plan_test.go` (extend)

**Interfaces:**
- Consumes: `SpendingByMonth`/`LimitsByMonth` (Task 2), `elementKey`, `elementOptions`, `envelopeCategories`, `convItem`, `s.convertor.BulkConvert`, `sortBudgetFolders`, `model.UncategorizedID/Name/Icon`.
- Produces (Task 4 extends these):
  - `type planElement struct` / `type planChild struct` (below)
  - `func planKey(monthIdx int, index string) string`, `func planChildKey(monthIdx int, index, subIndex string) string`
  - `func (s *Service) emitPlanElements(...)` — the shared emit pass Task 4 reuses unchanged.

**Semantics being implemented (decisions, from the spec + budget-page mirroring):**
- **Cells**: `Cells[i]` aligns with `Months[i]`. `Actual` is the converted spending total for that month in the ELEMENT's currency (`"0"` when none); `Planned` is the limit amount for that month (`""` when no limit row). Planned renders as `vo.NewDecimal(row.Amount).String()`.
- **Envelopes**: children are the side-matching member categories; a child's cell actual is that category's month total; the parent's actual is the sum of its children's (converted into the envelope currency). Parent planned = the envelope's own limit. Archived children with all-zero cells are dropped (mirrors the budget page). Children sorted by id.
- **Tags are leaf rows** (no children in the plan sheet — the per-category breakdown stays a budget-page drill-down concern). A tag row appears iff it has any actual or planned value in the window (the budget-page participation rule applied per-window).
- **Standalone expense categories**: non-archived always appear (as on the budget page); archived only with activity (any actual/planned cell) in the window.
- **Uncategorized (expense)**: synthetic row, `Id = model.UncategorizedID`, `Type = 1`, actual-only, emitted only when it has any actual; sorts last (the `sortLast` flag pattern).
- **Tag-bucketing rule** (mirror `buildElementsSpending`): a spending row with a tag id belongs to the TAG, not the category — including tagged-but-uncategorized rows.
- **Position**: dense 0-based index per folder over the sort-key order, `sortLast` trailing — the same algorithm as `assignElementPositions`.
- **Conversion**: every (element, month) amount is one entry in a single `BulkConvert` call. Each `ConvertItem` carries its month's `[start, end)` as its period, so the convertor uses that month's average rates per item — no per-month convert calls.

- [ ] **Step 1: Write the failing test**

Append to `internal/budget/api/plan_test.go`:

```go
// TestGetBudgetPlan_ExpenseCells: envelope parent/child cells, tag rows, the
// standalone category, the Uncategorized row, planned-from-limit cells and
// month alignment — all in one window.
func TestGetBudgetPlan_ExpenseCells(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	const rentCatID = "cccc2222-0000-7000-8000-0000000000c0"
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Category(fixture.Category{ID: rentCatID, UserID: seedUserID, Name: "Rent", Type: 0, Icon: "home"})

	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": budgetID1, "name": "Cells Budget", "currencyId": usdID, "startDate": "2024-04-01"})

	// Envelope over the seeded expense category catID ("Food").
	const envID = "beee2222-0000-7000-8000-0000000000c0"
	st, env := h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", tok, map[string]any{
		"budgetId": budgetID1, "id": envID, "name": "Living", "icon": "cart",
		"currencyId": usdID, "folderId": nil, "categories": []string{catID},
	})
	if st != http.StatusOK {
		t.Fatalf("create-envelope = %d; body=%s", st, env.raw)
	}

	// Transactions: Food in Apr + May, tagged expense in May, uncategorized in
	// May, Rent untouched (still visible: non-archived standalone).
	f.Transaction(fixture.Transaction{ID: "d0002222-0000-7000-8000-000000000001", UserID: seedUserID, AccountID: accountID,
		CategoryID: catID, Type: 0, Amount: "40.00000000", SpentAt: "2024-04-10 10:00:00"})
	f.Transaction(fixture.Transaction{ID: "d0002222-0000-7000-8000-000000000002", UserID: seedUserID, AccountID: accountID,
		CategoryID: catID, Type: 0, Amount: "60.00000000", SpentAt: "2024-05-02 09:00:00"})
	f.Transaction(fixture.Transaction{ID: "d0002222-0000-7000-8000-000000000003", UserID: seedUserID, AccountID: accountID,
		CategoryID: catID, TagID: tagID, Type: 0, Amount: "15.00000000", SpentAt: "2024-05-20 12:00:00"})
	f.Transaction(fixture.Transaction{ID: "d0002222-0000-7000-8000-000000000004", UserID: seedUserID, AccountID: accountID,
		Type: 0, Amount: "5.00000000", SpentAt: "2024-05-21 12:00:00"})

	// Limits: envelope Apr=100, May=120; Rent May=300.
	for _, l := range []struct{ id, period, amount string }{
		{envID, "2024-04-01", "100"}, {envID, "2024-05-01", "120"}, {rentCatID, "2024-05-01", "300"},
	} {
		st, env = h.do(t, http.MethodPost, "/api/v1/budget/set-limit", tok, map[string]any{
			"budgetId": budgetID1, "elementId": l.id, "period": l.period, "amount": l.amount,
		})
		if st != http.StatusOK {
			t.Fatalf("set-limit %s %s = %d; body=%s", l.id, l.period, st, env.raw)
		}
	}

	st, env = getPlan(t, h, tok, "id="+budgetID1+"&from=2024-04-01&months=3")
	if st != http.StatusOK {
		t.Fatalf("get-budget-plan = %d; body=%s", st, env.raw)
	}
	item := planItem(t, env)

	byID := map[string]model.PlanElementResult{}
	for _, el := range item.Structure.Elements {
		byID[el.Id] = el
	}

	// Envelope: parent actual = child (Food) totals; planned from its limits.
	envRow, ok := byID[envID]
	if !ok {
		t.Fatalf("envelope row missing; elements=%s", env.Data)
	}
	if len(envRow.Cells) != 3 {
		t.Fatalf("envelope cells = %d, want 3", len(envRow.Cells))
	}
	for i, want := range []struct{ actual, planned string }{
		{"40", "100"}, {"60", "120"}, {"0", ""},
	} {
		c := envRow.Cells[i]
		if !decEq(c.Actual, want.actual) {
			t.Errorf("envelope cell[%d].actual = %q want %q", i, c.Actual, want.actual)
		}
		if want.planned == "" && c.Planned != "" {
			t.Errorf("envelope cell[%d].planned = %q want empty", i, c.Planned)
		}
		if want.planned != "" && !decEq(c.Planned, want.planned) {
			t.Errorf("envelope cell[%d].planned = %q want %q", i, c.Planned, want.planned)
		}
	}
	if len(envRow.Children) != 1 || envRow.Children[0].Id != catID {
		t.Fatalf("envelope children = %+v", envRow.Children)
	}
	if !decEq(envRow.Children[0].Cells[0].Actual, "40") || !decEq(envRow.Children[0].Cells[1].Actual, "60") {
		t.Errorf("child cells = %+v", envRow.Children[0].Cells)
	}

	// Tag row: leaf, May actual 15 (the tagged row belongs to the tag, not Food).
	tagRow, ok := byID[tagID]
	if !ok {
		t.Fatalf("tag row missing")
	}
	if len(tagRow.Children) != 0 {
		t.Errorf("tag rows are leaves in the plan; children=%+v", tagRow.Children)
	}
	if !decEq(tagRow.Cells[1].Actual, "15") {
		t.Errorf("tag May actual = %q", tagRow.Cells[1].Actual)
	}

	// Standalone non-archived category with no activity still shows, planned May=300.
	rentRow, ok := byID[rentCatID]
	if !ok {
		t.Fatalf("standalone category row missing")
	}
	if !decEq(rentRow.Cells[1].Planned, "300") || !decEq(rentRow.Cells[0].Actual, "0") {
		t.Errorf("rent cells = %+v", rentRow.Cells)
	}

	// Uncategorized: actual-only May=5, planned always "".
	uncat, ok := byID[model.UncategorizedID]
	if !ok {
		t.Fatalf("uncategorized row missing")
	}
	if uncat.Type != 1 {
		t.Errorf("uncategorized type = %d want 1", uncat.Type)
	}
	if !decEq(uncat.Cells[1].Actual, "5") || uncat.Cells[1].Planned != "" {
		t.Errorf("uncategorized cells = %+v", uncat.Cells)
	}

	// Food must NOT appear standalone (it lives inside the envelope).
	if _, dup := byID[catID]; dup {
		t.Errorf("envelope child must not surface as a standalone row")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/budget/api/ -run TestGetBudgetPlan_ExpenseCells -v`
Expected: FAIL — "envelope row missing" (elements is still `[]`).

- [ ] **Step 3: Implement the walk in `internal/budget/builder_plan.go`**

Replace `buildPlanStructure` and add the supporting types/helpers. The complete implementation (Task 4 will extend the marked income points):

```go
// planElement / planChild are the in-progress plan rows accumulated before the
// single BulkConvert resolves every (element, month) actual.
type planElement struct {
	id         string
	typ        model.ElementType
	name       string
	icon       string
	ownerID    *string
	currencyID vo.Id
	isArchived bool
	folderID   *string
	sortKey    sortkey.Key
	sortLast   bool
	planned    []string // per month; "" = no limit row
	hasActual  bool     // any convert item accumulated in the window
	children   []planChild
}

type planChild struct {
	id         string
	typ        model.ElementType
	name       string
	icon       string
	ownerID    string
	isArchived bool
	hasActual  bool
}

// planKey / planChildKey are the per-month BulkConvert result keys.
func planKey(monthIdx int, index string) string {
	return fmt.Sprintf("plan%d_%s", monthIdx, index)
}
func planChildKey(monthIdx int, index, subIndex string) string {
	return fmt.Sprintf("plan%d_%s_%s", monthIdx, index, subIndex)
}

func (s *Service) buildPlanStructure(ctx context.Context, b *budgetAggregate, f filters, monthsList []time.Time) (model.PlanStructureResult, error) {
	nMonths := len(monthsList)
	monthIdx := map[string]int{}
	for i, m := range monthsList {
		monthIdx[m.Format(datetime.DateLayout)] = i
	}
	windowEnd := monthsList[0].AddDate(0, nMonths, 0)

	sorted := append([]*model.BudgetFolder(nil), b.folders...)
	sortBudgetFolders(sorted)
	folders := make([]model.BudgetFolderResult, 0, len(sorted))
	for i, fl := range sorted {
		folders = append(folders, model.BudgetFolderResult{Id: fl.ID.String(), Name: fl.Name, Position: i})
	}

	// --- window data: three whole-window queries ---
	expenseCategoryIDs := make([]vo.Id, 0, len(f.categories))
	for idStr := range f.categories {
		id, err := vo.ParseId(idStr)
		if err != nil {
			return model.PlanStructureResult{}, err
		}
		expenseCategoryIDs = append(expenseCategoryIDs, id)
	}
	spendRows, err := s.read.SpendingByMonth(ctx, expenseCategoryIDs, f.includedAccountIDs, monthsList[0], windowEnd)
	if err != nil {
		return model.PlanStructureResult{}, err
	}
	limitRows, err := s.read.LimitsByMonth(ctx, b.budget.ID, monthsList[0], windowEnd)
	if err != nil {
		return model.PlanStructureResult{}, err
	}
	// Task 4 adds: incomeRows, err := s.read.IncomeByMonth(...)

	// planned lookup: elementKey -> per-month amount strings.
	limits := map[string][]string{}
	for _, l := range limitRows {
		i, ok := monthIdx[l.Month]
		if !ok {
			continue
		}
		key := elementKey(l.ExternalID, model.ElementType(l.Type))
		row := limits[key]
		if row == nil {
			row = make([]string, nMonths)
			limits[key] = row
		}
		row[i] = vo.NewDecimal(l.Amount).String()
	}
	plannedFor := func(index string) []string {
		if row, ok := limits[index]; ok {
			return row
		}
		return make([]string, nMonths)
	}

	options := s.elementOptions(b)
	budgetCurrencyID := b.budget.CurrencyID
	elementCurrency := func(index string) vo.Id {
		if opt, ok := options[index]; ok && opt.currencyID != nil {
			return *opt.currencyID
		}
		return budgetCurrencyID
	}

	// envelope membership + stored envelope side (default expense when no row).
	envelopeCats, err := s.envelopeCategories(ctx, b)
	if err != nil {
		return model.PlanStructureResult{}, err
	}
	envelopeType := map[string]model.ElementType{}
	for _, e := range b.elements {
		if e.Type == model.ElementEnvelope || e.Type == model.ElementIncomeEnvelope {
			envelopeType[e.ExternalID.String()] = e.Type
		}
	}
	// categoryID -> owning envelope external id, split by the ENVELOPE's side:
	// only side-matching membership hides a category from the top level (a
	// residual dirty cross-side link must not trap a row invisibly).
	expenseChildOf := map[string]string{}
	incomeChildOf := map[string]string{}
	for _, env := range b.envelopes {
		income := envelopeType[env.ID.String()] == model.ElementIncomeEnvelope
		for _, catID := range envelopeCats[env.ID.String()] {
			if income {
				if _, ok := incomeChildOf[catID]; !ok {
					incomeChildOf[catID] = env.ID.String()
				}
			} else {
				if _, ok := expenseChildOf[catID]; !ok {
					expenseChildOf[catID] = env.ID.String()
				}
			}
		}
	}
	_ = incomeChildOf // consumed in the income walk (Task 4)

	toConvert := map[string][]model.ConvertItem{}
	addActual := func(el *planElement, i int, index string, amount vo.DecimalNumber, currencyID vo.Id, to vo.Id) {
		item := model.ConvertItem{
			PeriodStart: monthsList[i], PeriodEnd: monthsList[i].AddDate(0, 1, 0),
			From: currencyID, To: to, Amount: amount,
		}
		toConvert[planKey(i, index)] = append(toConvert[planKey(i, index)], item)
		el.hasActual = true
	}

	var elements []*planElement

	// --- expense envelopes (income envelopes: Task 4) ---
	for _, env := range b.envelopes {
		if envelopeType[env.ID.String()] == model.ElementIncomeEnvelope {
			continue // Task 4
		}
		index := elementKey(env.ID.String(), model.ElementEnvelope)
		el := &planElement{
			id: env.ID.String(), typ: model.ElementEnvelope, name: env.Name, icon: env.Icon,
			ownerID: nil, currencyID: elementCurrency(index), isArchived: env.IsArchived,
			folderID: optFolder(options[index]), sortKey: optSortKey(options[index]),
			planned: plannedFor(index),
		}
		for _, catID := range envelopeCats[env.ID.String()] {
			cat, ok := f.categories[catID]
			if !ok {
				continue // income or non-participant: renders on its own side
			}
			el.children = append(el.children, planChild{
				id: catID, typ: model.ElementCategory, name: cat.Name, icon: cat.Icon,
				ownerID: cat.OwnerID, isArchived: cat.IsArchived,
			})
		}
		elements = append(elements, el)
	}
	elementByID := map[string]*planElement{}
	for _, el := range elements {
		elementByID[el.id] = el
	}

	// --- tags (leaf rows) + standalone expense categories + expense Uncategorized ---
	tagEls := map[string]*planElement{}
	for tagIDStr, tag := range f.tags {
		index := elementKey(tagIDStr, model.ElementTag)
		el := &planElement{
			id: tagIDStr, typ: model.ElementTag, name: tag.Name, icon: "tag",
			ownerID: strPtr(tag.OwnerID), currencyID: elementCurrency(index), isArchived: tag.IsArchived,
			folderID: optFolder(options[index]), sortKey: optSortKey(options[index]),
			planned: plannedFor(index),
		}
		tagEls[tagIDStr] = el
		elements = append(elements, el)
	}
	catEls := map[string]*planElement{}
	for catIDStr, cat := range f.categories {
		if _, inEnvelope := expenseChildOf[catIDStr]; inEnvelope {
			continue
		}
		index := elementKey(catIDStr, model.ElementCategory)
		el := &planElement{
			id: catIDStr, typ: model.ElementCategory, name: cat.Name, icon: cat.Icon,
			ownerID: strPtr(cat.OwnerID), currencyID: elementCurrency(index), isArchived: cat.IsArchived,
			folderID: optFolder(options[index]), sortKey: optSortKey(options[index]),
			planned: plannedFor(index),
		}
		catEls[catIDStr] = el
		elements = append(elements, el)
	}
	expenseUncat := &planElement{
		id: model.UncategorizedID, typ: model.ElementCategory,
		name: model.UncategorizedName, icon: model.UncategorizedIcon,
		currencyID: budgetCurrencyID, sortLast: true, planned: make([]string, nMonths),
	}
	elements = append(elements, expenseUncat)

	// --- file the expense spending rows ---
	for _, row := range spendRows {
		i, ok := monthIdx[row.Month]
		if !ok {
			continue
		}
		cid, perr := vo.ParseId(row.CurrencyID)
		if perr != nil {
			return model.PlanStructureResult{}, perr
		}
		amount := vo.NewDecimal(row.Amount)
		switch {
		case row.TagID != nil && *row.TagID != "":
			// The tag wins over the category — a tagged expense belongs to its
			// tag row, exactly as on the budget page.
			if el, ok := tagEls[*row.TagID]; ok {
				addActual(el, i, elementKey(el.id, model.ElementTag), amount, cid, el.currencyID)
			}
		case row.CategoryID == nil:
			addActual(expenseUncat, i, elementKey(model.UncategorizedID, model.ElementCategory), amount, cid, budgetCurrencyID)
		default:
			catID := *row.CategoryID
			if envID, inEnvelope := expenseChildOf[catID]; inEnvelope {
				parent := elementByID[envID]
				parentIndex := elementKey(envID, model.ElementEnvelope)
				addActual(parent, i, parentIndex, amount, cid, parent.currencyID)
				subIndex := elementKey(catID, model.ElementCategory)
				toConvert[planChildKey(i, parentIndex, subIndex)] = append(toConvert[planChildKey(i, parentIndex, subIndex)], model.ConvertItem{
					PeriodStart: monthsList[i], PeriodEnd: monthsList[i].AddDate(0, 1, 0),
					From: cid, To: parent.currencyID, Amount: amount,
				})
				for ci := range parent.children {
					if parent.children[ci].id == catID {
						parent.children[ci].hasActual = true
					}
				}
			} else if el, ok := catEls[catID]; ok {
				addActual(el, i, elementKey(catID, model.ElementCategory), amount, cid, el.currencyID)
			}
		}
	}

	// Task 4 files the income rows here.

	converted, err := s.convertor.BulkConvert(ctx, monthsList[0], windowEnd, toConvert)
	if err != nil {
		return model.PlanStructureResult{}, err
	}

	result := s.emitPlanElements(elements, converted, nMonths)
	return model.PlanStructureResult{Folders: folders, Elements: result}, nil
}

// emitPlanElements prunes, renders and positions the accumulated rows.
// Visibility: non-archived elements always render (the sheet is the place to
// START planning a dormant row; hiding empties is a client-side toggle) —
// except tags, which render only when they participate (any actual or planned
// in the window, the budget-page rule applied per-window) and the synthetic
// Uncategorized rows, which render only with actuals. Archived anything needs
// window activity.
func (s *Service) emitPlanElements(elements []*planElement, converted map[string]vo.DecimalNumber, nMonths int) []model.PlanElementResult {
	zero := vo.NewDecimal("0")
	get := func(key string) vo.DecimalNumber {
		if v, ok := converted[key]; ok {
			return v
		}
		return zero
	}
	hasPlanned := func(el *planElement) bool {
		for _, p := range el.planned {
			if p != "" {
				return true
			}
		}
		return false
	}

	var kept []*planElement
	result := []model.PlanElementResult{}
	for _, el := range elements {
		active := el.hasActual || hasPlanned(el)
		switch {
		case el.sortLast: // synthetic Uncategorized rows
			if !el.hasActual {
				continue
			}
		case el.typ == model.ElementTag:
			if !active {
				continue
			}
		default:
			if el.isArchived && !active {
				continue
			}
		}

		index := elementKey(el.id, el.typ)
		cells := make([]model.PlanCellResult, nMonths)
		for i := 0; i < nMonths; i++ {
			cells[i] = model.PlanCellResult{Actual: get(planKey(i, index)).String(), Planned: el.planned[i]}
		}

		children := []model.PlanChildResult{}
		for _, ch := range el.children {
			if ch.isArchived && !ch.hasActual {
				continue
			}
			subIndex := elementKey(ch.id, ch.typ)
			chCells := make([]model.PlanChildCellResult, nMonths)
			for i := 0; i < nMonths; i++ {
				chCells[i] = model.PlanChildCellResult{Actual: get(planChildKey(i, index, subIndex)).String()}
			}
			children = append(children, model.PlanChildResult{
				Id: ch.id, Type: int(ch.typ.Int16()), Name: ch.name, Icon: ch.icon,
				IsArchived: boolToInt(ch.isArchived), OwnerUserId: ch.ownerID, Cells: chCells,
			})
		}
		sort.Slice(children, func(i, j int) bool { return children[i].Id < children[j].Id })

		kept = append(kept, el)
		result = append(result, model.PlanElementResult{
			Id: el.id, Type: int(el.typ.Int16()), Name: el.name, Icon: el.icon,
			CurrencyId: el.currencyID.String(), IsArchived: boolToInt(el.isArchived),
			FolderId: el.folderID, OwnerUserId: el.ownerID,
			Cells: cells, Children: children,
		})
	}

	// Dense 0-based position within the folder over sort-key order, synthetic
	// rows trailing — the same algorithm as assignElementPositions.
	order := make([]int, len(kept))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		x, y := kept[order[a]], kept[order[b]]
		if x.sortLast != y.sortLast {
			return !x.sortLast
		}
		if x.sortKey != y.sortKey {
			return x.sortKey < y.sortKey
		}
		return x.id < y.id
	})
	perFolder := map[string]int{}
	for _, i := range order {
		folder := ""
		if kept[i].folderID != nil {
			folder = *kept[i].folderID
		}
		result[i].Position = perFolder[folder]
		perFolder[folder]++
	}
	sortByPositionThenID(result,
		func(p model.PlanElementResult) int { return p.Position },
		func(p model.PlanElementResult) string { return p.Id })
	return result
}
```

Add the needed imports to `builder_plan.go`: `"fmt"`, `"sort"`, `"github.com/econumo/econumo/internal/shared/sortkey"`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/budget/api/ -run TestGetBudgetPlan -v -count=1`
Expected: PASS (skeleton tests from Task 1 must still pass — folders order, empty-window shape).
Then: `go test ./internal/budget/... -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/budget/builder_plan.go internal/budget/api/plan_test.go
git commit -m "feat(budget): plan sheet expense rows — per-month cells, one BulkConvert"
```

---

### Task 4: Income-side plan rows

Income envelopes (type 4) with income children, standalone income categories (type 3), and the income-side Uncategorized row — plus the pinned behavior for residual dirty cross-side links.

**Files:**
- Modify: `internal/budget/builder.go` (`filters` struct + `buildFilters`: expose income categories)
- Modify: `internal/budget/builder_plan.go` (the marked Task-4 points)
- Test: `internal/budget/api/plan_test.go` (extend)

**Interfaces:**
- Consumes: `IncomeByMonth` (Task 2), `model.ElementIncomeCategory`/`ElementIncomeEnvelope` (plan 01), `incomeChildOf` (Task 3).
- Produces: `filters.incomeCategories map[string]model.CategoryMeta` — populated in `buildFilters`, read ONLY by the plan builder (the frozen get-budget builders never touch it).

**Semantics:**
- Income categories/envelopes mirror the expense rules exactly, with types 3/4 and `IncomeByMonth` as the actual source.
- **Element rows are optional for display**: income rows synthesize from category metadata + envelope side; a `budgets_elements` row only contributes options (currency/folder/sort key) — so pre-plan-01 budgets show their income categories with no backfill.
- **Income Uncategorized**: `Id = model.UncategorizedID`, `Type = 3` (income category), actual-only, `sortLast`. It receives NULL-category income rows AND rows whose category is not a participant income category — no income ever silently vanishes (the Balance-reconciliation rationale in the spec).
- **Dirty-link robustness** (the plan-01 carry-over decision, resolved here): the apiparity fixture — and, in principle, any residual pre-migration row — can hold an EXPENSE envelope with a stored INCOME-category child link. Decision: such a link never hides the category. Side-split membership (`expenseChildOf`/`incomeChildOf` keyed by the envelope's side) means the income category renders as a standalone income row and the expense envelope simply has no such child. The fixture link is deliberately KEPT (Task 6) so the golden pins this rendering.

- [ ] **Step 1: Write the failing tests**

Append to `internal/budget/api/plan_test.go`:

```go
// TestGetBudgetPlan_IncomeRows: income envelope with child cells, standalone
// income category, income-side Uncategorized, planned income from set-limit.
func TestGetBudgetPlan_IncomeRows(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	const salaryCatID = "cccc2222-0000-7000-8000-0000000000c1"
	const bonusCatID = "cccc2222-0000-7000-8000-0000000000c2"
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Category(fixture.Category{ID: salaryCatID, UserID: seedUserID, Name: "Salary", Type: 1, Icon: "payments"})
	f.Category(fixture.Category{ID: bonusCatID, UserID: seedUserID, Name: "Bonus", Type: 1, Icon: "star"})

	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": budgetID1, "name": "Income Plan Budget", "currencyId": usdID, "startDate": "2024-04-01"})

	const incomeEnvID = "beee2222-0000-7000-8000-0000000000c1"
	st, env := h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", tok, map[string]any{
		"budgetId": budgetID1, "id": incomeEnvID, "name": "Salaries", "icon": "payments",
		"currencyId": usdID, "folderId": nil, "side": "income", "categories": []string{salaryCatID},
	})
	if st != http.StatusOK {
		t.Fatalf("create income envelope = %d; body=%s", st, env.raw)
	}

	// Income transactions: salary Apr+May, bonus May, category-less June.
	f.Transaction(fixture.Transaction{ID: "d0003222-0000-7000-8000-000000000001", UserID: seedUserID, AccountID: accountID,
		CategoryID: salaryCatID, Type: 1, Amount: "1000.00000000", SpentAt: "2024-04-05 08:00:00"})
	f.Transaction(fixture.Transaction{ID: "d0003222-0000-7000-8000-000000000002", UserID: seedUserID, AccountID: accountID,
		CategoryID: salaryCatID, Type: 1, Amount: "1100.00000000", SpentAt: "2024-05-05 08:00:00"})
	f.Transaction(fixture.Transaction{ID: "d0003222-0000-7000-8000-000000000003", UserID: seedUserID, AccountID: accountID,
		CategoryID: bonusCatID, Type: 1, Amount: "200.00000000", SpentAt: "2024-05-15 08:00:00"})
	f.Transaction(fixture.Transaction{ID: "d0003222-0000-7000-8000-000000000004", UserID: seedUserID, AccountID: accountID,
		Type: 1, Amount: "50.00000000", SpentAt: "2024-06-06 08:00:00"})

	// Planned income: envelope May=1500, standalone bonus May=250.
	for _, l := range []struct{ id, amount string }{{incomeEnvID, "1500"}, {bonusCatID, "250"}} {
		st, env = h.do(t, http.MethodPost, "/api/v1/budget/set-limit", tok, map[string]any{
			"budgetId": budgetID1, "elementId": l.id, "period": "2024-05-01", "amount": l.amount,
		})
		if st != http.StatusOK {
			t.Fatalf("set-limit %s = %d; body=%s", l.id, st, env.raw)
		}
	}

	st, env = getPlan(t, h, tok, "id="+budgetID1+"&from=2024-04-01&months=3")
	if st != http.StatusOK {
		t.Fatalf("get-budget-plan = %d; body=%s", st, env.raw)
	}
	item := planItem(t, env)

	rows := map[string][]model.PlanElementResult{}
	for _, el := range item.Structure.Elements {
		rows[el.Id] = append(rows[el.Id], el)
	}

	// Income envelope: type 4, child salary cells, planned May=1500.
	envRows := rows[incomeEnvID]
	if len(envRows) != 1 || envRows[0].Type != 4 {
		t.Fatalf("income envelope rows = %+v", envRows)
	}
	er := envRows[0]
	if !decEq(er.Cells[0].Actual, "1000") || !decEq(er.Cells[1].Actual, "1100") || !decEq(er.Cells[1].Planned, "1500") {
		t.Errorf("income envelope cells = %+v", er.Cells)
	}
	if len(er.Children) != 1 || er.Children[0].Id != salaryCatID || er.Children[0].Type != 3 {
		t.Fatalf("income envelope children = %+v", er.Children)
	}
	if !decEq(er.Children[0].Cells[1].Actual, "1100") {
		t.Errorf("salary child cells = %+v", er.Children[0].Cells)
	}

	// Standalone income category: type 3, May actual 200 + planned 250.
	bonusRows := rows[bonusCatID]
	if len(bonusRows) != 1 || bonusRows[0].Type != 3 {
		t.Fatalf("bonus rows = %+v", bonusRows)
	}
	if !decEq(bonusRows[0].Cells[1].Actual, "200") || !decEq(bonusRows[0].Cells[1].Planned, "250") {
		t.Errorf("bonus cells = %+v", bonusRows[0].Cells)
	}
	// Salary must not ALSO appear standalone.
	if len(rows[salaryCatID]) != 0 {
		t.Errorf("income envelope child must not surface standalone")
	}

	// TWO Uncategorized rows can coexist (same id, distinguished by type).
	var uncatTypes []int
	for _, u := range rows[model.UncategorizedID] {
		uncatTypes = append(uncatTypes, u.Type)
	}
	if len(uncatTypes) != 1 || uncatTypes[0] != 3 {
		// only the income one exists here (no uncategorized EXPENSE in this test)
		t.Fatalf("uncategorized rows types = %v, want [3]", uncatTypes)
	}
	for _, u := range rows[model.UncategorizedID] {
		if u.Type == 3 && !decEq(u.Cells[2].Actual, "50") {
			t.Errorf("income uncategorized cells = %+v", u.Cells)
		}
	}
}

// TestGetBudgetPlan_DirtyCrossSideLink: a stored income-category link on an
// EXPENSE envelope (pre-migration residue, unreachable via the API) must not
// trap the category — it renders as a standalone income row and the envelope
// shows no such child.
func TestGetBudgetPlan_DirtyCrossSideLink(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	const salaryCatID = "cccc2222-0000-7000-8000-0000000000c3"
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Category(fixture.Category{ID: salaryCatID, UserID: seedUserID, Name: "Salary Dirty", Type: 1, Icon: "payments"})

	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": budgetID1, "name": "Dirty Link Budget", "currencyId": usdID, "startDate": "2024-04-01"})

	const envID = "beee2222-0000-7000-8000-0000000000c3"
	st, env := h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", tok, map[string]any{
		"budgetId": budgetID1, "id": envID, "name": "Expenses", "icon": "cart",
		"currencyId": usdID, "folderId": nil, "categories": []string{},
	})
	if st != http.StatusOK {
		t.Fatalf("create-envelope = %d; body=%s", st, env.raw)
	}
	// Force the dirty link past the write-path validation.
	if _, err := h.db.Exec(`INSERT INTO budgets_envelopes_categories (envelope_id, category_id) VALUES (?, ?)`, envID, salaryCatID); err != nil {
		t.Fatalf("force dirty link: %v", err)
	}

	st, env = getPlan(t, h, tok, "id="+budgetID1+"&from=2024-04-01&months=2")
	if st != http.StatusOK {
		t.Fatalf("get-budget-plan = %d; body=%s", st, env.raw)
	}
	item := planItem(t, env)
	var sawSalaryStandalone, sawSalaryAsChild bool
	for _, el := range item.Structure.Elements {
		if el.Id == salaryCatID && el.Type == 3 {
			sawSalaryStandalone = true
		}
		for _, ch := range el.Children {
			if ch.Id == salaryCatID {
				sawSalaryAsChild = true
			}
		}
	}
	if !sawSalaryStandalone || sawSalaryAsChild {
		t.Fatalf("dirty link: standalone=%v asChild=%v; want standalone income row only. body=%s",
			sawSalaryStandalone, sawSalaryAsChild, env.Data)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/budget/api/ -run 'TestGetBudgetPlan_IncomeRows|TestGetBudgetPlan_DirtyCrossSideLink' -v`
Expected: FAIL — "income envelope rows = []" (income envelopes are skipped by the Task 3 walk).

- [ ] **Step 3: Expose income categories in `buildFilters`**

`internal/budget/builder.go` — add the field to `filters`:

```go
	categories         map[string]model.CategoryMeta // expense-only, keyed by id
	incomeCategories   map[string]model.CategoryMeta // income-only; read by the plan builder ONLY
```

and in `buildFilters`, replace the expense-only filter loop:

```go
	catMap := map[string]model.CategoryMeta{}
	incomeCatMap := map[string]model.CategoryMeta{}
	for _, c := range cats {
		if c.IsIncome {
			incomeCatMap[c.ID] = c
		} else {
			catMap[c.ID] = c
		}
	}
```

and add `incomeCategories: incomeCatMap,` to the returned `filters` literal. (The get-budget builders never read the new field — the frozen path is untouched; apiparity proves it in Task 6.)

- [ ] **Step 4: Implement the income walk in `buildPlanStructure`**

At the marked Task-4 points in `internal/budget/builder_plan.go`:

(a) Fetch the income rows next to the other two queries:

```go
	incomeRows, err := s.read.IncomeByMonth(ctx, f.includedAccountIDs, monthsList[0], windowEnd)
	if err != nil {
		return model.PlanStructureResult{}, err
	}
```

(b) In the envelope loop, REPLACE the `continue // Task 4` skip with the income branch — the whole loop becomes side-generic:

```go
	for _, env := range b.envelopes {
		typ := model.ElementEnvelope
		catSource := f.categories
		childTyp := model.ElementCategory
		if envelopeType[env.ID.String()] == model.ElementIncomeEnvelope {
			typ = model.ElementIncomeEnvelope
			catSource = f.incomeCategories
			childTyp = model.ElementIncomeCategory
		}
		index := elementKey(env.ID.String(), typ)
		el := &planElement{
			id: env.ID.String(), typ: typ, name: env.Name, icon: env.Icon,
			ownerID: nil, currencyID: elementCurrency(index), isArchived: env.IsArchived,
			folderID: optFolder(options[index]), sortKey: optSortKey(options[index]),
			planned: plannedFor(index),
		}
		for _, catID := range envelopeCats[env.ID.String()] {
			cat, ok := catSource[catID]
			if !ok {
				continue // wrong side or non-participant: renders on its own side
			}
			el.children = append(el.children, planChild{
				id: catID, typ: childTyp, name: cat.Name, icon: cat.Icon,
				ownerID: cat.OwnerID, isArchived: cat.IsArchived,
			})
		}
		elements = append(elements, el)
	}
```

(c) After the standalone-expense-category loop, add standalone income categories and the income Uncategorized row:

```go
	incomeCatEls := map[string]*planElement{}
	for catIDStr, cat := range f.incomeCategories {
		if _, inEnvelope := incomeChildOf[catIDStr]; inEnvelope {
			continue
		}
		index := elementKey(catIDStr, model.ElementIncomeCategory)
		el := &planElement{
			id: catIDStr, typ: model.ElementIncomeCategory, name: cat.Name, icon: cat.Icon,
			ownerID: strPtr(cat.OwnerID), currencyID: elementCurrency(index), isArchived: cat.IsArchived,
			folderID: optFolder(options[index]), sortKey: optSortKey(options[index]),
			planned: plannedFor(index),
		}
		incomeCatEls[catIDStr] = el
		elements = append(elements, el)
	}
	incomeUncat := &planElement{
		id: model.UncategorizedID, typ: model.ElementIncomeCategory,
		name: model.UncategorizedName, icon: model.UncategorizedIcon,
		currencyID: budgetCurrencyID, sortLast: true, planned: make([]string, nMonths),
	}
	elements = append(elements, incomeUncat)
```

(remove the `_ = incomeChildOf` placeholder).

(d) At the "Task 4 files the income rows here" marker, file the income spending:

```go
	for _, row := range incomeRows {
		i, ok := monthIdx[row.Month]
		if !ok {
			continue
		}
		cid, perr := vo.ParseId(row.CurrencyID)
		if perr != nil {
			return model.PlanStructureResult{}, perr
		}
		amount := vo.NewDecimal(row.Amount)
		// Rows with no category — or a category that is not a participant
		// income category — land in the income Uncategorized row, so no income
		// ever vanishes from the sheet's Balance math.
		catID := ""
		if row.CategoryID != nil {
			catID = *row.CategoryID
		}
		if catID == "" || (f.incomeCategories[catID].ID == "" && incomeChildOf[catID] == "") {
			addActual(incomeUncat, i, elementKey(model.UncategorizedID, model.ElementIncomeCategory), amount, cid, budgetCurrencyID)
			continue
		}
		if envID, inEnvelope := incomeChildOf[catID]; inEnvelope {
			parent := elementByID[envID]
			parentIndex := elementKey(envID, model.ElementIncomeEnvelope)
			addActual(parent, i, parentIndex, amount, cid, parent.currencyID)
			subIndex := elementKey(catID, model.ElementIncomeCategory)
			toConvert[planChildKey(i, parentIndex, subIndex)] = append(toConvert[planChildKey(i, parentIndex, subIndex)], model.ConvertItem{
				PeriodStart: monthsList[i], PeriodEnd: monthsList[i].AddDate(0, 1, 0),
				From: cid, To: parent.currencyID, Amount: amount,
			})
			for ci := range parent.children {
				if parent.children[ci].id == catID {
					parent.children[ci].hasActual = true
				}
			}
		} else if el, ok := incomeCatEls[catID]; ok {
			addActual(el, i, elementKey(catID, model.ElementIncomeCategory), amount, cid, el.currencyID)
		}
	}
```

Wiring note: `elementByID` must be (re)built AFTER the envelope loop so it holds both sides' envelope rows — move the `elementByID` construction below the full envelope loop if it isn't already.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/budget/api/ -run TestGetBudgetPlan -v -count=1`
Expected: PASS — all plan tests, including Tasks 1/3's.
Then: `go test ./internal/budget/... -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/budget/builder.go internal/budget/builder_plan.go internal/budget/api/plan_test.go
git commit -m "feat(budget): plan sheet income rows — envelopes, categories, income uncategorized"
```

---

### Task 5: MCP tool `get_budget_plan`

**Files:**
- Modify: `internal/budget/mcp/mcp.go` (one tool, after the `get_budget` tool)
- Modify: `internal/test/mcpparity/catalogue.go` (one scenario, after the existing `budget` scenario)
- Test: `internal/budget/mcp/mcp_test.go` (extend, following its existing per-tool test pattern)

**Interfaces:**
- Consumes: `svc.GetBudgetPlan` (Task 1).
- Produces: MCP tool `get_budget_plan` with input `{budget_id, from_month?, months?}`.

- [ ] **Step 1: Write the failing test**

Append to `internal/budget/mcp/mcp_test.go` (reusing its `newBudgetService`/`connectBudgetSession`/`structured` helpers and the `dbtest.NewSQLite` + `fixture` + `mcptest.CtxWithUser` setup — copy the opening lines of `TestBudgetTools`):

```go
func TestGetBudgetPlanTool(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	budgetID := f.Budget(fixture.Budget{UserID: userID, Name: "Plan Household"})

	svc := newBudgetService(t, db)
	ctx := mcptest.CtxWithUser(t, userID)
	cs := connectBudgetSession(t, ctx, svc)

	res, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name:      "get_budget_plan",
		Arguments: map[string]any{"budget_id": budgetID, "from_month": "2024-04", "months": 3},
	})
	if err != nil {
		t.Fatalf("get_budget_plan: %v", err)
	}
	if res.IsError {
		t.Fatalf("get_budget_plan errored: %#v", res.Content)
	}
	item, ok := structured(t, res)["item"].(map[string]any)
	if !ok {
		t.Fatalf("no item in structured result: %#v", res.StructuredContent)
	}
	months, ok := item["months"].([]any)
	if !ok || len(months) != 3 || months[0] != "2024-04-01" {
		t.Fatalf("months = %#v", item["months"])
	}

	// Malformed from_month is rejected before the service runs.
	res, err = cs.CallTool(ctx, &sdk.CallToolParams{
		Name:      "get_budget_plan",
		Arguments: map[string]any{"budget_id": budgetID, "from_month": "junk"},
	})
	if err != nil {
		t.Fatalf("bad from_month transport error: %v", err)
	}
	if !res.IsError || !strings.Contains(mcptest.Text(t, res), "from_month must be YYYY-MM") {
		t.Fatalf("bad from_month: IsError=%v content=%#v", res.IsError, res.Content)
	}

	// Out-of-range months surfaces the REST read's invalid-choice validation.
	res, err = cs.CallTool(ctx, &sdk.CallToolParams{
		Name:      "get_budget_plan",
		Arguments: map[string]any{"budget_id": budgetID, "months": 40},
	})
	if err != nil {
		t.Fatalf("bad months transport error: %v", err)
	}
	if !res.IsError || !strings.Contains(mcptest.Text(t, res), "not a valid choice") {
		t.Fatalf("bad months: IsError=%v content=%#v", res.IsError, res.Content)
	}
}
```

If the file has no `mcptest.Text`-style helper for a result's text content, extract the text the way the file's existing error-path assertions do (grep `IsError` in `mcp_test.go` and mirror that exact accessor).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/budget/mcp/ -run 'BudgetPlan|GetBudgetPlan' -v`
Expected: FAIL — tool `get_budget_plan` not registered.

- [ ] **Step 3: Implement the tool**

In `internal/budget/mcp/mcp.go`, after the `get_budget` tool registration:

```go
		type getBudgetPlanInput struct {
			BudgetID  string `json:"budget_id" jsonschema:"budget id (UUID), from list_budgets"`
			FromMonth string `json:"from_month,omitempty" jsonschema:"YYYY-MM window start; defaults to the current month"`
			Months    int    `json:"months,omitempty" jsonschema:"window length in months, 1-24; defaults to 12"`
		}

		sdk.AddTool(s, &sdk.Tool{Name: "get_budget_plan",
			Description: "Multi-month plan sheet: every income and expense row with per-month actual and planned amounts, plus opening balances and per-month rates. Planned values are edited with set_limit."},
			func(ctx context.Context, req *sdk.CallToolRequest, in getBudgetPlanInput) (*sdk.CallToolResult, model.GetBudgetPlanResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "get_budget_plan")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.GetBudgetPlanResult{}, err
				}
				from := ""
				if in.FromMonth != "" {
					if _, perr := time.Parse("2006-01", in.FromMonth); perr != nil {
						return nil, model.GetBudgetPlanResult{}, errs.NewValidation("from_month must be YYYY-MM")
					}
					from = in.FromMonth + "-01"
				}
				months := ""
				if in.Months != 0 {
					months = strconv.Itoa(in.Months)
				}
				res, err := svc.GetBudgetPlan(ctx, userID, model.GetBudgetPlanRequest{Id: in.BudgetID, From: from, Months: months})
				if err != nil {
					return nil, model.GetBudgetPlanResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})
```

Add `"strconv"` to the file's imports.

- [ ] **Step 4: Add the mcpparity scenario**

In `internal/test/mcpparity/catalogue.go`, after the existing `budget` scenario (mirror its style exactly — REST-seed with a literal id, then RPC):

```go
	// budget_plan REST-seeds a budget with a fixed 2024 window plus one planned
	// value, then drives get_budget_plan for a valid window and an out-of-range
	// months (the REST read's own scenario pins the full cell math; this pins
	// the MCP edge + schema).
	const mcpPlanBudgetID = "b0000000-0000-0000-0000-0000000000c9"
	register(Scenario{Name: "budget_plan", Steps: []Step{
		{Label: "seed-budget", Method: "POST", Path: "/api/v1/budget/create-budget",
			Body: map[string]any{"id": mcpPlanBudgetID, "name": "MCP Plan Budget", "currencyId": apiparity.USD, "startDate": "2024-04-01"}},
		{Label: "seed-limit", Method: "POST", Path: "/api/v1/budget/set-limit",
			Body: map[string]any{"budgetId": mcpPlanBudgetID, "elementId": apiparity.CatFood, "period": "2024-05-01", "amount": "300"}},
		{Label: "get-budget-plan",
			RPC: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_budget_plan","arguments":{"budget_id":"` + mcpPlanBudgetID + `","from_month":"2024-04","months":3}}}`},
		{Label: "get-budget-plan-bad-months",
			RPC: `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_budget_plan","arguments":{"budget_id":"` + mcpPlanBudgetID + `","from_month":"2024-04","months":40}}}`},
	}})
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/budget/... -count=1`
Expected: PASS.
`go test ./internal/test/mcpparity/ -count=1` now fails on the `tools/list` golden (new tool) and the missing `budget_plan` goldens — the expected intermediate failure; goldens regenerate in Task 6.

- [ ] **Step 6: Commit**

```bash
git add internal/budget/mcp/mcp.go internal/budget/mcp/mcp_test.go internal/test/mcpparity/catalogue.go
git commit -m "feat(budget): get_budget_plan MCP tool"
```

---

### Task 6: Parity scenario, goldens, OpenAPI, full gate

**Files:**
- Create: `internal/test/apiparity/catalogue_budget_plan.go`
- Modify: goldens under `internal/test/apiparity/testdata/golden/` (regenerated, inspected)
- Modify: goldens under `internal/test/mcpparity/testdata/` (regenerated, inspected)
- Modify: `docs/` OpenAPI artifacts via `make swagger`

**Interfaces:**
- Consumes: everything from Tasks 1–5.
- Produces: the route-coverage guard passes again (scenario count +1 REST, +1 MCP); the frozen-contract proof (every pre-existing golden byte-identical) is on record; the dirty-fixture rendering is pinned in a golden.

- [ ] **Step 1: Add the apiparity scenario**

Create `internal/test/apiparity/catalogue_budget_plan.go`. Mirror the `register`/`Scenario`/`Call` conventions of `catalogue_budget_income.go` (same package). The fixture's own transactions sit at the wall-clock `ClockTime`, OUTSIDE the fixed 2024 window — every transaction this scenario needs is created explicitly with a fixed date (copy the create-transaction body shape from `catalogue_transaction.go`, replacing its `ClockTime`-derived date with fixed `"2024-MM-DD HH:MM:SS"` literals).

```go
package apiparity

// get-budget-plan scenario: a fixed 2024 window over explicitly-dated data —
// an income envelope with planned income, an expense limit, transactions in
// distinct months on both sides (including category-less rows for the two
// Uncategorized cells) — plus the months-bounds validation error and a
// closing get-budget re-proof that none of it leaks into the budget view.
//
// The fixture's dirty Envelope1<-CatSalary link (an expense envelope holding
// an income-category child, re-seeded AFTER migrations run — a state
// production can no longer reach) is deliberately KEPT: the plan read must
// stay robust to residual dirty rows, and this golden pins the correct
// rendering — CatSalary surfaces as a standalone income row of the seeded
// Budget's plan, never as an expense-envelope child.

func init() {
	register(Scenario{Name: "budget_plan", Calls: func() []Call {
		const (
			planBudget    = "b0000000-0000-0000-0000-0000000000ef"
			incomeEnv     = "be000000-0000-0000-0000-0000000000ef"
			txAprFood     = "d0000000-0000-0000-0000-0000000000e1"
			txMayFood     = "d0000000-0000-0000-0000-0000000000e2"
			txMayNoCat    = "d0000000-0000-0000-0000-0000000000e3"
			txAprSalary   = "d0000000-0000-0000-0000-0000000000e4"
			txJunNoCatInc = "d0000000-0000-0000-0000-0000000000e5"
		)
		return []Call{
			{Label: "create-budget", Method: "POST", Path: "/api/v1/budget/create-budget", Auth: "owner",
				Body: map[string]any{"id": planBudget, "name": "Plan", "currencyId": USD, "startDate": "2024-04-01"}},
			{Label: "create-income-envelope", Method: "POST", Path: "/api/v1/budget/create-envelope", Auth: "owner",
				Body: map[string]any{"budgetId": planBudget, "id": incomeEnv, "name": "Salaries", "icon": "payments",
					"currencyId": USD, "folderId": nil, "side": "income", "categories": []string{CatSalary}}},
			{Label: "set-expense-limit", Method: "POST", Path: "/api/v1/budget/set-limit", Auth: "owner",
				Body: map[string]any{"budgetId": planBudget, "elementId": CatFood, "period": "2024-05-01", "amount": "300"}},
			{Label: "set-income-limit", Method: "POST", Path: "/api/v1/budget/set-limit", Auth: "owner",
				Body: map[string]any{"budgetId": planBudget, "elementId": incomeEnv, "period": "2024-05-01", "amount": "1500"}},
			// Explicitly-dated transactions — the seeded fixture rows sit at the
			// wall-clock ClockTime, outside this fixed window. Body shape copied
			// from catalogue_transaction.go's create-with-labels call (the "id"
			// is an operation id; the entity gets a server-minted UUIDv7, which
			// the golden normalizer redacts — nothing downstream needs it).
			{Label: "tx-apr-food", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{"id": txAprFood, "accountId": OwnerAccount, "type": "expense",
					"amount": "40.00", "categoryId": CatFood, "date": "2024-04-10 10:00:00"}},
			{Label: "tx-may-food", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{"id": txMayFood, "accountId": OwnerAccount, "type": "expense",
					"amount": "60.00", "categoryId": CatFood, "date": "2024-05-02 09:00:00"}},
			{Label: "tx-may-nocat", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{"id": txMayNoCat, "accountId": OwnerAccount, "type": "expense",
					"amount": "5.00", "date": "2024-05-21 12:00:00"}},
			{Label: "tx-apr-salary", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{"id": txAprSalary, "accountId": OwnerAccount, "type": "income",
					"amount": "1000.00", "categoryId": CatSalary, "date": "2024-04-05 08:00:00"}},
			{Label: "tx-jun-nocat-income", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{"id": txJunNoCatInc, "accountId": OwnerAccount, "type": "income",
					"amount": "50.00", "date": "2024-06-06 08:00:00"}},
			{Label: "get-budget-plan", Method: "GET",
				Path: "/api/v1/budget/get-budget-plan?id=" + planBudget + "&from=2024-04-01&months=3", Auth: "owner"},
			{Label: "err:months-out-of-range", Method: "GET",
				Path: "/api/v1/budget/get-budget-plan?id=" + planBudget + "&from=2024-04-01&months=40", Auth: "owner"},
			// Frozen-contract re-proof: the budget view shows none of the income
			// structure, planned income, or plan-only rows.
			{Label: "get-budget-after", Method: "GET",
				Path: "/api/v1/budget/get-budget?id=" + planBudget + "&date=2024-05-15", Auth: "owner"},
			// The seeded fixture Budget's plan — THIS is the dirty-link pin: its
			// Envelope1 (expense) holds the CatSalary (income) child link.
			{Label: "get-budget-plan-fixture-budget", Method: "GET",
				Path: "/api/v1/budget/get-budget-plan?id=" + Budget + "&from=2024-04-01&months=2", Auth: "owner"},
		}
	}})
}
```

Fill the five create-transaction Calls with the real body shape from `catalogue_transaction.go` (field names, id-as-operation-id semantics, account = `OwnerAccount`); keep the amounts/dates in the comment table.

- [ ] **Step 2: Regenerate and INSPECT the goldens**

```bash
UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ -count=1
UPDATE_GOLDEN=1 go test ./internal/test/mcpparity/ -count=1
git status --short
git diff --stat internal/test/apiparity/testdata/ internal/test/mcpparity/testdata/
git diff internal/test/apiparity/testdata/ internal/test/mcpparity/testdata/ | head -300
```

Inspection checklist — every point must hold before staging:
- **Zero diffs in any pre-existing golden**, REST and MCP — including every `get-budget` golden (the `buildFilters` change added a field nothing on the frozen path reads).
- New REST goldens exist only for `budget_plan`. In its `get-budget-plan` response: `months` = the three 2024 dates; envelope/child cells and both Uncategorized rows carry the comment-table amounts; every planned cell without a limit is `""`; `err:months-out-of-range` carries the standard 400 envelope with the frozen `"The value you selected is not a valid choice."` on field `months`.
- In `get-budget-plan-fixture-budget`: `CatSalary` appears as a standalone `type: 3` row; `Envelope1` has NO child with `CatSalary`'s id.
- `get-budget-after` contains no income envelope id, no `CatSalary`, no `1500`.
- mcpparity: the only pre-existing-file diff is `tools/list` gaining `get_budget_plan` (inspect its schema block); new goldens only for the `budget_plan` scenario.

If any pre-existing golden diffs: STOP, do not regenerate around it — that is a frozen-contract regression to fix in code.

- [ ] **Step 3: Regenerate OpenAPI docs and run the full gate**

```bash
make swagger
make go-test
```

Expected: PASS — docs freshness, both parity guards (scenario/route counts grew, nothing orphaned), i18ntest, coverage ≥ 80.

- [ ] **Step 4: PostgreSQL + engine-comparison tiers**

```bash
make test-repo-pgsql
make test-engines
```

If no local PostgreSQL is running, start a disposable one first (the pattern used for plan 01):

```bash
docker run -d --rm --name econumo-pg-test -e POSTGRES_PASSWORD=test -e POSTGRES_DB=econumo_test -p 5433:5432 postgres:17
export DATABASE_TEST_PGSQL_URL="postgres://postgres:test@127.0.0.1:5433/econumo_test?sslmode=disable"
# ... run the two make targets, then:
docker stop econumo-pg-test
```

Expected: PASS — the three new queries return byte-identical wire output on both engines (the month TEXT expression + the float/NUMERIC scan split are exactly what enginecompare exercises).

- [ ] **Step 5: Commit**

```bash
git add internal/test/apiparity/ internal/test/mcpparity/ docs/
git commit -m "test(budget): get-budget-plan parity scenarios + goldens; OpenAPI"
```

---

## Ordering & dependencies

Tasks run 1 → 6 in order. Task 3 needs Task 2's queries and Task 1's skeleton; Task 4 needs Task 3's walk; Tasks 5–6 need the finished read. The apiparity route-coverage guard is red from Task 1 until Task 6, and mcpparity's `tools/list` golden from Task 5 until Task 6 — both by design; nothing else may be red at any checkpoint.

## Out of scope for this plan

- All frontend work: the plan sheet, mode toggle, totals/Balance math, keyboard grid, row density, `METRICS` events, UI i18n strings, and the `LimitEditor`/`SetLimitDialog`/`useSetLimit`/`canUpdateLimits` explicit-period refactor — plan 03.
- Server-side totals, per-cell carryover display — spec v1 exclusions.
- Any change to `get-budget` or any other existing endpoint's bytes.
- Income-row drag re-ordering (plan-view assignment happens via "Move to folder…" in plan 03; `move-element` already enforces sides from plan 01).
