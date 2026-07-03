# Phase 5: Handler De-Boilerplating Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Collapse the ~17-line decode/validate/call/respond ritual across 77 uniform handlers (62 AuthBody + 12 AuthNoBody + 3 PublicBody) into 1–5-line bodies via three generic helpers, keeping every named method (swag annotations live on them) and leaving the 7 genuinely special handlers hand-written.

**Architecture:** New package `internal/ui/endpoint` (imports `httpx` + `middleware` + `shared/vo` — the same import-cycle constraint that placed `RequireUser` in middleware forbids putting this in `httpx`; spec's "httpx.Handle" name amended accordingly). It promotes the generic adapter that `internal/budget/api/budget.go` already grew locally (proven in-repo pattern). Endpoint methods keep their names + swag blocks; bodies become a single helper call with a closure where per-endpoint extras (`reqctx.AddLogAttr`) exist.

**Spec:** Phase 5 of `docs/superpowers/specs/2026-07-01-feature-package-restructure-design.md`.

## Global Constraints

- **Behavior-identical**: same decode, same validation, same envelope, same log attrs at the same point in the flow. Goldens byte-identical; committed OpenAPI docs byte-identical (`git status --short internal/test/apiparity/testdata internal/ui/apidoc/docs` empty after every task — swag blocks are NOT touched); never UPDATE_GOLDEN.
- Named handler methods and `routes.go` files are untouched except handler BODIES (and budget's local adapter removal). The `RegisterAPI`/TokenVerifier threading and `stubVerifier` middleware tests must be untouched.
- `make test` green per commit (gate 72, guard 84, archtest); tagged tail green; gofmt clean.
- The 7 specials stay hand-written: `budget.GetBudget`, `budget.GetTransactionList`, `transaction.ExportTransactionList` (CSV), `transaction.ImportTransactionList` (multipart), `transaction.GetTransactionList` (query+manual Validate), `user.LoginUser` (clock param + `httpx.Raw` + post-call AddLogAttr). Exception: `user.LogoutUser` converts via `HandleNoBody` with a `_ vo.Id` closure (it gates on auth but ignores the id — same behavior).
- Branch model: base `refactor/feature-packages`, branch per task (`p5/task-NN-…`); merge to `golang` at phase end after `make regression`.

---

### Task 1: `internal/ui/endpoint` + template conversion (category)

**Files:**
- Create: `internal/ui/endpoint/endpoint.go`
- Create: `internal/ui/endpoint/endpoint_test.go`
- Modify: `internal/category/api/{category.go,categorylist.go}` (7 methods — bodies only)

**Interfaces (Produces — Tasks 2-3 rely on these exact signatures):**

```go
// Package endpoint holds the generic request/response combinators the feature
// api packages build their handlers from. A handler METHOD stays named (swag
// annotations live on it); its body delegates here. This package lives outside
// httpx because it needs middleware.RequireUser, and middleware already
// imports httpx.
package endpoint

import (
	"context"
	"net/http"

	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/ui/httpx"
	"github.com/econumo/econumo/internal/ui/middleware"
)

// Handle serves an authenticated JSON endpoint: require user, decode+validate
// Req, call, write the OK envelope. dev gates 500 stack traces exactly as the
// hand-written handlers did.
func Handle[Req any, Res any](w http.ResponseWriter, r *http.Request, dev bool,
	call func(ctx context.Context, userID vo.Id, req Req) (Res, error),
) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}
	var req Req
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(w, err, dev)
		return
	}
	res, err := call(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err, dev)
		return
	}
	httpx.OK(w, res)
}

// HandleNoBody serves an authenticated endpoint with no request body.
func HandleNoBody[Res any](w http.ResponseWriter, r *http.Request, dev bool,
	call func(ctx context.Context, userID vo.Id) (Res, error),
) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}
	res, err := call(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, err, dev)
		return
	}
	httpx.OK(w, res)
}

// HandlePublic serves an unauthenticated JSON endpoint (register, remind,
// reset). No user gate; decode+validate, call, OK envelope.
func HandlePublic[Req any, Res any](w http.ResponseWriter, r *http.Request, dev bool,
	call func(ctx context.Context, req Req) (Res, error),
) {
	var req Req
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(w, err, dev)
		return
	}
	res, err := call(r.Context(), req)
	if err != nil {
		httpx.WriteError(w, err, dev)
		return
	}
	httpx.OK(w, res)
}
```

Conversion pattern (bodies only; the method signature, doc comment, and swag block DO NOT change):

```go
func (h *Handlers) CreateCategory(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, func(ctx context.Context, userID vo.Id, req appcategory.CreateCategoryRequest) (appcategory.CreateCategoryResult, error) {
		reqctx.AddLogAttr(ctx, "category_id", req.Id)
		return h.svc.CreateCategory(ctx, userID, req)
	})
}

func (h *Handlers) ArchiveCategory(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.ArchiveCategory)   // no extras → method value
}

func (h *Handlers) GetCategoryList(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.dev, h.read.GetCategoryList)
}
```

Category's per-method map: CreateCategory/UpdateCategory/DeleteCategory keep their `AddLogAttr("category_id", req.Id)` in the closure (same position: after decode, before svc); ArchiveCategory/UnarchiveCategory/OrderCategoryList use method values; GetCategoryList uses HandleNoBody on `h.read`.

`endpoint_test.go` — unit tests using a stub call (no server): Handle returns 401 without user in ctx (build ctx via the middleware package's exported surface — same technique as `middleware_test.go`'s RequireUser tests; if the ctx key isn't reachable from outside the middleware package, route through `middleware.JWT` with a real token from `internal/test/testkeys`, or assert the 401 path only via the full-stack goldens and unit-test just decode-failure + success + svc-error paths with a user injected through an exported test hook — INSPECT first, pick the simplest that works, report the choice); decode failure → 400 envelope; svc error → WriteError path; success → OK envelope.

- [ ] **Step 1:** Create the package verbatim + tests; `go test ./internal/ui/endpoint/` PASS.
- [ ] **Step 2:** Convert category's 7 methods per the map; delete now-unused imports.
- [ ] **Step 3:** Verify: `make test`; tagged tail; goldens + OpenAPI status EMPTY; archtest green.
- [ ] **Step 4:** Commit `refactor(category): handlers on the generic endpoint helpers`.

---

### Task 2: Convert account, connection, currency, payee, tag, user

Per the audit table (in this plan's dispatch context): account 12 methods (11 A/B + none special), connection 7 (all A/B), currency 2 (B on `h.read`), payee 7, tag 7, user 10 of 13 (A: UpdateBudget/UpdateCurrency/UpdateName/UpdatePassword/UpdateReportPeriod; B: CompleteOnboarding on svc, GetOptionList/GetUserData on read; C via HandlePublic: RegisterUser/RemindPassword/ResetPassword; converts LogoutUser via `HandleNoBody(w, r, h.dev, func(ctx context.Context, _ vo.Id) (appuser.LogoutResult, error) { return h.svc.Logout(ctx) })`; LEAVES LoginUser hand-written).

Notes: account.CreateAccount keeps its `AddLogAttr("account_id", req.Id)` closure. connection.GenerateInvite/DeleteInvite keep their unconditional DecodeValidate semantics (Handle does exactly that — the swag optional-body mismatch is pre-existing doc quirk, do NOT change annotations).

- [ ] **Step 1:** Convert feature by feature, compiling between each.
- [ ] **Step 2:** Verify after each feature (`go build` + focused `go test ./internal/<feature>/...`); full `make test` + tagged tail + goldens/docs empty at the end.
- [ ] **Step 3:** Commit `refactor: remaining features on the generic endpoint helpers` (or one commit per feature if any needs discussion — implementer's judgment, report).

---

### Task 3: Convert transaction + budget; delete budget's local adapter

- transaction: CreateTransaction/UpdateTransaction/DeleteTransaction → `endpoint.Handle` method values. ExportTransactionList/ImportTransactionList/GetTransactionList stay hand-written (note their bodies still use middleware.RequireUser + httpx directly — untouched).
- budget: replace the local `handle[Req,Res]`/`optQuery`-adjacent generic plumbing in `internal/budget/api/budget.go` with `endpoint.Handle`/`endpoint.HandleNoBody` across the 19 A/B methods in `budget.go` + `budget_more.go`; DELETE the local `handle` adapter once unused. GetBudget/GetTransactionList (query-param specials) stay hand-written (keep `optQuery` if only they use it).

- [ ] **Step 1:** Convert; sweep: budget's local `func handle[` gone; `grep -rn 'endpoint.Handle' internal | wc -l` ≈ 77.
- [ ] **Step 2:** Verify: `make test`; tagged tail; goldens/docs empty; archtest green.
- [ ] **Step 3:** Commit `refactor(transaction,budget): handlers on the generic endpoint helpers; drop local adapter`.

---

### Task 4: Phase checkpoint (controller)

- [ ] `make test` + `make regression` PASS; CLAUDE.md's API-handler-pattern paragraph updated to describe the endpoint helpers (one short edit, controller or fold into Task 3); final whole-branch review; merge to `golang`.

---

## Verification checklist (end of phase)

- [ ] 77 handlers on `endpoint.{Handle,HandleNoBody,HandlePublic}`; 6 specials + LoginUser hand-written; budget's local adapter gone.
- [ ] Goldens + OpenAPI byte-identical across the phase; stubVerifier tests untouched; `make regression` green.
- [ ] Branch merged into `golang`.
