# Phase 0: Test Investment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Strengthen the test safety net (composition-root smoke harness with golden files, full 84-route scenario coverage, cli/storage tests, raised coverage gate) before the feature-package restructure begins.

**Architecture:** Extract the enginecompare API scenario catalogue into an untagged shared package `internal/test/apiparity`. The same catalogue then drives two consumers: a new sqlite-only smoke suite with committed golden response files (runs in every `make test`, exercises the real `server.BuildAPI`), and the existing build-tagged two-engine parity suite (runs in `make regression`). A route-coverage guard test with a shrinking allowlist forces every registered route into the catalogue.

**Tech Stack:** Go stdlib testing, `net/http/httptest`, `internal/test/{dbtest,fixture,testkeys}`, `server.BuildAPI`, golden files under `testdata/`.

**Spec:** `docs/superpowers/specs/2026-07-01-feature-package-restructure-design.md` (Phase 0 section).

## Global Constraints

- **Wire contract is frozen**: never change routes, envelope, datetime formats, validation strings. Tests PIN this behavior; they must never motivate changing it.
- **No SQL, sqlc config, generated-code, or migration changes.** `internal/infra/storage/**` is only *read* by tests, except one new test file in `internal/infra/storage/sqlite/`.
- **No production code changes in Phase 0** except: the `GO_COVER_MIN` line in `Makefile` (Task 16). Everything else is test code, fixture-builder additions in `internal/test/`, and the shared catalogue package.
- `make test` must pass after EVERY commit. `make regression` at the checkpoints marked below (needs the compose PostgreSQL or `DATABASE_TEST_PGSQL_URL`).
- Existing tests are never deleted or weakened.
- Baseline measured 2026-07-01: cross-package total 68.0%, gate 64%. Registered routes: 84; in enginecompare catalogue: 49.

## Key existing code (read before starting any task)

- `internal/test/enginecompare/apiparity_harness_test.go` — harness: `newAPIHarness` boots `server.BuildAPI(cfg, db.Raw, jwtSvc, clk)` over `httptest`, `seedAPIFixture` seeds via `fixture.New(t, db).WithCrypto("")`, fixed IDs `apiOwnerID`/`apiGuestID`/etc., `apiClockTime = time.Now().UTC().Truncate(time.Second)`.
- `internal/test/enginecompare/apiparity_test.go` — `apiCall{label,method,path,auth,body}`, `apiScenario func() []apiCall`, `runAPIOnBoth` (sqlite reference vs pgsql), `replay`, `normalizeBody` (redacts UUIDv7 via `uuidV7Re`).
- `internal/test/dbtest` — `dbtest.NewSQLite(t)`, `dbtest.NewPostgres(t)` (skips when `DATABASE_TEST_PGSQL_URL` unset); `*dbtest.DB` has `.Raw`, `.Engine`, `.TX`.
- `internal/test/fixture/entities.go` — typed builder; existing methods include `User`, `DefaultOptions`, `Connect`, `Folder`, `Account`, `AccountInFolder`, `AccountOption`, `AccountAccess`, `Category`, `Tag`, `Payee`, `Transaction`, `Budget`, `BudgetElement`, `BudgetLimit`.
- `internal/cli/cli.go` — `func Run(args []string) int`; existing tests in `cli_test.go` cover the registry and usage paths only.

---

### Task 1: Extract shared catalogue package `internal/test/apiparity`

Pure code motion + export renames. The enginecompare package keeps ONLY the two-engine comparison; everything reusable moves to an untagged package.

**Files:**
- Create: `internal/test/apiparity/apiparity.go` (Call/Scenario/catalogue registry)
- Create: `internal/test/apiparity/harness.go` (harness, moved from `apiparity_harness_test.go`)
- Create: `internal/test/apiparity/fixture.go` (seed + ID constants, moved from `apiparity_harness_test.go`)
- Create: `internal/test/apiparity/normalize.go`
- Create: `internal/test/apiparity/catalogue.go` (the existing scenarios, moved from `apiparity_test.go`)
- Modify: `internal/test/enginecompare/apiparity_test.go` (keep only `runAPIOnBoth` + the parity `Test…` funcs, consuming apiparity)
- Delete: `internal/test/enginecompare/apiparity_harness_test.go` (content absorbed)

**Interfaces (Produces — every later task consumes these):**
```go
package apiparity

type Call struct {
    Label  string // unique within a scenario; prefix "err:" marks an expected-non-2xx call
    Method string // "GET" | "POST"
    Path   string // "/api/v1/…", MAY carry a query string
    Auth   string // "owner" | "guest" | "" (public)
    Body   any    // JSON-marshalled when non-nil
    // For non-JSON requests (multipart import). When RawBody != nil it wins over Body.
    RawBody     []byte
    ContentType string
}
type Scenario struct {
    Name  string
    Calls func() []Call
}
func Catalogue() []Scenario            // ordered registry of ALL scenarios
func register(s Scenario)              // package-private; each scenario file calls it from init()
func Seed(t testing.TB, db *dbtest.DB) // was seedAPIFixture
func NewHarness(t *testing.T, db *dbtest.DB) *Harness // was newAPIHarness
func (h *Harness) Replay(t *testing.T, calls []Call) (statuses []int, bodies [][]byte)
func (h *Harness) Engine() string
var ClockTime time.Time                // was apiClockTime (fixed instant, near-now)
func NormalizeParity(b []byte) string  // was normalizeBody: redact UUIDv7 only
const SeedPassword = "secret-pw"
// Exported fixture ids (was api… consts): OwnerID, OwnerEmail, GuestID, GuestEmail,
// USD, OwnerFolder, GuestFolder, OwnerAccount, SharedAccount, CatFood, CatSalary,
// TagWork, PayeeShop, Txn1, Txn2, Budget — same literal values as today.
```

- [ ] **Step 1: Create the package with moved code.** Copy the harness, fixture seed, constants, and normalizer out of the two tagged files into the four new files, applying ONLY these changes: drop the `//go:build enginecompare` line, rename to the exported identifiers above, add the `RawBody`/`ContentType` fields to `Call`, and add the registry:

```go
// internal/test/apiparity/apiparity.go
// Package apiparity holds the shared API scenario catalogue: an ordered set of
// HTTP call sequences replayed against the REAL production handler
// (server.BuildAPI). Two consumers: the untagged sqlite smoke suite in this
// package (golden files, every `make test`) and the build-tagged enginecompare
// parity suite (sqlite-vs-pgsql byte equality, `make regression`).
package apiparity

var catalogue []Scenario

func register(s Scenario) { catalogue = append(catalogue, s) }

func Catalogue() []Scenario { return catalogue }
```

In `harness.go`, extend the request builder (was `h.call`) to honor `RawBody`:

```go
if c.RawBody != nil {
    rdr = bytes.NewReader(c.RawBody)
} else if c.Body != nil { /* existing JSON marshal */ }
...
if c.ContentType != "" {
    req.Header.Set("Content-Type", c.ContentType)
} else if c.Body != nil {
    req.Header.Set("Content-Type", "application/json")
}
```

- [ ] **Step 2: Move the existing scenarios into `catalogue.go`.** Each former `TestAPIParity_Xxx` body becomes a registered scenario; keep names identical (they are the golden-file keys later):

```go
// internal/test/apiparity/catalogue.go — pattern, repeat for EVERY existing scenario
func init() {
    register(Scenario{Name: "user_reads", Calls: func() []Call {
        return []Call{
            {Label: "get-user-data", Method: "POST", Path: "/api/v1/user/get-user-data", Auth: "owner", Body: map[string]any{}},
            {Label: "get-option-list", Method: "POST", Path: "/api/v1/user/get-option-list", Auth: "owner", Body: map[string]any{}},
        }
    }})
}
```

- [ ] **Step 3: Rewrite the enginecompare consumer.** `internal/test/enginecompare/apiparity_test.go` (still `//go:build enginecompare`) shrinks to one test that iterates the shared catalogue:

```go
func TestAPIParity_Catalogue(t *testing.T) {
    for _, sc := range apiparity.Catalogue() {
        sc := sc
        t.Run(sc.Name, func(t *testing.T) {
            calls := sc.Calls()
            var refStatus []int
            var refBody [][]byte
            t.Run("sqlite", func(t *testing.T) {
                h := apiparity.NewHarness(t, dbtest.NewSQLite(t))
                refStatus, refBody = h.Replay(t, calls)
            })
            t.Run("postgresql", func(t *testing.T) {
                h := apiparity.NewHarness(t, dbtest.NewPostgres(t)) // SKIPs if env unset
                pgStatus, pgBody := h.Replay(t, calls)
                for i := range calls {
                    if pgStatus[i] != refStatus[i] {
                        t.Errorf("[%s] status mismatch: sqlite=%d pgsql=%d", calls[i].Label, refStatus[i], pgStatus[i])
                    }
                    if ref, pg := apiparity.NormalizeParity(refBody[i]), apiparity.NormalizeParity(pgBody[i]); ref != pg {
                        t.Errorf("[%s] body mismatch:\n  sqlite: %s\n  pgsql : %s", calls[i].Label, ref, pg)
                    }
                }
            })
        })
    }
}
```

Other tagged files (`scenarios_test.go`, `budget_tag_visibility_test.go`, `connection_invite_test.go`, `reset_password_test.go`, `harness_test.go`, `doc.go`) stay untouched.

- [ ] **Step 4: Verify both tiers.**

Run: `make test`
Expected: PASS (apiparity compiles untagged; nothing runs its scenarios yet)

Run: `CGO_ENABLED=0 go test -count=1 -tags enginecompare ./internal/test/enginecompare/ -run TestAPIParity -v 2>&1 | tail -20`
Expected: PASS, sqlite subtests run, postgresql subtests SKIP (or run, if PG env is set); same scenario names as before the move.

- [ ] **Step 5: Commit**

```bash
git add internal/test/apiparity internal/test/enginecompare
git commit -m "test: extract shared API scenario catalogue into internal/test/apiparity"
```

---

### Task 2: Sqlite smoke suite with golden response files

The characterization suite: replays the whole catalogue against `server.BuildAPI` on sqlite in the untagged tier and diffs normalized responses against committed goldens. This is what pins route behavior through Phases 1–6.

**Files:**
- Modify: `internal/test/apiparity/normalize.go` (add `NormalizeGolden`)
- Create: `internal/test/apiparity/smoke_test.go`
- Create: `internal/test/apiparity/testdata/golden/*.golden` (generated)

**Interfaces:**
- Consumes: `Catalogue()`, `NewHarness`, `Replay`, `NormalizeParity` from Task 1.
- Produces: `NormalizeGolden(b []byte) string`; the convention that a `Call.Label` starting with `"err:"` may return 4xx, all other calls MUST return 2xx.

- [ ] **Step 1: Add the golden normalizer.** Golden files must be stable across runs (the harness clock is near-now), so beyond UUIDv7 also redact datetimes, dates, and JWTs:

```go
// normalize.go
var (
    datetimeRe = regexp.MustCompile(`\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}`)
    dateRe     = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)
    jwtRe      = regexp.MustCompile(`eyJ[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+`)
)

// NormalizeGolden makes a response body stable across runs AND engines: the
// parity redaction (UUIDv7) plus clock-derived datetimes/dates and JWTs.
// Everything else — field names, amounts, names, ordering, envelope shape,
// validation messages — is compared byte-for-byte against the golden.
func NormalizeGolden(b []byte) string {
    s := NormalizeParity(b)
    s = jwtRe.ReplaceAllString(s, "<jwt>")
    s = datetimeRe.ReplaceAllString(s, "<datetime>")
    s = dateRe.ReplaceAllString(s, "<date>")
    return s
}
```

- [ ] **Step 2: Write the smoke runner.**

```go
// smoke_test.go
package apiparity

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"
    "testing"

    "github.com/econumo/econumo/internal/test/dbtest"
)

// TestSmoke_Catalogue replays every catalogue scenario against the REAL
// production handler (server.BuildAPI) on a fresh sqlite DB and compares each
// normalized response against the committed golden file. Regenerate goldens
// with: UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ — then INSPECT the
// diff before committing: a golden change means observable behavior changed.
func TestSmoke_Catalogue(t *testing.T) {
    for _, sc := range Catalogue() {
        sc := sc
        t.Run(sc.Name, func(t *testing.T) {
            h := NewHarness(t, dbtest.NewSQLite(t))
            calls := sc.Calls()
            statuses, bodies := h.Replay(t, calls)

            var got strings.Builder
            for i, c := range calls {
                if !strings.HasPrefix(c.Label, "err:") && (statuses[i] < 200 || statuses[i] > 299) {
                    t.Errorf("[%s] expected 2xx, got %d: %s", c.Label, statuses[i], bodies[i])
                }
                fmt.Fprintf(&got, "== %s %s %s [%s] -> %d\n%s\n", c.Label, c.Method, c.Path, c.Auth, statuses[i], NormalizeGolden(bodies[i]))
            }

            golden := filepath.Join("testdata", "golden", sc.Name+".golden")
            if os.Getenv("UPDATE_GOLDEN") != "" {
                if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
                    t.Fatal(err)
                }
                if err := os.WriteFile(golden, []byte(got.String()), 0o644); err != nil {
                    t.Fatal(err)
                }
                return
            }
            want, err := os.ReadFile(golden)
            if err != nil {
                t.Fatalf("missing golden %s (run with UPDATE_GOLDEN=1): %v", golden, err)
            }
            if string(want) != got.String() {
                t.Errorf("golden mismatch for %s.\n--- want\n%s\n--- got\n%s", sc.Name, want, got.String())
            }
        })
    }
}
```

- [ ] **Step 3: Generate the goldens and inspect them.**

Run: `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ && git status --short internal/test/apiparity/testdata/`
Expected: one `.golden` per existing scenario. Open two of them and check: envelope `{"success":true,…}` shapes present, no raw datetimes/JWTs surviving normalization.

- [ ] **Step 4: Run the suite for real.**

Run: `go test ./internal/test/apiparity/ -v 2>&1 | tail -15`
Expected: PASS, one subtest per scenario.

Run: `make test`
Expected: PASS. Note the printed total coverage — it should JUMP (server/… now covered); record the number for Task 16.

- [ ] **Step 5: Commit**

```bash
git add internal/test/apiparity
git commit -m "test: sqlite smoke suite replaying the API catalogue against goldens"
```

---

### Task 3: Route-coverage guard with shrinking allowlist

**Files:**
- Create: `internal/test/apiparity/guard_test.go`

**Interfaces:**
- Consumes: `Catalogue()`.
- Produces: `missingFromCatalogue` — the allowlist var each scenario task (5–11) shrinks; Task 12 deletes it.

- [ ] **Step 1: Write the guard.**

```go
// guard_test.go
package apiparity

import (
    "os"
    "path/filepath"
    "regexp"
    "runtime"
    "strings"
    "testing"
)

// missingFromCatalogue lists registered routes that do NOT yet have a
// catalogue scenario. Every entry here is a hole in the safety net. Tasks in
// docs/superpowers/plans/2026-07-01-phase0-test-investment.md remove entries
// as scenarios are added; the list must reach empty and then be deleted.
var missingFromCatalogue = map[string]bool{
    "GET /api/v1/budget/get-transaction-list":            true,
    "GET /api/v1/transaction/export-transaction-list":    true,
    "POST /api/v1/account/order-account-list":            true,
    "POST /api/v1/account/order-folder-list":             true,
    "POST /api/v1/account/replace-folder":                true,
    "POST /api/v1/account/show-folder":                   true,
    "POST /api/v1/budget/accept-access":                  true,
    "POST /api/v1/budget/change-element-currency":        true,
    "POST /api/v1/budget/create-envelope":                true,
    "POST /api/v1/budget/create-folder":                  true,
    "POST /api/v1/budget/decline-access":                 true,
    "POST /api/v1/budget/delete-envelope":                true,
    "POST /api/v1/budget/delete-folder":                  true,
    "POST /api/v1/budget/exclude-account":                true,
    "POST /api/v1/budget/grant-access":                   true,
    "POST /api/v1/budget/include-account":                true,
    "POST /api/v1/budget/move-element-list":              true,
    "POST /api/v1/budget/order-folder-list":              true,
    "POST /api/v1/budget/reset-budget":                   true,
    "POST /api/v1/budget/revoke-access":                  true,
    "POST /api/v1/budget/update-envelope":                true,
    "POST /api/v1/budget/update-folder":                  true,
    "POST /api/v1/category/order-category-list":          true,
    "POST /api/v1/connection/revoke-account-access":      true,
    "POST /api/v1/connection/set-account-access":         true,
    "POST /api/v1/payee/order-payee-list":                true,
    "POST /api/v1/tag/order-tag-list":                    true,
    "POST /api/v1/transaction/import-transaction-list":   true,
    "POST /api/v1/transaction/update-transaction":        true,
    "POST /api/v1/user/complete-onboarding":              true,
    "POST /api/v1/user/logout-user":                      true,
    "POST /api/v1/user/register-user":                    true,
    "POST /api/v1/user/update-budget":                    true,
    "POST /api/v1/user/update-currency":                  true,
    "POST /api/v1/user/update-password":                  true,
}

var routePatternRe = regexp.MustCompile(`"((?:GET|POST) /api/v1/[a-z-]+/[a-z-]+)"`)

// registeredRoutes scans the route-registration source files for mux patterns.
// Source-scanning (vs runtime introspection) is deliberate: http.ServeMux does
// not expose its patterns. If registration files move (Phase 2 moves them to
// internal/<feature>/api/), update handlerGlobs — the guard failing loudly on
// zero matches protects against silently scanning nothing.
func registeredRoutes(t *testing.T) map[string]bool {
    t.Helper()
    _, thisFile, _, _ := runtime.Caller(0)
    repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
    handlerGlobs := []string{"internal/ui/handler/*/routes.go", "internal/*/api/routes.go"}
    routes := map[string]bool{}
    for _, g := range handlerGlobs {
        files, err := filepath.Glob(filepath.Join(repoRoot, g))
        if err != nil {
            t.Fatal(err)
        }
        for _, f := range files {
            src, err := os.ReadFile(f)
            if err != nil {
                t.Fatal(err)
            }
            for _, m := range routePatternRe.FindAllStringSubmatch(string(src), -1) {
                routes[m[1]] = true
            }
        }
    }
    if len(routes) == 0 {
        t.Fatal("route scan found nothing — handlerGlobs are stale")
    }
    return routes
}

// TestGuard_EveryRouteHasScenario fails when a registered route has neither a
// catalogue call nor an allowlist entry — so a new endpoint cannot ship
// without landing in the smoke+parity safety net.
func TestGuard_EveryRouteHasScenario(t *testing.T) {
    covered := map[string]bool{}
    for _, sc := range Catalogue() {
        for _, c := range sc.Calls() {
            path := c.Path
            if i := strings.IndexByte(path, '?'); i >= 0 {
                path = path[:i]
            }
            covered[c.Method+" "+path] = true
        }
    }
    var missing []string
    for r := range registeredRoutes(t) {
        if !covered[r] && !missingFromCatalogue[r] {
            missing = append(missing, r)
        }
    }
    if len(missing) > 0 {
        t.Errorf("routes with no catalogue scenario (add one, or allowlist with a plan reference):\n  %s", strings.Join(missing, "\n  "))
    }
    for r := range missingFromCatalogue {
        if covered[r] {
            t.Errorf("route %s now has a scenario — remove it from missingFromCatalogue", r)
        }
    }
}
```

- [ ] **Step 2: Run it.**

Run: `go test ./internal/test/apiparity/ -run TestGuard -v`
Expected: PASS (the 35 gaps are allowlisted; the reverse check confirms none of them secretly has a scenario). If it FAILS listing extra routes, the registered-route count drifted from the plan's baseline — add those routes to the allowlist AND to the matching scenario task below.

- [ ] **Step 3: Commit**

```bash
git add internal/test/apiparity/guard_test.go
git commit -m "test: route-coverage guard with shrinking allowlist (35 gaps)"
```

---

### Task 4: Fixture-builder additions + extended seed

New scenario tasks need seed rows that the builder can't create yet. Calls can't chain (bodies are built before replay), so anything a write-endpoint targets must pre-exist with a fixed id.

**Files:**
- Modify: `internal/test/fixture/entities.go` (4 new builder methods)
- Modify: `internal/test/apiparity/fixture.go` (new constants + seed rows)

**Interfaces:**
- Produces (fixture builder):
  - `func (b *Builder) BudgetFolder(f BudgetFolder) string` — `type BudgetFolder struct { ID, BudgetID, Name string; Position int }`
  - `func (b *Builder) BudgetEnvelope(e BudgetEnvelope) string` — `type BudgetEnvelope struct { ID, BudgetID, Name, Icon string; Archived bool }`
  - `func (b *Builder) EnvelopeCategory(envelopeID, categoryID string)`
  - `func (b *Builder) BudgetAccess(budgetID, userID string, role int, accepted bool)`
- Produces (apiparity constants, all referenced by Tasks 5–11):
  - `OwnerFolder2 = "f0000000-0000-0000-0000-000000000003"` (second account folder, owner)
  - `Budget2 = "b0000000-0000-0000-0000-000000000002"` (second budget, owner — no invites)
  - `BudgetFolder1 = "bf000000-0000-0000-0000-000000000001"` (folder inside Budget)
  - `Envelope1 = "be000000-0000-0000-0000-000000000001"` (envelope inside Budget, linked to CatSalary)
  - `ElementFood = "e0000000-0000-0000-0000-000000000001"` (budgets_elements row: BudgetID=Budget, ExternalID=CatFood, Type=category, Position=0)

- [ ] **Step 1: Write a failing compile-time consumer.** In `internal/test/apiparity/fixture.go`, extend `Seed` (after the existing budget seed) — this won't compile until the builder methods exist:

```go
// Second account folder (replace-folder / order-folder-list targets).
f.Folder(fixture.Folder{ID: OwnerFolder2, UserID: OwnerID, Name: "Spare"})

// Second budget with no invites (grant-access target).
f.Budget(fixture.Budget{ID: Budget2, UserID: OwnerID, CurrencyID: USD, Name: "Second"})

// Budget structure: a folder, an envelope (with a category link), and a
// category element row so move/change-currency/envelope scenarios have fixed
// ids to reference.
f.BudgetFolder(fixture.BudgetFolder{ID: BudgetFolder1, BudgetID: Budget, Name: "Bills"})
f.BudgetEnvelope(fixture.BudgetEnvelope{ID: Envelope1, BudgetID: Budget, Name: "Envelope", Icon: "cart"})
f.EnvelopeCategory(Envelope1, CatSalary)
f.BudgetElement(fixture.BudgetElement{ID: ElementFood, BudgetID: Budget, ExternalID: CatFood, Type: 0, Position: 0})

// Pending (not accepted) budget invite: guest invited to Budget — the
// accept-access and decline-access scenarios each consume it on a fresh DB.
f.BudgetAccess(Budget, GuestID, 2 /* role: user */, false)
```

Run: `go build ./internal/...`
Expected: FAIL — `f.BudgetFolder undefined` etc.

- [ ] **Step 2: Implement the builder methods** in `internal/test/fixture/entities.go`, following the existing `BudgetElement` style (schema: `budgets_folders(id,budget_id,name,position,created_at,updated_at)`, `budgets_envelopes(id,budget_id,name,icon,is_archived,created_at,updated_at)`, `budgets_envelopes_categories(budget_envelope_id,category_id)`, `budgets_access(budget_id,user_id,role,is_accepted,created_at,updated_at)`):

```go
// BudgetFolder describes a budgets_folders row.
type BudgetFolder struct {
    ID       string
    BudgetID string
    Name     string
    Position int
}

func (b *Builder) BudgetFolder(f BudgetFolder) string {
    b.t.Helper()
    id := b.orNewID(f.ID)
    if f.Name == "" {
        f.Name = "Folder"
    }
    now := b.now()
    b.insert(`INSERT INTO budgets_folders (id, budget_id, name, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
        id, f.BudgetID, f.Name, f.Position, now, now)
    return id
}

// BudgetEnvelope describes a budgets_envelopes row.
type BudgetEnvelope struct {
    ID       string
    BudgetID string
    Name     string
    Icon     string
    Archived bool
}

func (b *Builder) BudgetEnvelope(e BudgetEnvelope) string {
    b.t.Helper()
    id := b.orNewID(e.ID)
    now := b.now()
    b.insert(`INSERT INTO budgets_envelopes (id, budget_id, name, icon, is_archived, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
        id, e.BudgetID, nullable(e.Name), nullable(e.Icon), e.Archived, now, now)
    return id
}

func (b *Builder) EnvelopeCategory(envelopeID, categoryID string) {
    b.t.Helper()
    b.insert(`INSERT INTO budgets_envelopes_categories (budget_envelope_id, category_id) VALUES (?, ?)`,
        envelopeID, categoryID)
}

// BudgetAccess grants userID access to budgetID. role uses the stored SMALLINT
// (see internal/domain/budget/valueobject.go); accepted=false models a pending invite.
func (b *Builder) BudgetAccess(budgetID, userID string, role int, accepted bool) {
    b.t.Helper()
    now := b.now()
    b.insert(`INSERT INTO budgets_access (budget_id, user_id, role, is_accepted, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
        budgetID, userID, role, accepted, now, now)
}
```

Before committing, verify the role integer for "user" against `internal/domain/budget/valueobject.go` and adjust the `Seed` call's comment/value if it differs.

- [ ] **Step 3: Regenerate goldens (seed changed → read-endpoint responses changed), inspect, run.**

Run: `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ && git diff --stat internal/test/apiparity/testdata/`
Expected: diffs only in scenarios that read budget/folder/account lists — inspect one diff to confirm it's only the new seed rows appearing.

Run: `make test`
Expected: PASS.

Run: `CGO_ENABLED=0 go test -count=1 -tags enginecompare ./internal/test/enginecompare/ -run TestAPIParity 2>&1 | tail -3`
Expected: PASS (sqlite; pgsql skips or passes).

- [ ] **Step 4: Commit**

```bash
git add internal/test/fixture internal/test/apiparity
git commit -m "test(fixture): budget folder/envelope/access builders + extended API seed"
```

---

### Task 5: User-module scenarios (6 routes)

**Files:**
- Create: `internal/test/apiparity/catalogue_user.go`
- Modify: `internal/test/apiparity/guard_test.go` (remove 7 allowlist entries)

**Interfaces:** Consumes Task 1's `register`/`Call`, Task 4's constants.

- [ ] **Step 1: Add the scenarios.** Request shapes verified against the handlers' DTO `Validate()`:

```go
// catalogue_user.go
package apiparity

func init() {
    register(Scenario{Name: "user_writes", Calls: func() []Call {
        return []Call{
            // Public registration; returns the created user WITHOUT a token (frozen contract).
            {Label: "register-user", Method: "POST", Path: "/api/v1/user/register-user", Auth: "",
                Body: map[string]any{"email": "newuser@example.test", "password": SeedPassword, "name": "Newbie"}},
            {Label: "complete-onboarding", Method: "POST", Path: "/api/v1/user/complete-onboarding", Auth: "owner"}, // no body
            {Label: "update-currency", Method: "POST", Path: "/api/v1/user/update-currency", Auth: "owner",
                Body: map[string]any{"currency": "USD"}},
            // Field name is "value" (a budget id) — frozen quirk.
            {Label: "update-budget", Method: "POST", Path: "/api/v1/user/update-budget", Auth: "owner",
                Body: map[string]any{"value": Budget}},
            // JWT is stateless: token stays usable after both of these.
            {Label: "update-password", Method: "POST", Path: "/api/v1/user/update-password", Auth: "owner",
                Body: map[string]any{"oldPassword": SeedPassword, "newPassword": "new-secret-pw"}},
            {Label: "logout-user", Method: "POST", Path: "/api/v1/user/logout-user", Auth: "owner"}, // pins the frozen {"result":"test"} quirk
            {Label: "get-user-data-after", Method: "POST", Path: "/api/v1/user/get-user-data", Auth: "owner", Body: map[string]any{}},
        }
    }})
}
```

- [ ] **Step 2: Remove from the allowlist** in `guard_test.go`: the 6 `/api/v1/user/…` entries (`register-user`, `complete-onboarding`, `update-currency`, `update-budget`, `update-password`, `logout-user`).

- [ ] **Step 3: Generate goldens, verify statuses, run guard.**

Run: `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ -run 'TestSmoke_Catalogue/user_writes' -v`
Then: `go test ./internal/test/apiparity/ -v -run 'TestSmoke_Catalogue/user_writes|TestGuard'`
Expected: PASS. Every non-`err:` call 2xx — if any 4xx, the request body is wrong: fix the body, do NOT golden a failure. Inspect `testdata/golden/user_writes.golden`: registration returns a user object and NO token; logout data is `{"result":"test"}`.

- [ ] **Step 4: Full suites.**

Run: `make test && CGO_ENABLED=0 go test -count=1 -tags enginecompare ./internal/test/enginecompare/ -run TestAPIParity 2>&1 | tail -3`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/test/apiparity
git commit -m "test(apiparity): user-module write scenarios (7 routes)"
```

---

### Task 6: Account-module scenarios (4 routes)

**Files:**
- Create: `internal/test/apiparity/catalogue_account.go`
- Modify: `internal/test/apiparity/guard_test.go` (remove 4 entries)

- [ ] **Step 1: Add the scenario.** Order matters: `replace-folder` deletes `OwnerFolder2`, so it goes LAST.

```go
// catalogue_account.go
package apiparity

func init() {
    register(Scenario{Name: "account_folder_writes", Calls: func() []Call {
        return []Call{
            {Label: "order-account-list", Method: "POST", Path: "/api/v1/account/order-account-list", Auth: "owner",
                Body: map[string]any{"changes": []map[string]any{{"id": OwnerAccount, "folderId": OwnerFolder, "position": 0}}}},
            {Label: "order-folder-list", Method: "POST", Path: "/api/v1/account/order-folder-list", Auth: "owner",
                Body: map[string]any{"changes": []map[string]any{{"id": OwnerFolder, "position": 0}, {"id": OwnerFolder2, "position": 1}}}},
            {Label: "show-folder", Method: "POST", Path: "/api/v1/account/show-folder", Auth: "owner",
                Body: map[string]any{"id": OwnerFolder2}},
            // Moves OwnerFolder2's accounts into OwnerFolder and DELETES OwnerFolder2.
            {Label: "replace-folder", Method: "POST", Path: "/api/v1/account/replace-folder", Auth: "owner",
                Body: map[string]any{"id": OwnerFolder2, "replaceId": OwnerFolder}},
            {Label: "get-account-list-after", Method: "GET", Path: "/api/v1/account/get-account-list", Auth: "owner"},
        }
    }})
}
```

- [ ] **Step 2: Remove the 4 `/api/v1/account/…` allowlist entries.**

- [ ] **Step 3: Goldens + suites** (same commands as Task 5 Step 3–4 with `account_folder_writes`).
Expected: PASS, all non-`err:` calls 2xx.

- [ ] **Step 4: Commit**

```bash
git add internal/test/apiparity
git commit -m "test(apiparity): account folder write scenarios (4 routes)"
```

---

### Task 7: Order-list scenarios — category, tag, payee (3 routes)

**Files:**
- Create: `internal/test/apiparity/catalogue_orderlists.go`
- Modify: `internal/test/apiparity/guard_test.go` (remove 3 entries)

- [ ] **Step 1: Add the scenario.**

```go
// catalogue_orderlists.go
package apiparity

func init() {
    register(Scenario{Name: "order_lists", Calls: func() []Call {
        return []Call{
            {Label: "order-category-list", Method: "POST", Path: "/api/v1/category/order-category-list", Auth: "owner",
                Body: map[string]any{"changes": []map[string]any{{"id": CatFood, "position": 1}, {"id": CatSalary, "position": 0}}}},
            {Label: "get-category-list-after", Method: "POST", Path: "/api/v1/category/get-category-list", Auth: "owner", Body: map[string]any{}},
            {Label: "order-tag-list", Method: "POST", Path: "/api/v1/tag/order-tag-list", Auth: "owner",
                Body: map[string]any{"changes": []map[string]any{{"id": TagWork, "position": 0}}}},
            {Label: "order-payee-list", Method: "POST", Path: "/api/v1/payee/order-payee-list", Auth: "owner",
                Body: map[string]any{"changes": []map[string]any{{"id": PayeeShop, "position": 0}}}},
        }
    }})
}
```

- [ ] **Step 2: Remove the 3 allowlist entries** (`category/order-category-list`, `tag/order-tag-list`, `payee/order-payee-list`).

- [ ] **Step 3: Goldens + suites** (as Task 5 Steps 3–4, scenario `order_lists`). Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/test/apiparity
git commit -m "test(apiparity): category/tag/payee order-list scenarios (3 routes)"
```

---

### Task 8: Connection-module scenarios (2 routes)

**Files:**
- Create: `internal/test/apiparity/catalogue_connection.go`
- Modify: `internal/test/apiparity/guard_test.go` (remove 2 entries)

- [ ] **Step 1: Add the scenario.** Owner and guest are already `Connect`ed in the seed; owner manages access to their OWN account:

```go
// catalogue_connection.go
package apiparity

func init() {
    register(Scenario{Name: "connection_access_writes", Calls: func() []Call {
        return []Call{
            // role enum: "admin" | "user" | "guest" (owner is not an input role).
            {Label: "set-account-access", Method: "POST", Path: "/api/v1/connection/set-account-access", Auth: "owner",
                Body: map[string]any{"accountId": OwnerAccount, "userId": GuestID, "role": "user"}},
            {Label: "get-connection-list-after-set", Method: "GET", Path: "/api/v1/connection/get-connection-list", Auth: "owner"},
            {Label: "revoke-account-access", Method: "POST", Path: "/api/v1/connection/revoke-account-access", Auth: "owner",
                Body: map[string]any{"accountId": OwnerAccount, "userId": GuestID}},
        }
    }})
}
```

If `get-connection-list` is not the exact read route name, check `internal/ui/handler/connection/routes.go` and use a registered GET from there (any already-covered read is fine — its purpose is to pin post-write state).

- [ ] **Step 2: Remove the 2 allowlist entries.**

- [ ] **Step 3: Goldens + suites** (scenario `connection_access_writes`). Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/test/apiparity
git commit -m "test(apiparity): connection access write scenarios (2 routes)"
```

---

### Task 9: Transaction-module scenarios (3 routes: update, CSV export, multipart import)

**Files:**
- Create: `internal/test/apiparity/catalogue_transaction.go`
- Modify: `internal/test/apiparity/guard_test.go` (remove 3 entries)

- [ ] **Step 1: Add the scenarios.** Export returns `text/csv` (no JSON envelope) — normalization still applies. Import is multipart, using `RawBody`:

```go
// catalogue_transaction.go
package apiparity

import (
    "bytes"
    "mime/multipart"
)

func init() {
    register(Scenario{Name: "transaction_writes", Calls: func() []Call {
        return []Call{
            {Label: "update-transaction", Method: "POST", Path: "/api/v1/transaction/update-transaction", Auth: "owner",
                Body: map[string]any{
                    "id": Txn1, "type": "expense", "amount": "15.00",
                    "accountId": OwnerAccount, "categoryId": CatFood,
                    "date": ClockTime.Format("2006-01-02 15:04:05"),
                    "description": "lunch updated", "payeeId": PayeeShop,
                }},
            {Label: "export-csv", Method: "GET", Path: "/api/v1/transaction/export-transaction-list?accountId=" + OwnerAccount, Auth: "owner"},
        }
    }})

    register(Scenario{Name: "transaction_import", Calls: func() []Call {
        body, ctype := buildImportBody()
        return []Call{
            {Label: "import-transaction-list", Method: "POST", Path: "/api/v1/transaction/import-transaction-list",
                Auth: "owner", RawBody: body, ContentType: ctype},
            {Label: "get-transaction-list-after", Method: "GET", Path: "/api/v1/transaction/get-transaction-list?accountId=" + OwnerAccount, Auth: "owner"},
        }
    }})
}

// buildImportBody assembles the multipart form the import endpoint expects:
// a CSV file, a column-mapping JSON, and a fixed accountId override.
func buildImportBody() ([]byte, string) {
    var buf bytes.Buffer
    w := multipart.NewWriter(&buf)
    fw, err := w.CreateFormFile("file", "import.csv")
    if err != nil {
        panic(err)
    }
    fw.Write([]byte("Date,Amount,Description,Category\n2026-01-15,12.34,coffee,Food\n"))
    w.WriteField("mapping", `{"date":"Date","amount":"Amount","description":"Description","category":"Category"}`)
    w.WriteField("accountId", OwnerAccount)
    w.Close()
    return buf.Bytes(), w.FormDataContentType()
}
```

If `get-transaction-list` takes different query params, check `internal/ui/handler/transaction/routes.go` + its handler and use the registered read with correct params.

- [ ] **Step 2: Remove the 3 allowlist entries.**

- [ ] **Step 3: Goldens + suites.** Inspect `transaction_import.golden`: the import response must show `"imported":1,"skipped":0` — an import that silently imported 0 rows is a WRONG golden; fix the CSV/mapping until a row lands. Inspect `transaction_writes.golden`: export body is CSV (dates redacted), update returns the transaction plus refreshed account balances.
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/test/apiparity
git commit -m "test(apiparity): transaction update/export/import scenarios (3 routes)"
```

---

### Task 10: Budget structure scenarios (9 routes)

**Files:**
- Create: `internal/test/apiparity/catalogue_budget_structure.go`
- Modify: `internal/test/apiparity/guard_test.go` (remove 9 entries)

- [ ] **Step 1: Add the scenario.** Create-endpoints take a client-supplied `id` used as the idempotency/external key; fresh literal (non-v7) UUIDs keep goldens strict:

```go
// catalogue_budget_structure.go
package apiparity

func init() {
    register(Scenario{Name: "budget_structure_writes", Calls: func() []Call {
        const (
            newFolder   = "bf000000-0000-0000-0000-0000000000aa"
            newEnvelope = "be000000-0000-0000-0000-0000000000aa"
        )
        return []Call{
            {Label: "create-folder", Method: "POST", Path: "/api/v1/budget/create-folder", Auth: "owner",
                Body: map[string]any{"budgetId": Budget, "id": newFolder, "name": "Bills"}},
            {Label: "update-folder", Method: "POST", Path: "/api/v1/budget/update-folder", Auth: "owner",
                Body: map[string]any{"budgetId": Budget, "id": newFolder, "name": "Bills 2"}},
            {Label: "order-folder-list", Method: "POST", Path: "/api/v1/budget/order-folder-list", Auth: "owner",
                Body: map[string]any{"budgetId": Budget, "items": []map[string]any{{"id": BudgetFolder1, "position": 0}, {"id": newFolder, "position": 1}}}},
            {Label: "create-envelope", Method: "POST", Path: "/api/v1/budget/create-envelope", Auth: "owner",
                Body: map[string]any{"budgetId": Budget, "id": newEnvelope, "name": "Groceries", "icon": "cart",
                    "currencyId": USD, "folderId": newFolder, "categories": []string{}}},
            {Label: "update-envelope", Method: "POST", Path: "/api/v1/budget/update-envelope", Auth: "owner",
                Body: map[string]any{"budgetId": Budget, "id": Envelope1, "name": "Envelope 2", "icon": "cart",
                    "currencyId": USD, "isArchived": 0, "categories": []string{CatSalary}}},
            {Label: "move-element-list", Method: "POST", Path: "/api/v1/budget/move-element-list", Auth: "owner",
                Body: map[string]any{"budgetId": Budget, "items": []map[string]any{{"id": CatFood, "folderId": BudgetFolder1, "position": 0}}}},
            {Label: "change-element-currency", Method: "POST", Path: "/api/v1/budget/change-element-currency", Auth: "owner",
                Body: map[string]any{"budgetId": Budget, "elementId": CatFood, "currencyId": USD}},
            {Label: "delete-envelope", Method: "POST", Path: "/api/v1/budget/delete-envelope", Auth: "owner",
                Body: map[string]any{"budgetId": Budget, "id": Envelope1}},
            {Label: "delete-folder", Method: "POST", Path: "/api/v1/budget/delete-folder", Auth: "owner",
                Body: map[string]any{"budgetId": Budget, "id": newFolder}},
            {Label: "get-budget-after", Method: "GET", Path: "/api/v1/budget/get-budget?id=" + Budget, Auth: "owner"},
        }
    }})
}
```

Two verify-before-golden points: (a) `move-element-list`/`change-element-currency` identify elements by EXTERNAL id — the plan assumes the category id works; if a call 400s, check `internal/app/budget/move.go` for the expected identifier and adjust. (b) `get-budget`'s exact query params: copy from the already-covered read scenario in `catalogue.go`.

- [ ] **Step 2: Remove the 9 allowlist entries** (`create-folder`, `update-folder`, `delete-folder`, `order-folder-list`, `create-envelope`, `update-envelope`, `delete-envelope`, `move-element-list`, `change-element-currency`).

- [ ] **Step 3: Goldens + suites.** All non-`err:` calls 2xx; the closing `get-budget-after` pins the resulting structure. Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/test/apiparity
git commit -m "test(apiparity): budget structure write scenarios (9 routes)"
```

---

### Task 11: Budget access & account scenarios (8 routes)

**Files:**
- Create: `internal/test/apiparity/catalogue_budget_access.go`
- Modify: `internal/test/apiparity/guard_test.go` (remove 8 entries)

- [ ] **Step 1: Add three scenarios** (fresh DB per scenario, so the single seeded pending invite serves accept and decline separately):

```go
// catalogue_budget_access.go
package apiparity

func init() {
    register(Scenario{Name: "budget_access_accept", Calls: func() []Call {
        return []Call{
            // Guest accepts the seeded pending invite to Budget…
            {Label: "accept-access", Method: "POST", Path: "/api/v1/budget/accept-access", Auth: "guest",
                Body: map[string]any{"budgetId": Budget}},
            // …then owner revokes it again.
            {Label: "revoke-access", Method: "POST", Path: "/api/v1/budget/revoke-access", Auth: "owner",
                Body: map[string]any{"budgetId": Budget, "userId": GuestID}},
            // Owner grants guest access to the invite-free second budget.
            // role enum: "admin" | "user" | "guest".
            {Label: "grant-access", Method: "POST", Path: "/api/v1/budget/grant-access", Auth: "owner",
                Body: map[string]any{"budgetId": Budget2, "userId": GuestID, "role": "user"}},
        }
    }})

    register(Scenario{Name: "budget_access_decline", Calls: func() []Call {
        return []Call{
            {Label: "decline-access", Method: "POST", Path: "/api/v1/budget/decline-access", Auth: "guest",
                Body: map[string]any{"budgetId": Budget}},
        }
    }})

    register(Scenario{Name: "budget_account_writes", Calls: func() []Call {
        period := ClockTime.Format("2006-01") + "-01"
        return []Call{
            // Field-name quirk (frozen): include/exclude carry the budget id as "id".
            {Label: "exclude-account", Method: "POST", Path: "/api/v1/budget/exclude-account", Auth: "owner",
                Body: map[string]any{"id": Budget, "accountId": OwnerAccount}},
            {Label: "include-account", Method: "POST", Path: "/api/v1/budget/include-account", Auth: "owner",
                Body: map[string]any{"id": Budget, "accountId": OwnerAccount}},
            {Label: "reset-budget", Method: "POST", Path: "/api/v1/budget/reset-budget", Auth: "owner",
                Body: map[string]any{"id": Budget, "startedAt": period}},
            // Exactly-one-selector rule: categoryId alone is a valid mode.
            {Label: "get-transaction-list", Method: "GET",
                Path: "/api/v1/budget/get-transaction-list?budgetId=" + Budget + "&periodStart=" + period + "&categoryId=" + CatFood,
                Auth: "owner"},
        }
    }})
}
```

- [ ] **Step 2: Remove the 8 allowlist entries** (`accept-access`, `decline-access`, `grant-access`, `revoke-access`, `include-account`, `exclude-account`, `reset-budget`, `GET …/get-transaction-list`).

- [ ] **Step 3: Goldens + suites.** `get-transaction-list` must return Txn1 (non-empty items) — an empty list means the period/selector is wrong, not golden-worthy. Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/test/apiparity
git commit -m "test(apiparity): budget access + account scenarios (8 routes)"
```

---

### Task 12: Delete the allowlist; full-catalogue regression checkpoint

**Files:**
- Modify: `internal/test/apiparity/guard_test.go`

- [ ] **Step 1: Verify the allowlist is empty, then delete it.** Remove the `missingFromCatalogue` var, its reverse-check loop, and the `!missingFromCatalogue[r]` condition. The guard now hard-fails on ANY route without a scenario.

- [ ] **Step 2: Run everything, including the two-engine parity over the full catalogue.**

Run: `make test`
Expected: PASS, guard strict.

Run: `make regression`
Expected: PASS — all 84 routes now compared sqlite-vs-PostgreSQL. If a parity mismatch surfaces on a newly covered route, that is a REAL pre-existing engine bug: do not mask it in the test; report it, file it as its own fix commit before proceeding.

- [ ] **Step 3: Commit**

```bash
git add internal/test/apiparity/guard_test.go
git commit -m "test(apiparity): guard now strict — every route has a scenario"
```

---

### Task 13: Storage sqlite backend tests

`internal/infra/storage/sqlite` is 0% because tests open DBs via dbtest, never through the production `backend.Open` boot path.

**Files:**
- Create: `internal/infra/storage/sqlite/sqlite_test.go`

- [ ] **Step 1: Read `internal/infra/storage/sqlite/sqlite.go` fully** (≈100 lines): note `normalizeDSN`'s exact accepted forms and how the busy-timeout is configured (field/setter). Cover whatever configuration hook exists.

- [ ] **Step 2: Write the tests** (in-package, so unexported `normalizeDSN` is reachable):

```go
package sqlite

import (
    "context"
    "path/filepath"
    "testing"
)

func TestNormalizeDSN(t *testing.T) {
    cases := []struct{ in, want string }{
        {"sqlite:///abs/path/db.sqlite", "/abs/path/db.sqlite"},
        {"/plain/path/db.sqlite", "/plain/path/db.sqlite"},
        // Extend from the forms normalizeDSN's own code/comments accept.
    }
    for _, c := range cases {
        if got := normalizeDSN(c.in); got != c.want {
            t.Errorf("normalizeDSN(%q) = %q, want %q", c.in, got, c.want)
        }
    }
}

func TestOpen_PragmasAndPing(t *testing.T) {
    b := New()
    db, err := b.Open(context.Background(), "sqlite://"+filepath.Join(t.TempDir(), "t.sqlite"))
    if err != nil {
        t.Fatalf("Open: %v", err)
    }
    defer db.Close()
    var fk int
    if err := db.QueryRow("PRAGMA foreign_keys;").Scan(&fk); err != nil {
        t.Fatal(err)
    }
    if fk != 1 {
        t.Errorf("foreign_keys = %d, want 1", fk)
    }
}
```

Adjust the `"sqlite://…"` DSN form in `TestOpen` to whichever form Step 1 found `normalizeDSN` accepts for absolute paths, and add a busy-timeout assertion (`PRAGMA busy_timeout;`) if the backend exposes a way to set it.

- [ ] **Step 3: Run.**

Run: `go test ./internal/infra/storage/sqlite/ -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/infra/storage/sqlite
git commit -m "test(storage): cover the sqlite backend Open path and DSN normalization"
```

---

### Task 14: CLI command tests

`internal/cli` is at 33%; `cli.Run(args) int` is directly callable. Commands read config from env — `t.Setenv` gives each test an isolated sqlite DB and JWT dir.

**Files:**
- Create: `internal/cli/commands_test.go` (keep the existing `cli_test.go` untouched)

- [ ] **Step 1: Write the lifecycle test.**

```go
package cli

import (
    "os"
    "path/filepath"
    "testing"
)

// cliEnv points the container at an isolated sqlite DB + JWT dir. The container
// runs migrations on open (same as serve), so commands work on a fresh file.
func cliEnv(t *testing.T) {
    t.Helper()
    dir := t.TempDir()
    t.Setenv("DATABASE_URL", "sqlite://"+filepath.Join(dir, "db.sqlite"))
    t.Setenv("ECONUMO_JWT_PRIVATE_KEY_PATH", filepath.Join(dir, "jwt", "private.pem"))
    t.Setenv("ECONUMO_JWT_PUBLIC_KEY_PATH", filepath.Join(dir, "jwt", "public.pem"))
    t.Setenv("ECONUMO_JWT_PASSPHRASE", "test-pass")
}

func TestUserCommandLifecycle(t *testing.T) {
    cliEnv(t)
    steps := []struct {
        name string
        args []string
        want int
    }{
        {"create", []string{"user:create", "Test User", "cli@example.test", "secret-pw"}, 0},
        {"create-duplicate", []string{"user:create", "Test User", "cli@example.test", "secret-pw"}, 1},
        {"change-password", []string{"user:change-password", "cli@example.test", "new-pw"}, 0},
        {"change-email", []string{"user:change-email", "cli@example.test", "cli2@example.test"}, 0},
        {"activate", []string{"user:activate", "cli2@example.test"}, 0},
        {"change-password-unknown", []string{"user:change-password", "nobody@example.test", "x"}, 1},
    }
    for _, s := range steps {
        if got := Run(s.args); got != s.want {
            t.Fatalf("%s: Run(%v) = %d, want %d", s.name, s.args, got, s.want)
        }
    }
}

func TestJwtGenerate(t *testing.T) {
    cliEnv(t)
    if got := Run([]string{"jwt:generate"}); got != 0 {
        t.Fatalf("jwt:generate = %d, want 0", got)
    }
    if _, err := os.Stat(os.Getenv("ECONUMO_JWT_PRIVATE_KEY_PATH")); err != nil {
        t.Errorf("private key not written: %v", err)
    }
}

func TestCurrencyAdd(t *testing.T) {
    cliEnv(t)
    if got := Run([]string{"currency:add", "XTS", "Test Currency", "2"}); got != 0 {
        t.Fatalf("currency:add = %d, want 0", got)
    }
}

func TestDataRemoveSaltRefusesEmptySalt(t *testing.T) {
    cliEnv(t)
    t.Setenv("ECONUMO_DATA_SALT", "")
    if got := Run([]string{"data:remove-salt"}); got == 0 {
        t.Error("data:remove-salt must refuse to run with an empty salt")
    }
}

func TestQuietFlag(t *testing.T) {
    cliEnv(t)
    if got := Run([]string{"-q", "user:create", "Quiet User", "quiet@example.test", "secret-pw"}); got != 0 {
        t.Fatalf("-q user:create = %d, want 0", got)
    }
}
```

- [ ] **Step 2: Run; fix expectations against reality.**

Run: `go test ./internal/cli/ -v`
Expected: PASS. Two assumptions to verify on failure: (a) the container migrates a fresh DB on open — if not, the first `user:create` fails; then open the DB once via `dbtest`-style migration inside `cliEnv` instead. (b) exact exit codes for error paths — read `cli.go`'s `Run` for the code it returns on command error and match it.

- [ ] **Step 3: Check coverage moved.**

Run: `go test ./internal/cli/ -cover`
Expected: ≥70% (was 32.8% cross-package / 19.6% self).

- [ ] **Step 4: Commit**

```bash
git add internal/cli/commands_test.go
git commit -m "test(cli): cover command lifecycle, jwt:generate, currency:add, salt refusal"
```

---

### Task 15: Negative-path catalogue scenarios (validation + authorization contract)

Validation strings and the 401 envelope are frozen contract; goldens must pin them. This also lifts the weaker handler packages (`account` 69.7%, `category` 71.3%) toward the ≥85% target by exercising their error branches.

**Files:**
- Create: `internal/test/apiparity/catalogue_negative.go`

- [ ] **Step 1: Add the scenario.** All labels carry the `err:` prefix (guard-exempt from the 2xx rule; these paths are already covered by positive scenarios, so no allowlist changes):

```go
// catalogue_negative.go
package apiparity

func init() {
    register(Scenario{Name: "negative_paths", Calls: func() []Call {
        return []Call{
            // 401 envelope: no token on a protected route.
            {Label: "err:unauthenticated", Method: "POST", Path: "/api/v1/category/get-category-list", Auth: "", Body: map[string]any{}},
            // Tier-1 validation messages (frozen strings).
            {Label: "err:category-name-blank", Method: "POST", Path: "/api/v1/category/create-category", Auth: "owner",
                Body: map[string]any{"id": "c0000000-0000-0000-0000-0000000000ee", "name": "", "type": "expense", "icon": "i"}},
            {Label: "err:category-name-too-short", Method: "POST", Path: "/api/v1/category/create-category", Auth: "owner",
                Body: map[string]any{"id": "c0000000-0000-0000-0000-0000000000ef", "name": "ab", "type": "expense", "icon": "i"}},
            {Label: "err:order-empty-changes", Method: "POST", Path: "/api/v1/category/order-category-list", Auth: "owner",
                Body: map[string]any{"changes": []map[string]any{}}},
            {Label: "err:account-order-empty", Method: "POST", Path: "/api/v1/account/order-account-list", Auth: "owner",
                Body: map[string]any{"changes": []map[string]any{}}},
            {Label: "err:replace-folder-blank", Method: "POST", Path: "/api/v1/account/replace-folder", Auth: "owner",
                Body: map[string]any{"id": "", "replaceId": ""}},
            // Authorization: guest may not mutate owner's resources.
            {Label: "err:guest-updates-owner-account", Method: "POST", Path: "/api/v1/account/update-account", Auth: "guest",
                Body: map[string]any{"id": OwnerAccount, "name": "Hacked", "currencyId": USD}},
            {Label: "err:update-budget-bad-uuid", Method: "POST", Path: "/api/v1/user/update-budget", Auth: "owner",
                Body: map[string]any{"value": "not-a-uuid"}},
            {Label: "err:update-tx-bad-type", Method: "POST", Path: "/api/v1/transaction/update-transaction", Auth: "owner",
                Body: map[string]any{"id": Txn1, "type": "bogus", "amount": "1.00", "accountId": OwnerAccount,
                    "date": ClockTime.Format("2006-01-02 15:04:05")}},
        }
    }})
}
```

Before generating goldens, check `internal/ui/handler/account/update…`'s request shape for `update-account` (field names) so the guest-authorization call fails on AUTHORIZATION (403/400 with an access message), not on decode.

- [ ] **Step 2: Goldens + inspect.** Every response must be a 4xx with the error envelope (`"success":false`, `errors` object). Confirm the frozen strings appear verbatim (e.g. `"Category name must be 3-64 characters"`, `"This value should not be blank."`).

Run: `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ -run 'TestSmoke_Catalogue/negative_paths' -v && go test ./internal/test/apiparity/`
Expected: PASS.

- [ ] **Step 3: Handler-coverage check.**

Run: `CGO_ENABLED=0 go test -count=1 ./... -coverpkg=./internal/...,./pkg/... -coverprofile=/tmp/p0.out >/dev/null 2>&1; go tool cover -func=/tmp/p0.out | grep -E 'ui/handler/(account|category)/' | awk '{s+=$NF; n++} END {print s/n}'`
Expected: ≥85 for each of the two packages (compute per package). If short, list that package's remaining 0% functions (`go tool cover -func=/tmp/p0.out | grep 'ui/handler/account' | awk '$NF=="0.0%"'`) and add one `err:` or positive call per uncovered endpoint branch to this scenario, regenerate goldens.

- [ ] **Step 4: Commit**

```bash
git add internal/test/apiparity
git commit -m "test(apiparity): negative-path scenarios pinning validation and auth contract"
```

---

### Task 16: Ratchet the coverage gate; phase-boundary checkpoint

**Files:**
- Modify: `Makefile` (the `GO_COVER_MIN ?= 64` line, currently line 79)
- Modify: `.github/workflows/go-tests.yml` ONLY if it hardcodes its own GO_COVER_MIN (check first)

- [ ] **Step 1: Measure.**

Run: `make test 2>&1 | grep 'total coverage'`
Expected: a number ≥74% (baseline 68.0% + smoke harness + scenarios + cli/storage tests).

- [ ] **Step 2: Ratchet.** Set the gate to (measured − 2), but not below 72:

```makefile
GO_COVER_MIN ?= 72
```

(replace 72 with the computed value). Check the CI workflow: `grep -n GO_COVER_MIN .github/workflows/go-tests.yml` — if it sets its own value, align it; if it just calls `make test`, no change.

- [ ] **Step 3: Repo-package target check (spec: repo packages ≥75%).**

Run: `go tool cover -func=coverage.out | awk -F'[:\t]+' 'NF>2 && $1!="total" {n=split($1,p,"/"); pkg=""; for(i=1;i<n;i++) pkg=pkg p[i] "/"; cov=$NF; gsub("%","",cov); sum[pkg]+=cov; cnt[pkg]++} END {for (k in sum) printf "%5.1f%% %s\n", sum[k]/cnt[k], k}' | grep 'infra/repo' | sort -n`

(`coverage.out` is left behind by `make test`.) Note: the pgsql adapter shims in each repo (`pgsql.go`) legitimately show 0% in this sqlite-only profile — they are exercised by `make regression`. Judge each package by its sqlite/shared files: for any repo whose non-pgsql functions include 0% entries (list them with `go tool cover -func=coverage.out | grep 'infra/repo/<name>' | awk '$NF=="0.0%"'`), add an integration test case to that repo's existing `*_integration_test.go` exercising the uncovered method against `dbtest.NewSQLite` with the fixture builder — same table-driven style as the file's existing cases. Commit those separately as `test(repo): cover remaining <name> repository methods`.

- [ ] **Step 4: Full verification.**

Run: `make test`
Expected: PASS with the new gate.

Run: `make regression`
Expected: PASS.

- [ ] **Step 5: Commit, and close the phase.**

```bash
git add Makefile
git commit -m "test: ratchet GO_COVER_MIN to <value> after Phase 0 test investment"
```

Phase 0 is complete. Per the spec's merge cadence: merge `refactor/feature-packages` into `golang` now (fast-forward or merge commit per repo habit), before Phase 1 planning starts.

---

## Verification checklist (end of phase)

- [ ] `make test` green with the raised gate; smoke suite replays ALL scenarios with goldens.
- [ ] `make regression` green; parity suite covers all 84 routes on both engines.
- [ ] Guard test is strict (no allowlist).
- [ ] `git log --oneline` shows ~16 small commits, each independently green.
- [ ] Cross-package coverage ≥74% measured; gate at measured−2.
- [ ] Branch merged into `golang`.
