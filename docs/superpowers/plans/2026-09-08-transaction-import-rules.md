# Transaction Import — Stage 4 (Rules) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add import rules — per-user classify/skip patterns that fire inside the import pipeline, a "make this a rule?" prompt after editing an imported transaction, a rules management page, preview/apply against existing imports, and optional LLM rule suggestions.

**Architecture:** Rules live in the existing `imports` feature (`internal/imports`) as a new `import_rules` table (the stage-1 placeholder is dropped and recreated to the spec shape) plus the two label join tables that already exist. The pipeline loads the user's rule set once per operation and consults it in `resolve` (skip rules, after currency conversion, before the matcher) and `place` (classify rules on create; adopted candidates snapshot their own values), recording what was applied on the ledger row (`applied_*`, `import_link_applied_labels`). The HTTP edge gains seven routes; rule detection/diffing is client-side over the ledger row's `applied*` fields; `suggest-rules` calls an OpenAI-compatible chat endpoint configured by `ECONUMO_AI_DSN` and is disabled when unset.

**Tech Stack:** Go 1.27 (stdlib HTTP, sqlc, modernc sqlite / pgx), React 19 + Vite + TanStack Query + Zustand + vitest/msw, i18n catalogues in `locales/`.

**Spec:** `docs/superpowers/specs/2026-08-15-transaction-import-design.md` — Part 2 (`import_rules`, skip rules, `import_rule_labels`, `import_link_applied_labels`), Part 4/5 (pipeline stage 3), Part 7 (rule prompt, apply scope, LLM suggestions, `ECONUMO_AI_DSN`), Part 8 (rule endpoints, rate limiting), Part 9 (rules management UI).

**Base branch:** `feature/transaction-import` (NOT `main`). **Working branch:** `feature/transaction-import-rules`. The PR at the end targets `feature/transaction-import`.

## Global Constraints

- Features never import features (`internal/test/archtest`): everything the `imports` feature needs from `transaction`/`category`/`payee`/`tag` comes through `internal/imports/ports.go` interfaces wired in `internal/server/glue_imports.go`.
- Every new SQL query is written for BOTH engines (`internal/infra/storage/sqlc/query/{sqlite,pgsql}/imports.sql`) with the terminating `;` on the SAME line as the statement, then `sqlc generate` (config `internal/infra/storage/sqlc/sqlc.yaml`) and the generated `imports.sql.go` in BOTH `gen/sqlite` and `gen/pgsql` is inspected for truncated statements. Migrations are paired: `internal/infra/storage/migrations/{sqlite,pgsql}/20260908000000.sql`.
- SQLite is the reference engine; PostgreSQL must match byte-for-byte (`make test` runs `enginecompare`). Decimal assertions in tests compare normalized `vo.Decimal` values.
- The wire contract is frozen: `POST` for writes, `GET` for reads, `/api/v1/import/<verb>-<subject>`, the response envelope, `"2006-01-02 15:04:05"` datetimes, `""` for NULL ids. `isCaseSensitive` is a plain JSON bool in both requests and results (a design ruling; the only non-string scalar on these DTOs besides `priority`, `matched`, `alreadyEdited`, `updated`, `skipped`).
- Every coded error gets an `errors.<code>` entry in ALL 11 catalogues (`locales/{de,en,es,fr,it,nl,pl,pt,ru,uk,zh}.json`) with real translations, and every new UI string is added to all 11 catalogues (`internal/test/i18ntest` enforces parity).
- Every new REST route gets an `apiparity` scenario + golden (`UPDATE_GOLDEN=1 go test ./internal/test/apiparity/`, then inspect the diff). `make swagger` regenerates the committed OpenAPI docs after any handler annotation change (`make go-lint` fails otherwise).
- Rule semantics (spec Part 2): classify rules only ADD classification (`NULL` target = leave alone); skip rules carry no targets and no labels; a matching skip rule beats every classify rule regardless of priority; skip rules act only on events that reached stage 3 (mapped account, currency converted); `apply-rule` refuses skip rules; `suggest-rules` never returns skip rules; label cap per row is 10 (`model.MaxImportRuleLabels`, mirroring the transaction feature's unexported `maxLabelsPerImportRow`).
- Match-field ruling: `external_payee` matches the event payee (ledger `external_payee`, the text that becomes the transaction description); `description` matches the provider description (ledger `external_description`), falling back to the payee when the provider sends none (Apple Wallet). `external_category` is reserved by the spec but no provider supplies one yet — the DTO rejects it (`common.invalid_choice`) in this stage.
- Text normalization is ONE function on the server (`imports.NormalizeText`: trim, collapse internal whitespace runs to one space; case-fold with `strings.ToLower` unless the rule is case-sensitive) and mirrored verbatim in the SPA (`web/src/lib/importMatch.ts`), so the count the prompt shows equals what the server will match.
- The SimpleFIN access URL is never persisted, logged, formatted into an error, put in a query string, a query key, or an analytics event. No rule endpoint touches it.
- Product analytics: every new user action fires a `METRICS` event (`web/src/lib/metrics.ts`) at the mutation's success point; `metrics-coverage.test.ts` fails on an unfired key.
- `docs/regression-test-plan.md` gains items for every user-observable change in the same PR; CLAUDE.md's route count and env-var list are updated.
- Go toolchain: `export PATH=/usr/local/go/bin:$PATH:$HOME/go/bin GOTOOLCHAIN=go1.27.1`. Run from the worktree root; never `cd` out of it. Commit trailer: `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>`.
- Known pre-existing failures not to "fix": vitest `web/src/api/transaction.test.ts` (Blob), oxlint warnings, an occasional `PlanSheet.test.tsx` 5 s timeout.

---

## File map

| File | Responsibility |
|---|---|
| `internal/infra/storage/migrations/{sqlite,pgsql}/20260908000000.sql` | Drop the placeholder `import_rules`/`import_rule_labels`, recreate to spec |
| `internal/infra/storage/migrations/migrations_import_test.go` | Probe the new columns; CHECK constraint test |
| `internal/infra/storage/sqlc/query/{sqlite,pgsql}/imports.sql` | Rule + label-join queries |
| `internal/model/imports.go` | `ImportRule`, `ImportClassification`, rule constants, `MaxImportRuleLabels` |
| `internal/model/imports_dto.go` | Rule DTOs + `Validate()`; provenance DTO gains `applied*` fields |
| `internal/model/imports_dto_test.go` | Validation tests |
| `internal/imports/repository.go` | `Repository` gains rule/label methods |
| `internal/imports/repo/{repo,sqlite,pgsql,rule}.go` | Rule persistence (engine-adapter pattern) |
| `internal/imports/repo/rule_test.go` | Repo round-trip tests |
| `internal/test/fixture/imports.go` | `ImportRule` fixture builder |
| `internal/imports/rules.go` | `NormalizeText`, `matchRule`, `ruleSet` (skip / classify), `loadRules` |
| `internal/imports/rules_test.go` | Pure matcher tests |
| `internal/imports/ports.go` | `ClassificationLister`, `Completer`; `TransactionLister.GetByID`; `TransactionWriter.UpdateTransactionReplacingLabels`; `RateScopeSuggestRules` |
| `internal/imports/service.go` | `NewService` takes the lister; `SetCompleter` |
| `internal/imports/ingest.go`, `sync.go`, `accountlink.go`, `queue.go` | Rule set threaded through `resolve`/`place`/`applyEvent`/`convertQueued`; applied snapshot written |
| `internal/imports/rule.go` | Rule CRUD, preview, apply use cases |
| `internal/imports/rule_test.go` | Service tests for CRUD/preview/apply |
| `internal/imports/suggest.go`, `suggest_test.go` | `SuggestRules` (LLM) |
| `internal/imports/api/rule.go`, `routes.go`, `handlers.go` | Seven new handlers + registration |
| `internal/server/glue_imports.go`, `server.go` | `ImportsClassificationLister`, lister/writer additions, AI completer wiring |
| `internal/shared/errs/codes.go`, `locales/*.json` | Three new codes + translations (not-found stays uncoded) |
| `internal/infra/ai/{client,client_test}.go` | OpenAI-compatible chat-completions client |
| `internal/config/config.go`, `config_test.go` | `ECONUMO_AI_DSN`, `ECONUMO_RATE_LIMIT_SUGGEST_RULES` |
| `internal/web/router/router.go`, `router_test.go`, `web/public/econumo-config.js` | `AI_ENABLED` config key |
| `internal/test/apiparity/scenarios_imports.go` (or the existing imports scenario file) + goldens | Coverage for the seven routes |
| `web/src/api/dto/imports.ts`, `web/src/api/imports.ts`, `web/src/app/queryKeys.ts`, `web/src/features/imports/queries.ts` | DTOs, client, hooks |
| `web/src/lib/importMatch.ts`, `importMatch.test.ts` | `normalizeMatchText`, `suggestMatchValue`, `ruleDiff` |
| `web/src/features/imports/RulePromptDialog.tsx`, `.test.tsx` | Post-edit "make this a rule?" prompt |
| `web/src/features/imports/ImportRulesPage.tsx`, `ImportRuleDialog.tsx`, `.test.tsx` | Rules management |
| `web/src/features/transactions/TransactionDialog.tsx`, `web/src/app/uiStore.ts`, `web/src/app/layouts/ApplicationLayout.tsx`, `web/src/app/routes.tsx`, `web/src/app/router-pages.ts`, `web/src/features/settings/SettingsPage.tsx` | Wiring |
| `web/src/lib/metrics.ts`, `web/src/lib/config.ts`, `web/src/test/fixtures.ts` | Metrics, `isAiEnabled()`, msw fixtures |
| `docs/regression-test-plan.md`, `CLAUDE.md`, `.env.example` | Docs |

---

### Task 1: Migration — recreate `import_rules` to the spec shape

**Files:**
- Create: `internal/infra/storage/migrations/sqlite/20260908000000.sql`
- Create: `internal/infra/storage/migrations/pgsql/20260908000000.sql`
- Modify: `internal/infra/storage/migrations/migrations_import_test.go`

**Interfaces:**
- Produces: tables `import_rules` (columns below) and `import_rule_labels(rule_id, label_id)`; `import_link_applied_labels(link_id, label_id)` is untouched (already created in `20260901000000.sql`).

The stage-1 placeholder (`20260901000000.sql`) created `import_rules` with columns that never shipped in any release (`position, match_payee, match_description, match_amount_min, match_amount_max, category_id, payee_id, tag_id`). The migration drops and recreates it — the table has never held data.

- [ ] **Step 1: Write the failing migration test**

Append to `internal/infra/storage/migrations/migrations_import_test.go`:

```go
func TestMigration20260908_ImportRules(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	for _, q := range []string{
		`SELECT id, user_id, source_id, action, match_field, match_type, match_value, is_case_sensitive,
		        target_category_id, target_payee_id, target_tag_id, priority, created_at, updated_at
		   FROM import_rules WHERE 1 = 0`,
		`SELECT rule_id, label_id FROM import_rule_labels WHERE 1 = 0`,
		`SELECT link_id, label_id FROM import_link_applied_labels WHERE 1 = 0`,
	} {
		if _, err := db.Raw.ExecContext(ctx, db.Rebind(q)); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	// the placeholder shape must be gone
	if _, err := db.Raw.ExecContext(ctx, db.Rebind(`SELECT match_payee FROM import_rules WHERE 1 = 0`)); err == nil {
		t.Fatal("placeholder column match_payee still exists")
	}

	f := fixture.New(t, db)
	user := f.User(fixture.User{Email: "rules@example.test"})
	cat := f.Category(fixture.Category{UserID: user})
	insert := func(id, action string, category *string) error {
		_, err := db.Raw.ExecContext(ctx, db.Rebind(`INSERT INTO import_rules
			(id, user_id, source_id, action, match_field, match_type, match_value, is_case_sensitive,
			 target_category_id, target_payee_id, target_tag_id, priority, created_at, updated_at)
			VALUES (?, ?, NULL, ?, 'external_payee', 'contains', 'STARBUCKS', ?, ?, NULL, NULL, 0, ?, ?)`),
			id, user, action, false, category, "2026-09-08 00:00:00", "2026-09-08 00:00:00")
		return err
	}
	if err := insert(vo.NewId().String(), "classify", &cat); err != nil {
		t.Fatalf("classify rule with a target must insert: %v", err)
	}
	if err := insert(vo.NewId().String(), "skip", nil); err != nil {
		t.Fatalf("skip rule without targets must insert: %v", err)
	}
	if err := insert(vo.NewId().String(), "skip", &cat); err == nil {
		t.Fatal("CHECK must reject a skip rule with a target")
	}
}
```

Add the imports the file needs if missing: `"github.com/econumo/econumo/internal/test/fixture"` and `"github.com/econumo/econumo/internal/shared/vo"`.

Also update the existing probe in `TestMigration20260901_ImportTablesAndTokenScope`: the line that selects the placeholder columns from `import_rules` (`position, match_payee, ...`) must now select `id, user_id, source_id, action, match_field, match_type, match_value, priority` instead — the 20260901 test runs on the fully migrated schema, so it sees the recreated table.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/infra/storage/migrations/ -run 'TestMigration20260908|TestMigration20260901' -v`
Expected: FAIL — `no such column: is_case_sensitive` (sqlite) on the first probe.

- [ ] **Step 3: Write the sqlite migration**

`internal/infra/storage/migrations/sqlite/20260908000000.sql`:

```sql
-- Stage 4: import rules. The stage-1 placeholder shape never shipped with
-- data, so it is recreated rather than altered.
DROP TABLE IF EXISTS import_rule_labels;
DROP TABLE IF EXISTS import_rules;

CREATE TABLE import_rules (
    id TEXT NOT NULL PRIMARY KEY,
    user_id TEXT NOT NULL,
    source_id TEXT DEFAULT NULL,
    action TEXT NOT NULL,
    match_field TEXT NOT NULL,
    match_type TEXT NOT NULL,
    match_value TEXT NOT NULL,
    is_case_sensitive BOOLEAN DEFAULT '0' NOT NULL,
    target_category_id TEXT DEFAULT NULL,
    target_payee_id TEXT DEFAULT NULL,
    target_tag_id TEXT DEFAULT NULL,
    priority INTEGER DEFAULT 0 NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT FK_import_rules_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT FK_import_rules_source FOREIGN KEY (source_id) REFERENCES import_sources (id) ON DELETE CASCADE,
    CONSTRAINT FK_import_rules_category FOREIGN KEY (target_category_id) REFERENCES categories (id) ON DELETE SET NULL,
    CONSTRAINT FK_import_rules_payee FOREIGN KEY (target_payee_id) REFERENCES payees (id) ON DELETE SET NULL,
    CONSTRAINT FK_import_rules_tag FOREIGN KEY (target_tag_id) REFERENCES tags (id) ON DELETE SET NULL,
    CONSTRAINT CHK_import_rules_skip_has_no_targets CHECK (action = 'classify' OR (target_category_id IS NULL AND target_payee_id IS NULL AND target_tag_id IS NULL))
);
CREATE INDEX IDX_import_rules_user_id_priority ON import_rules (user_id, priority);
CREATE INDEX IDX_import_rules_source_id ON import_rules (source_id);

CREATE TABLE import_rule_labels (
    rule_id TEXT NOT NULL,
    label_id TEXT NOT NULL,
    PRIMARY KEY (rule_id, label_id),
    CONSTRAINT FK_import_rule_labels_rule FOREIGN KEY (rule_id) REFERENCES import_rules (id) ON DELETE CASCADE,
    CONSTRAINT FK_import_rule_labels_label FOREIGN KEY (label_id) REFERENCES labels (id) ON DELETE CASCADE
);
CREATE INDEX IDX_import_rule_labels_label_id ON import_rule_labels (label_id);
```

- [ ] **Step 4: Write the pgsql migration**

`internal/infra/storage/migrations/pgsql/20260908000000.sql`:

```sql
DROP TABLE IF EXISTS import_rule_labels;
DROP TABLE IF EXISTS import_rules;

CREATE TABLE import_rules (
    id UUID NOT NULL PRIMARY KEY,
    user_id UUID NOT NULL,
    source_id UUID DEFAULT NULL,
    action TEXT NOT NULL,
    match_field TEXT NOT NULL,
    match_type TEXT NOT NULL,
    match_value TEXT NOT NULL,
    is_case_sensitive BOOLEAN DEFAULT false NOT NULL,
    target_category_id UUID DEFAULT NULL,
    target_payee_id UUID DEFAULT NULL,
    target_tag_id UUID DEFAULT NULL,
    priority BIGINT DEFAULT 0 NOT NULL,
    created_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL,
    updated_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL,
    CONSTRAINT FK_import_rules_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT FK_import_rules_source FOREIGN KEY (source_id) REFERENCES import_sources (id) ON DELETE CASCADE,
    CONSTRAINT FK_import_rules_category FOREIGN KEY (target_category_id) REFERENCES categories (id) ON DELETE SET NULL,
    CONSTRAINT FK_import_rules_payee FOREIGN KEY (target_payee_id) REFERENCES payees (id) ON DELETE SET NULL,
    CONSTRAINT FK_import_rules_tag FOREIGN KEY (target_tag_id) REFERENCES tags (id) ON DELETE SET NULL,
    CONSTRAINT CHK_import_rules_skip_has_no_targets CHECK (action = 'classify' OR (target_category_id IS NULL AND target_payee_id IS NULL AND target_tag_id IS NULL))
);
CREATE INDEX IDX_import_rules_user_id_priority ON import_rules (user_id, priority);
CREATE INDEX IDX_import_rules_source_id ON import_rules (source_id);

CREATE TABLE import_rule_labels (
    rule_id UUID NOT NULL,
    label_id UUID NOT NULL,
    PRIMARY KEY (rule_id, label_id),
    CONSTRAINT FK_import_rule_labels_rule FOREIGN KEY (rule_id) REFERENCES import_rules (id) ON DELETE CASCADE,
    CONSTRAINT FK_import_rule_labels_label FOREIGN KEY (label_id) REFERENCES labels (id) ON DELETE CASCADE
);
CREATE INDEX IDX_import_rule_labels_label_id ON import_rule_labels (label_id);
```

Check the existing pgsql migrations for the exact `users.id`/`categories.id` column types (they are `UUID` in this repo — confirm with `grep -n "CREATE TABLE categories" -A 3 internal/infra/storage/migrations/pgsql/*.sql`) and match them.

- [ ] **Step 5: Run the tests to verify they pass on sqlite, then on PostgreSQL**

Run: `go test ./internal/infra/storage/migrations/ -run 'TestMigration20260908|TestMigration20260901' -v`
Expected: PASS.

Run (throwaway Postgres, see `.remember` / memory `go-toolchain-env`): `DATABASE_TEST_PGSQL_URL='postgres://econumo:econumo@127.0.0.1:55432/econumo_test?sslmode=disable' DBTEST_ENGINE=pgsql go test -tags enginecompare ./internal/infra/storage/migrations/ -run TestMigration20260908 -v`
Expected: PASS.

- [ ] **Step 6: Regenerate sqlc and confirm the model changed**

Run: `sqlc generate -f internal/infra/storage/sqlc/sqlc.yaml && go build ./... && grep -n "IsCaseSensitive" internal/infra/storage/sqlc/gen/sqlite/models.go internal/infra/storage/sqlc/gen/pgsql/models.go`
Expected: `ImportRule` in both `models.go` files carries `IsCaseSensitive bool`, `Priority int64`, `SourceID *string`, `TargetCategoryID *string`, etc. `go build` passes (no query references the old columns — verify with `grep -rn "match_payee" internal/` returning only the 20260901 migration).

- [ ] **Step 7: Commit**

```bash
git add internal/infra/storage/migrations internal/infra/storage/sqlc/gen
git commit -m "feat(imports): recreate import_rules to the stage-4 spec shape

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Model + DTOs + validation

**Files:**
- Modify: `internal/model/imports.go`
- Modify: `internal/model/imports_dto.go`
- Test: `internal/model/imports_dto_test.go`

**Interfaces (produces — every later task relies on these exact names):**

```go
// internal/model/imports.go
const (
	ImportRuleActionClassify = "classify"
	ImportRuleActionSkip     = "skip"

	ImportRuleMatchFieldDescription   = "description"
	ImportRuleMatchFieldExternalPayee = "external_payee"

	ImportRuleMatchTypeExact    = "exact"
	ImportRuleMatchTypeContains = "contains"
	ImportRuleMatchTypePrefix   = "prefix"

	// MaxImportRuleLabels mirrors the transaction feature's per-row label cap.
	MaxImportRuleLabels = 10

	ImportRuleScopeRun    = "run"
	ImportRuleScopeSource = "source"
	ImportRuleScopeAll    = "all"
)

type ImportRule struct {
	ID               vo.Id
	UserID           vo.Id
	SourceID         *vo.Id // nil = every source
	Action           string
	MatchField       string
	MatchType        string
	MatchValue       string
	IsCaseSensitive  bool
	TargetCategoryID *vo.Id
	TargetPayeeID    *vo.Id
	TargetTagID      *vo.Id
	LabelIDs         []vo.Id
	Priority         int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// ImportClassification is what the pipeline decided for one row: the ids
// written to the transaction and snapshotted on the ledger as applied_*.
type ImportClassification struct {
	CategoryID *vo.Id
	PayeeID    *vo.Id
	TagID      *vo.Id
	LabelIDs   []vo.Id
	RuleID     *vo.Id // the first classify rule that contributed, nil when none fired
}
```

```go
// internal/model/imports_dto.go
type ImportRuleSpec struct {
	SourceId        *string  `json:"sourceId"`        // nil/"" = every source
	Action          string   `json:"action"`
	MatchField      string   `json:"matchField"`
	MatchType       string   `json:"matchType"`
	MatchValue      string   `json:"matchValue"`
	IsCaseSensitive bool     `json:"isCaseSensitive"`
	CategoryId      *string  `json:"categoryId"`
	PayeeId         *string  `json:"payeeId"`
	TagId           *string  `json:"tagId"`
	LabelIds        []string `json:"labelIds"`
	Priority        int      `json:"priority"`
}
func (r ImportRuleSpec) Validate() error

type CreateImportRuleRequest struct { Id string `json:"id"`; ImportRuleSpec }   // Id optional (client-minted for idempotency), validated as an id when present
type UpdateImportRuleRequest struct { Id string `json:"id"`; ImportRuleSpec }   // Id required
type DeleteImportRuleRequest struct { Id string `json:"id"` }
type PreviewImportRuleRequest struct { ImportRuleSpec; Scope string `json:"scope"`; RunId string `json:"runId"`; ScopeSourceId string `json:"scopeSourceId"` }
type PreviewImportRuleResult struct { Matched int `json:"matched"`; AlreadyEdited int `json:"alreadyEdited"` }
type ApplyImportRuleRequest struct { RuleId string `json:"ruleId"`; Scope string `json:"scope"`; RunId string `json:"runId"`; ScopeSourceId string `json:"scopeSourceId"`; IncludeEdited bool `json:"includeEdited"` }
type ApplyImportRuleResult struct { Updated int `json:"updated"`; Skipped int `json:"skipped"` }
type ImportRuleResult struct {
	Id, SourceId, Action, MatchField, MatchType, MatchValue string
	IsCaseSensitive bool
	CategoryId, PayeeId, TagId string
	LabelIds []string
	Priority int
	CreatedAt, UpdatedAt string
}   // json tags: id, sourceId, action, matchField, matchType, matchValue, isCaseSensitive, categoryId, payeeId, tagId, labelIds, priority, createdAt, updatedAt
type GetImportRuleListResult struct { Items []ImportRuleResult `json:"items"` }
type ImportRuleSuggestion struct { ImportRuleSpec; Reason string `json:"reason"` }
type SuggestImportRulesRequest struct { Scope string `json:"scope"`; RunId string `json:"runId"`; ScopeSourceId string `json:"scopeSourceId"` }   // same scope rules as preview/apply
type SuggestImportRulesResult struct { Items []ImportRuleSuggestion `json:"items"` }
```

Note on the `PreviewImportRuleRequest`: `ImportRuleSpec` is embedded, so the preview's `SourceId` (scope) collides with the spec's `SourceId` (rule scope). Avoid the collision by naming the preview scope fields `Scope`, `RunId`, `ScopeSourceId` with json tag `scopeSourceId`. Same for `ApplyImportRuleRequest`: `Scope`, `RunId`, `ScopeSourceId`. Both `Validate()` methods require `scope ∈ {run, source, all}`, `runId` non-blank when scope=run, `scopeSourceId` non-blank when scope=source.

`TransactionImportLinkResult` gains (all strings, `""` for NULL; `labelIds` always a list, never null):
`RunId string json:"runId"`, `ExternalDescription string json:"externalDescription"`, `AppliedCategoryId json:"appliedCategoryId"`, `AppliedPayeeId json:"appliedPayeeId"`, `AppliedTagId json:"appliedTagId"`, `AppliedLabelIds []string json:"appliedLabelIds"`, `AppliedRuleId json:"appliedRuleId"`.

- [ ] **Step 1: Write the failing validation tests**

Append to the EXISTING `internal/model/imports_dto_test.go` (package `model_test`; it already defines `fieldCodes(t, err) map[string]string` and imports `errors`, `strings`, `testing`, `model`, `errs` — do not redefine them):

```go
func classifySpec() model.ImportRuleSpec {
	cat := "0192d6d4-0000-7000-8000-000000000001"
	return model.ImportRuleSpec{
		Action: model.ImportRuleActionClassify, MatchField: model.ImportRuleMatchFieldExternalPayee,
		MatchType: model.ImportRuleMatchTypeContains, MatchValue: "STARBUCKS", CategoryId: &cat,
	}
}

func TestImportRuleSpec_Validate(t *testing.T) {
	if err := classifySpec().Validate(); err != nil {
		t.Fatalf("valid classify spec: %v", err)
	}
	skip := model.ImportRuleSpec{Action: model.ImportRuleActionSkip, MatchField: model.ImportRuleMatchFieldDescription, MatchType: model.ImportRuleMatchTypePrefix, MatchValue: "PAYMENT - THANK YOU"}
	if err := skip.Validate(); err != nil {
		t.Fatalf("valid skip spec: %v", err)
	}

	blank := classifySpec()
	blank.MatchValue = "   "
	if got := fieldCodes(t, blank.Validate()); got["matchValue"] != errs.CodeIsBlank {
		t.Errorf("blank matchValue: %v", got)
	}
	long := classifySpec()
	long.MatchValue = strings.Repeat("x", 256)
	if got := fieldCodes(t, long.Validate()); got["matchValue"] != errs.CodeTooLong {
		t.Errorf("long matchValue: %v", got)
	}
	for _, tc := range []struct{ name string; mut func(*model.ImportRuleSpec); field string }{
		{"bad action", func(s *model.ImportRuleSpec) { s.Action = "delete" }, "action"},
		{"reserved match field", func(s *model.ImportRuleSpec) { s.MatchField = "external_category" }, "matchField"},
		{"bad match type", func(s *model.ImportRuleSpec) { s.MatchType = "regex" }, "matchType"},
		{"bad source id", func(s *model.ImportRuleSpec) { v := "nope"; s.SourceId = &v }, "sourceId"},
		{"bad category id", func(s *model.ImportRuleSpec) { v := "nope"; s.CategoryId = &v }, "categoryId"},
		{"bad label id", func(s *model.ImportRuleSpec) { s.LabelIds = []string{"nope"} }, "labelIds"},
	} {
		s := classifySpec()
		tc.mut(&s)
		got := fieldCodes(t, s.Validate())
		if tc.field == "action" || tc.field == "matchField" || tc.field == "matchType" {
			if got[tc.field] != errs.CodeInvalidChoice {
				t.Errorf("%s: %v", tc.name, got)
			}
		} else if got[tc.field] != errs.CodeInvalidFormat {
			t.Errorf("%s: %v", tc.name, got)
		}
	}

	// classify with nothing to apply is pointless; skip with targets/labels is contradictory
	empty := classifySpec()
	empty.CategoryId = nil
	if got := fieldCodes(t, empty.Validate()); got["action"] != errs.CodeInvalidChoice {
		t.Errorf("classify without targets: %v", got)
	}
	cat := "0192d6d4-0000-7000-8000-000000000001"
	skipWithTarget := skip
	skipWithTarget.CategoryId = &cat
	if got := fieldCodes(t, skipWithTarget.Validate()); got["categoryId"] != errs.CodeInvalidChoice {
		t.Errorf("skip with target: %v", got)
	}
	skipWithLabels := skip
	skipWithLabels.LabelIds = []string{"0192d6d4-0000-7000-8000-000000000002"}
	if got := fieldCodes(t, skipWithLabels.Validate()); got["labelIds"] != errs.CodeInvalidChoice {
		t.Errorf("skip with labels: %v", got)
	}
	tooMany := classifySpec()
	for i := 0; i < model.MaxImportRuleLabels+1; i++ {
		tooMany.LabelIds = append(tooMany.LabelIds, "0192d6d4-0000-7000-8000-0000000000"+string(rune('a'+i))+"0")
	}
	if got := fieldCodes(t, tooMany.Validate()); got["labelIds"] != errs.CodeTooLong {
		t.Errorf("too many labels: %v", got)
	}
}

func TestPreviewAndApplyImportRuleRequest_Validate(t *testing.T) {
	p := model.PreviewImportRuleRequest{ImportRuleSpec: classifySpec(), Scope: "run"}
	if got := fieldCodes(t, p.Validate()); got["runId"] != errs.CodeIsBlank {
		t.Errorf("scope=run without runId: %v", got)
	}
	p.Scope = "source"
	if got := fieldCodes(t, p.Validate()); got["scopeSourceId"] != errs.CodeIsBlank {
		t.Errorf("scope=source without scopeSourceId: %v", got)
	}
	p.Scope = "everything"
	if got := fieldCodes(t, p.Validate()); got["scope"] != errs.CodeInvalidChoice {
		t.Errorf("bad scope: %v", got)
	}
	p.Scope = "all"
	if err := p.Validate(); err != nil {
		t.Errorf("scope=all: %v", err)
	}
	a := model.ApplyImportRuleRequest{Scope: "all"}
	if got := fieldCodes(t, a.Validate()); got["ruleId"] != errs.CodeIsBlank {
		t.Errorf("apply without ruleId: %v", got)
	}
	a.RuleId = "0192d6d4-0000-7000-8000-000000000009"
	if err := a.Validate(); err != nil {
		t.Errorf("valid apply: %v", err)
	}
	if got := fieldCodes(t, model.UpdateImportRuleRequest{ImportRuleSpec: classifySpec()}.Validate()); got["id"] != errs.CodeIsBlank {
		t.Errorf("update without id: %v", got)
	}
	if got := fieldCodes(t, model.DeleteImportRuleRequest{}.Validate()); got["id"] != errs.CodeIsBlank {
		t.Errorf("delete without id: %v", got)
	}
}
```

Note: the "too many labels" loop needs 11 syntactically valid UUIDs; if `vo.ParseId` rejects the constructed strings, build them with `vo.NewId().String()` instead (import `internal/shared/vo`).

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/model/ -run 'ImportRule' -v`
Expected: FAIL to compile — `undefined: model.ImportRuleSpec`.

- [ ] **Step 3: Add the model types and constants**

Append to `internal/model/imports.go` the constants and structs listed under **Interfaces** above (verbatim).

- [ ] **Step 4: Add the DTOs and validation**

Append to `internal/model/imports_dto.go`:

```go
const importRuleMatchValueMax = 255

func invalidChoiceField(key string) errs.FieldError {
	return errs.FieldError{Key: key, Message: "The value you selected is not a valid choice.", Code: errs.CodeInvalidChoice}
}

func invalidFormatField(key string) errs.FieldError {
	return errs.FieldError{Key: key, Message: "This value is not valid.", Code: errs.CodeInvalidFormat}
}

func optionalIDField(key string, raw *string, fields *[]errs.FieldError) {
	if raw == nil || *raw == "" {
		return
	}
	if _, err := vo.ParseId(*raw); err != nil {
		*fields = append(*fields, invalidFormatField(key))
	}
}

func (r ImportRuleSpec) Validate() error {
	var fields []errs.FieldError
	switch r.Action {
	case ImportRuleActionClassify, ImportRuleActionSkip:
	default:
		fields = append(fields, invalidChoiceField("action"))
	}
	switch r.MatchField {
	case ImportRuleMatchFieldDescription, ImportRuleMatchFieldExternalPayee:
	default:
		// external_category is reserved by the design; no provider supplies one yet
		fields = append(fields, invalidChoiceField("matchField"))
	}
	switch r.MatchType {
	case ImportRuleMatchTypeExact, ImportRuleMatchTypeContains, ImportRuleMatchTypePrefix:
	default:
		fields = append(fields, invalidChoiceField("matchType"))
	}
	if strings.TrimSpace(r.MatchValue) == "" {
		fields = append(fields, blankField("matchValue"))
	} else if len(r.MatchValue) > importRuleMatchValueMax {
		fields = append(fields, tooLongField("matchValue"))
	}
	optionalIDField("sourceId", r.SourceId, &fields)
	optionalIDField("categoryId", r.CategoryId, &fields)
	optionalIDField("payeeId", r.PayeeId, &fields)
	optionalIDField("tagId", r.TagId, &fields)
	for _, id := range r.LabelIds {
		if _, err := vo.ParseId(id); err != nil {
			fields = append(fields, invalidFormatField("labelIds"))
			break
		}
	}
	if len(r.LabelIds) > MaxImportRuleLabels {
		fields = append(fields, tooLongField("labelIds"))
	}
	hasTarget := hasID(r.CategoryId) || hasID(r.PayeeId) || hasID(r.TagId) || len(r.LabelIds) > 0
	switch r.Action {
	case ImportRuleActionClassify:
		if !hasTarget {
			fields = append(fields, invalidChoiceField("action"))
		}
	case ImportRuleActionSkip:
		// the CHECK constraint covers the target columns; labels are rows in
		// another table, so the service is the only guard for them
		for _, f := range []struct{ key string; set bool }{
			{"categoryId", hasID(r.CategoryId)}, {"payeeId", hasID(r.PayeeId)}, {"tagId", hasID(r.TagId)}, {"labelIds", len(r.LabelIds) > 0},
		} {
			if f.set {
				fields = append(fields, invalidChoiceField(f.key))
			}
		}
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

func hasID(raw *string) bool { return raw != nil && *raw != "" }

func (r CreateImportRuleRequest) Validate() error {
	if r.Id != "" {
		if _, err := vo.ParseId(r.Id); err != nil {
			return errs.NewValidation("Validation failed", invalidFormatField("id"))
		}
	}
	return r.ImportRuleSpec.Validate()
}

func (r UpdateImportRuleRequest) Validate() error {
	if err := requireNonBlank("id", r.Id); err != nil {
		return err
	}
	return r.ImportRuleSpec.Validate()
}

func (r DeleteImportRuleRequest) Validate() error { return requireNonBlank("id", r.Id) }

func validateRuleScope(scope, runID, sourceID string) error {
	var fields []errs.FieldError
	switch scope {
	case ImportRuleScopeRun:
		if strings.TrimSpace(runID) == "" {
			fields = append(fields, blankField("runId"))
		}
	case ImportRuleScopeSource:
		if strings.TrimSpace(sourceID) == "" {
			fields = append(fields, blankField("scopeSourceId"))
		}
	case ImportRuleScopeAll:
	default:
		fields = append(fields, invalidChoiceField("scope"))
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

func (r PreviewImportRuleRequest) Validate() error {
	if err := r.ImportRuleSpec.Validate(); err != nil {
		return err
	}
	return validateRuleScope(r.Scope, r.RunId, r.ScopeSourceId)
}

func (r ApplyImportRuleRequest) Validate() error {
	if err := requireNonBlank("ruleId", r.RuleId); err != nil {
		return err
	}
	return validateRuleScope(r.Scope, r.RunId, r.ScopeSourceId)
}

func (r SuggestImportRulesRequest) Validate() error {
	return validateRuleScope(r.Scope, r.RunId, r.ScopeSourceId)
}
```

Declare the struct types exactly as in **Interfaces** (with json tags spelled out; embed `ImportRuleSpec` in the create/update/preview requests and in `ImportRuleSuggestion`), add the new fields to `TransactionImportLinkResult`, and add `GetImportRuleListResult`, `SuggestImportRulesRequest` (with the `Validate` above) and `SuggestImportRulesResult`. Check `errs.CodeInvalidFormat` exists in `internal/shared/errs/codes.go` (it does; reuse its message text from an existing `invalidFormat` helper if one is already defined elsewhere in `model` — grep for `CodeInvalidFormat` in `internal/model/` and reuse rather than duplicate).

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/model/ -run 'ImportRule' -v && go build ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/model
git commit -m "feat(imports): rule model, DTOs and validation

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: sqlc queries, repository methods, fixture builder

**Files:**
- Modify: `internal/infra/storage/sqlc/query/sqlite/imports.sql`
- Modify: `internal/infra/storage/sqlc/query/pgsql/imports.sql`
- Modify: `internal/imports/repository.go`
- Modify: `internal/imports/repo/repo.go` (querier interface + type aliases), `internal/imports/repo/sqlite.go`, `internal/imports/repo/pgsql.go`
- Create: `internal/imports/repo/rule.go`
- Modify: `internal/test/fixture/imports.go`
- Test: `internal/imports/repo/rule_test.go`

**Interfaces:**
- Consumes: `model.ImportRule`, `model.ImportTransactionLink` (Task 2), generated `sqlitegen.ImportRule` (Task 1).
- Produces (added to the `Repository` interface in `internal/imports/repository.go`):

```go
// rules
InsertRule(ctx context.Context, r *model.ImportRule) error            // also writes r.LabelIDs
UpdateRule(ctx context.Context, r *model.ImportRule) error            // also replaces the label set
DeleteRule(ctx context.Context, id vo.Id) error
GetRule(ctx context.Context, id vo.Id) (*model.ImportRule, error)     // errs.NotFound "Import rule not found" (LabelIDs populated)
ListRulesByUser(ctx context.Context, userID vo.Id) ([]model.ImportRule, error) // ORDER BY priority, created_at, id; LabelIDs populated
// applied-label snapshot on ledger rows
ReplaceLinkAppliedLabels(ctx context.Context, linkID vo.Id, labelIDs []vo.Id) error
ListLinkAppliedLabels(ctx context.Context, linkID vo.Id) ([]vo.Id, error)
// scope=all for preview/apply
ListLinksByUser(ctx context.Context, userID vo.Id) ([]model.ImportTransactionLink, error)
```

- Fixture: `fixture.ImportRule{ID, UserID, SourceID, Action, MatchField, MatchType, MatchValue string; IsCaseSensitive bool; CategoryID, PayeeID, TagID string; LabelIDs []string; Priority int}`; `func (b *Builder) ImportRule(r ImportRule) string` (defaults: `Action=classify`, `MatchField=external_payee`, `MatchType=contains`, `MatchValue="Rule"`, ID minted when empty; `""` id strings → NULL).

- [ ] **Step 1: Write the failing repo test**

`internal/imports/repo/rule_test.go`:

```go
package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/imports/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestRules_RoundTrip(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	user := f.User(fixture.User{Email: "r@example.test"})
	cat := f.Category(fixture.Category{UserID: user})
	l1 := f.Label(fixture.Label{UserID: user, Name: "Trip"})
	l2 := f.Label(fixture.Label{UserID: user, Name: "Work"})
	src := f.ImportSource(fixture.ImportSource{UserID: user, Name: "iPhone"})
	r := repo.NewRepo(db.Engine, db.TX)
	now := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)

	catID, srcID := vo.MustParseId(cat), vo.MustParseId(src)
	rule := &model.ImportRule{
		ID: vo.NewId(), UserID: vo.MustParseId(user), SourceID: &srcID,
		Action: model.ImportRuleActionClassify, MatchField: model.ImportRuleMatchFieldExternalPayee,
		MatchType: model.ImportRuleMatchTypeContains, MatchValue: "STARBUCKS", IsCaseSensitive: true,
		TargetCategoryID: &catID, LabelIDs: []vo.Id{vo.MustParseId(l1), vo.MustParseId(l2)}, Priority: 5,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := r.InsertRule(ctx, rule); err != nil {
		t.Fatalf("InsertRule: %v", err)
	}
	got, err := r.GetRule(ctx, rule.ID)
	if err != nil {
		t.Fatalf("GetRule: %v", err)
	}
	if got.SourceID == nil || *got.SourceID != srcID || !got.IsCaseSensitive || got.Priority != 5 || got.TargetCategoryID == nil || *got.TargetCategoryID != catID {
		t.Fatalf("round trip: %+v", got)
	}
	if len(got.LabelIDs) != 2 {
		t.Fatalf("labels: %+v", got.LabelIDs)
	}
	if !got.CreatedAt.Equal(now) {
		t.Errorf("created_at = %s", got.CreatedAt)
	}

	// update replaces the label set and clears the source scope
	rule.SourceID, rule.LabelIDs, rule.MatchValue, rule.IsCaseSensitive = nil, []vo.Id{vo.MustParseId(l2)}, "starbucks", false
	if err := r.UpdateRule(ctx, rule); err != nil {
		t.Fatalf("UpdateRule: %v", err)
	}
	got, _ = r.GetRule(ctx, rule.ID)
	if got.SourceID != nil || got.IsCaseSensitive || got.MatchValue != "starbucks" || len(got.LabelIDs) != 1 || got.LabelIDs[0].String() != l2 {
		t.Fatalf("after update: %+v", got)
	}

	// list is priority-ordered and carries labels
	second := &model.ImportRule{ID: vo.NewId(), UserID: vo.MustParseId(user), Action: model.ImportRuleActionSkip, MatchField: model.ImportRuleMatchFieldDescription, MatchType: model.ImportRuleMatchTypePrefix, MatchValue: "PAYMENT", Priority: 1, CreatedAt: now, UpdatedAt: now}
	if err := r.InsertRule(ctx, second); err != nil {
		t.Fatalf("InsertRule skip: %v", err)
	}
	list, err := r.ListRulesByUser(ctx, vo.MustParseId(user))
	if err != nil || len(list) != 2 || list[0].ID != second.ID || len(list[1].LabelIDs) != 1 {
		t.Fatalf("ListRulesByUser: %v %+v", err, list)
	}

	if err := r.DeleteRule(ctx, rule.ID); err != nil {
		t.Fatalf("DeleteRule: %v", err)
	}
	if _, err := r.GetRule(ctx, rule.ID); err == nil {
		t.Fatal("deleted rule must be gone")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("want NotFound, got %v", err)
	}
	// the join rows went with it (FK cascade)
	var n int
	if err := db.Raw.QueryRowContext(ctx, db.Rebind(`SELECT count(*) FROM import_rule_labels WHERE rule_id = ?`), rule.ID.String()).Scan(&n); err != nil || n != 0 {
		t.Fatalf("rule labels left behind: %d %v", n, err)
	}
}

func TestLinkAppliedLabels_ReplaceAndList(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	user := f.User(fixture.User{Email: "r@example.test"})
	l1 := f.Label(fixture.Label{UserID: user, Name: "A"})
	l2 := f.Label(fixture.Label{UserID: user, Name: "B"})
	src := f.ImportSource(fixture.ImportSource{UserID: user, Name: "iPhone"})
	link := f.ImportTransactionLink(fixture.ImportTransactionLink{SourceID: src, ExternalAccountID: "card", ExternalTransactionID: "t1", Status: model.ImportLinkStatusQueued, ExternalAmount: "1", ExternalPostedAt: time.Now()})
	r := repo.NewRepo(db.Engine, db.TX)
	linkID := vo.MustParseId(link)
	if err := r.ReplaceLinkAppliedLabels(ctx, linkID, []vo.Id{vo.MustParseId(l1), vo.MustParseId(l2)}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	got, err := r.ListLinkAppliedLabels(ctx, linkID)
	if err != nil || len(got) != 2 {
		t.Fatalf("list: %v %v", got, err)
	}
	if err := r.ReplaceLinkAppliedLabels(ctx, linkID, nil); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if got, _ = r.ListLinkAppliedLabels(ctx, linkID); len(got) != 0 {
		t.Fatalf("after clear: %v", got)
	}
	all, err := r.ListLinksByUser(ctx, vo.MustParseId(user))
	if err != nil || len(all) != 1 || all[0].ID != linkID {
		t.Fatalf("ListLinksByUser: %v %+v", err, all)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/imports/repo/ -run 'TestRules_RoundTrip|TestLinkAppliedLabels' -v`
Expected: FAIL to compile — `r.InsertRule undefined`.

- [ ] **Step 3: Add the queries (both engines)**

Append to `internal/infra/storage/sqlc/query/sqlite/imports.sql` (note: the `;` stays on the statement's last line):

```sql
-- name: InsertImportRule :exec
INSERT INTO import_rules (id, user_id, source_id, action, match_field, match_type, match_value, is_case_sensitive, target_category_id, target_payee_id, target_tag_id, priority, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateImportRule :exec
UPDATE import_rules
SET source_id = ?, action = ?, match_field = ?, match_type = ?, match_value = ?, is_case_sensitive = ?, target_category_id = ?, target_payee_id = ?, target_tag_id = ?, priority = ?, updated_at = ?
WHERE id = ?;

-- name: DeleteImportRule :exec
DELETE FROM import_rules WHERE id = ?;

-- name: GetImportRuleByID :one
SELECT id, user_id, source_id, action, match_field, match_type, match_value, is_case_sensitive, target_category_id, target_payee_id, target_tag_id, priority, created_at, updated_at
FROM import_rules
WHERE id = ?;

-- name: ListImportRulesByUser :many
SELECT id, user_id, source_id, action, match_field, match_type, match_value, is_case_sensitive, target_category_id, target_payee_id, target_tag_id, priority, created_at, updated_at
FROM import_rules
WHERE user_id = ?
ORDER BY priority, created_at, id;

-- name: DeleteImportRuleLabels :exec
DELETE FROM import_rule_labels WHERE rule_id = ?;

-- name: InsertImportRuleLabel :exec
INSERT INTO import_rule_labels (rule_id, label_id) VALUES (?, ?);

-- name: ListImportRuleLabels :many
SELECT rule_id, label_id FROM import_rule_labels WHERE rule_id = ? ORDER BY label_id;

-- name: ListImportRuleLabelsByUser :many
SELECT rl.rule_id, rl.label_id
FROM import_rule_labels rl
JOIN import_rules r ON r.id = rl.rule_id
WHERE r.user_id = ?
ORDER BY rl.rule_id, rl.label_id;

-- name: DeleteImportLinkAppliedLabels :exec
DELETE FROM import_link_applied_labels WHERE link_id = ?;

-- name: InsertImportLinkAppliedLabel :exec
INSERT INTO import_link_applied_labels (link_id, label_id) VALUES (?, ?);

-- name: ListImportLinkAppliedLabels :many
SELECT link_id, label_id FROM import_link_applied_labels WHERE link_id = ? ORDER BY label_id;

-- name: ListImportTransactionLinksByUser :many
SELECT l.id, l.source_id, l.run_id, l.event_id, l.external_account_id, l.external_transaction_id, l.transaction_id, l.status, l.external_payee, l.external_description, l.external_amount, l.external_currency, l.external_posted_at, l.applied_category_id, l.applied_payee_id, l.applied_tag_id, l.applied_rule_id, l.imported_at
FROM import_transaction_links l
JOIN import_sources s ON s.id = l.source_id
WHERE s.user_id = ?
ORDER BY l.imported_at, l.id;
```

Append the same block to `internal/infra/storage/sqlc/query/pgsql/imports.sql` with `$1…$N` placeholders in argument order (e.g. `InsertImportRule` uses `$1`–`$14`; `UpdateImportRule` uses `$1`–`$11` for the SET list and `$12` for `id`) and, unlike the older entries in that file, the `;` on the same line as the statement's last clause.

Run: `sqlc generate -f internal/infra/storage/sqlc/sqlc.yaml && grep -n "const listImportTransactionLinksByUser" -A 8 internal/infra/storage/sqlc/gen/pgsql/imports.sql.go`
Expected: the generated SQL constant contains the full statement through `ORDER BY l.imported_at, l.id` (a `;` on its own line would have truncated it — see memory `sqlc-query-semicolon-placement`). Spot-check `updateImportRule` in both engines the same way.

- [ ] **Step 4: Extend the querier interface and adapters**

In `internal/imports/repo/repo.go` add type aliases next to the existing ones:

```go
ruleRow                 = sqlitegen.ImportRule
insertRuleParams        = sqlitegen.InsertImportRuleParams
updateRuleParams        = sqlitegen.UpdateImportRuleParams
ruleLabelRow            = sqlitegen.ImportRuleLabel
insertRuleLabelParams   = sqlitegen.InsertImportRuleLabelParams
linkAppliedLabelRow     = sqlitegen.ImportLinkAppliedLabel
insertLinkAppliedParams = sqlitegen.InsertImportLinkAppliedLabelParams
```

(Confirm the generated struct names with `grep -n "^type Import" internal/infra/storage/sqlc/gen/sqlite/models.go` — sqlc singularizes table names.)

Add to the `querier` interface:

```go
InsertImportRule(ctx context.Context, db backend.DBTX, p insertRuleParams) error
UpdateImportRule(ctx context.Context, db backend.DBTX, p updateRuleParams) error
DeleteImportRule(ctx context.Context, db backend.DBTX, id string) error
GetImportRuleByID(ctx context.Context, db backend.DBTX, id string) (ruleRow, error)
ListImportRulesByUser(ctx context.Context, db backend.DBTX, userID string) ([]ruleRow, error)
DeleteImportRuleLabels(ctx context.Context, db backend.DBTX, ruleID string) error
InsertImportRuleLabel(ctx context.Context, db backend.DBTX, p insertRuleLabelParams) error
ListImportRuleLabels(ctx context.Context, db backend.DBTX, ruleID string) ([]ruleLabelRow, error)
ListImportRuleLabelsByUser(ctx context.Context, db backend.DBTX, userID string) ([]ruleLabelRow, error)
DeleteImportLinkAppliedLabels(ctx context.Context, db backend.DBTX, linkID string) error
InsertImportLinkAppliedLabel(ctx context.Context, db backend.DBTX, p insertLinkAppliedParams) error
ListImportLinkAppliedLabels(ctx context.Context, db backend.DBTX, linkID string) ([]linkAppliedLabelRow, error)
ListImportTransactionLinksByUser(ctx context.Context, db backend.DBTX, userID string) ([]linkRow, error)
```

`sqlite.go`: one passthrough per method, e.g. `func (sqliteQuerier) InsertImportRule(ctx context.Context, db backend.DBTX, p insertRuleParams) error { return sqlitegen.New(db).InsertImportRule(ctx, p) }`.

`pgsql.go`: whole-struct conversions, e.g. `pgsqlgen.New(db).InsertImportRule(ctx, pgsqlgen.InsertImportRuleParams(p))`, and for the `:many` readers a conversion loop `out[i] = ruleRow(row)` exactly like the existing `ListImportTransactionLinksByRun` adapter. If the pgsql generated struct's field order/types differ from sqlite's (they should not — both map `BOOLEAN` to `bool`, `BIGINT`/`INTEGER` to `int64`), convert field-by-field instead of by whole-struct cast.

- [ ] **Step 5: Implement the repository methods**

Create `internal/imports/repo/rule.go`:

```go
package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (r *Repo) InsertRule(ctx context.Context, rule *model.ImportRule) error {
	err := r.q.InsertImportRule(ctx, r.db(ctx), insertRuleParams{
		ID: rule.ID.String(), UserID: rule.UserID.String(), SourceID: optionalID(rule.SourceID),
		Action: rule.Action, MatchField: rule.MatchField, MatchType: rule.MatchType, MatchValue: rule.MatchValue,
		IsCaseSensitive: rule.IsCaseSensitive, TargetCategoryID: optionalID(rule.TargetCategoryID),
		TargetPayeeID: optionalID(rule.TargetPayeeID), TargetTagID: optionalID(rule.TargetTagID),
		Priority: int64(rule.Priority), CreatedAt: rule.CreatedAt, UpdatedAt: rule.UpdatedAt,
	})
	if err != nil {
		return err
	}
	return r.replaceRuleLabels(ctx, rule.ID, rule.LabelIDs)
}

func (r *Repo) UpdateRule(ctx context.Context, rule *model.ImportRule) error {
	err := r.q.UpdateImportRule(ctx, r.db(ctx), updateRuleParams{
		SourceID: optionalID(rule.SourceID), Action: rule.Action, MatchField: rule.MatchField, MatchType: rule.MatchType,
		MatchValue: rule.MatchValue, IsCaseSensitive: rule.IsCaseSensitive, TargetCategoryID: optionalID(rule.TargetCategoryID),
		TargetPayeeID: optionalID(rule.TargetPayeeID), TargetTagID: optionalID(rule.TargetTagID),
		Priority: int64(rule.Priority), UpdatedAt: rule.UpdatedAt, ID: rule.ID.String(),
	})
	if err != nil {
		return err
	}
	return r.replaceRuleLabels(ctx, rule.ID, rule.LabelIDs)
}

func (r *Repo) replaceRuleLabels(ctx context.Context, ruleID vo.Id, labelIDs []vo.Id) error {
	if err := r.q.DeleteImportRuleLabels(ctx, r.db(ctx), ruleID.String()); err != nil {
		return err
	}
	for _, id := range labelIDs {
		if err := r.q.InsertImportRuleLabel(ctx, r.db(ctx), insertRuleLabelParams{RuleID: ruleID.String(), LabelID: id.String()}); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repo) DeleteRule(ctx context.Context, id vo.Id) error {
	return r.q.DeleteImportRule(ctx, r.db(ctx), id.String())
}

func (r *Repo) GetRule(ctx context.Context, id vo.Id) (*model.ImportRule, error) {
	row, err := r.q.GetImportRuleByID(ctx, r.db(ctx), id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Import rule not found")
		}
		return nil, err
	}
	rule, err := ruleFromRow(row)
	if err != nil {
		return nil, err
	}
	labels, err := r.q.ListImportRuleLabels(ctx, r.db(ctx), id.String())
	if err != nil {
		return nil, err
	}
	for _, l := range labels {
		lid, err := vo.ParseId(l.LabelID)
		if err != nil {
			return nil, err
		}
		rule.LabelIDs = append(rule.LabelIDs, lid)
	}
	return rule, nil
}

func (r *Repo) ListRulesByUser(ctx context.Context, userID vo.Id) ([]model.ImportRule, error) {
	rows, err := r.q.ListImportRulesByUser(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	labelRows, err := r.q.ListImportRuleLabelsByUser(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	labels := map[string][]vo.Id{}
	for _, l := range labelRows {
		lid, err := vo.ParseId(l.LabelID)
		if err != nil {
			return nil, err
		}
		labels[l.RuleID] = append(labels[l.RuleID], lid)
	}
	out := make([]model.ImportRule, 0, len(rows))
	for _, row := range rows {
		rule, err := ruleFromRow(row)
		if err != nil {
			return nil, err
		}
		rule.LabelIDs = labels[row.ID]
		out = append(out, *rule)
	}
	return out, nil
}

func ruleFromRow(row ruleRow) (*model.ImportRule, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	userID, err := vo.ParseId(row.UserID)
	if err != nil {
		return nil, err
	}
	rule := &model.ImportRule{
		ID: id, UserID: userID, Action: row.Action, MatchField: row.MatchField, MatchType: row.MatchType,
		MatchValue: row.MatchValue, IsCaseSensitive: row.IsCaseSensitive, Priority: int(row.Priority),
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	for _, f := range []struct {
		dst **vo.Id
		src *string
	}{{&rule.SourceID, row.SourceID}, {&rule.TargetCategoryID, row.TargetCategoryID}, {&rule.TargetPayeeID, row.TargetPayeeID}, {&rule.TargetTagID, row.TargetTagID}} {
		v, err := parseOptionalID(f.src)
		if err != nil {
			return nil, err
		}
		*f.dst = v
	}
	return rule, nil
}

func (r *Repo) ReplaceLinkAppliedLabels(ctx context.Context, linkID vo.Id, labelIDs []vo.Id) error {
	if err := r.q.DeleteImportLinkAppliedLabels(ctx, r.db(ctx), linkID.String()); err != nil {
		return err
	}
	for _, id := range labelIDs {
		if err := r.q.InsertImportLinkAppliedLabel(ctx, r.db(ctx), insertLinkAppliedParams{LinkID: linkID.String(), LabelID: id.String()}); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repo) ListLinkAppliedLabels(ctx context.Context, linkID vo.Id) ([]vo.Id, error) {
	rows, err := r.q.ListImportLinkAppliedLabels(ctx, r.db(ctx), linkID.String())
	if err != nil {
		return nil, err
	}
	out := make([]vo.Id, 0, len(rows))
	for _, row := range rows {
		id, err := vo.ParseId(row.LabelID)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

func (r *Repo) ListLinksByUser(ctx context.Context, userID vo.Id) ([]model.ImportTransactionLink, error) {
	rows, err := r.q.ListImportTransactionLinksByUser(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	out := make([]model.ImportTransactionLink, 0, len(rows))
	for _, row := range rows {
		l, err := linkFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, nil
}
```

`optionalID` and `parseOptionalID` already exist in `repo.go`. If the sqlite time columns come back as `time.Time` already (they do for the other import tables — check `linkFromRow`'s handling of `ImportedAt`), no parsing is needed; mirror whatever `linkFromRow` does for datetime columns.

Add the eight method signatures to the `Repository` interface in `internal/imports/repository.go` (grouped under a `// rules` comment after the link methods).

- [ ] **Step 6: Add the fixture builder**

Append to `internal/test/fixture/imports.go`:

```go
type ImportRule struct {
	ID              string
	UserID          string
	SourceID        string // "" -> NULL (every source)
	Action          string // default classify
	MatchField      string // default external_payee
	MatchType       string // default contains
	MatchValue      string // default "Rule"
	IsCaseSensitive bool
	CategoryID      string // "" -> NULL
	PayeeID         string
	TagID           string
	LabelIDs        []string
	Priority        int
}

func (b *Builder) ImportRule(r ImportRule) string {
	b.t.Helper()
	if r.ID == "" {
		r.ID = vo.NewId().String()
	}
	if r.Action == "" {
		r.Action = "classify"
	}
	if r.MatchField == "" {
		r.MatchField = "external_payee"
	}
	if r.MatchType == "" {
		r.MatchType = "contains"
	}
	if r.MatchValue == "" {
		r.MatchValue = "Rule"
	}
	b.exec(`INSERT INTO import_rules (id, user_id, source_id, action, match_field, match_type, match_value, is_case_sensitive, target_category_id, target_payee_id, target_tag_id, priority, created_at, updated_at)
	        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.UserID, nullable(r.SourceID), r.Action, r.MatchField, r.MatchType, r.MatchValue, r.IsCaseSensitive,
		nullable(r.CategoryID), nullable(r.PayeeID), nullable(r.TagID), r.Priority, b.rawTime(), b.rawTime())
	for _, l := range r.LabelIDs {
		b.exec(`INSERT INTO import_rule_labels (rule_id, label_id) VALUES (?, ?)`, r.ID, l)
	}
	return r.ID
}
```

Match the file's existing helpers: look at how `ImportTransactionLink` in the same file writes NULLable ids and timestamps (its `""`-to-NULL helper and its time formatting — memory `sqlite-time-binding-layout`: post-port tables use `fixture.RawTime`) and reuse those exact helper names instead of `nullable`/`b.rawTime()` if they are named differently.

- [ ] **Step 7: Run the tests on both engines**

Run: `go test ./internal/imports/repo/ -run 'TestRules_RoundTrip|TestLinkAppliedLabels' -v`
Expected: PASS.

Run: `DATABASE_TEST_PGSQL_URL='postgres://econumo:econumo@127.0.0.1:55432/econumo_test?sslmode=disable' DBTEST_ENGINE=pgsql go test -tags enginecompare ./internal/imports/repo/ -run 'TestRules_RoundTrip|TestLinkAppliedLabels' -v`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/infra/storage/sqlc internal/imports/repository.go internal/imports/repo internal/test/fixture/imports.go
git commit -m "feat(imports): rule persistence and applied-label snapshot

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Rule matching (pure)

**Files:**
- Create: `internal/imports/rules.go`
- Test: `internal/imports/rules_test.go`

**Interfaces (produces):**

```go
// NormalizeText trims and collapses internal whitespace runs to one space.
// Case folding is the caller's decision (see matchRule). The SPA mirrors this
// in web/src/lib/importMatch.ts — keep them identical.
func NormalizeText(s string) string

// ruleText picks the event text a rule's match_field reads.
func ruleText(field string, ev model.IngestEvent) string

func matchRule(r model.ImportRule, ev model.IngestEvent) bool

// ruleSet is one user's rules, filtered to a source, ordered by priority.
type ruleSet struct{ rules []model.ImportRule }

func newRuleSet(rules []model.ImportRule, sourceID vo.Id) ruleSet   // keeps rules with SourceID nil or == sourceID; sorts by Priority, CreatedAt, ID
func (rs ruleSet) skip(ev model.IngestEvent) *vo.Id                 // first matching skip rule's id
func (rs ruleSet) classify(ev model.IngestEvent) model.ImportClassification // first-wins per field, labels unioned in priority order, capped at MaxImportRuleLabels
```

- [ ] **Step 1: Write the failing tests**

`internal/imports/rules_test.go` (package `imports` — internal test so it can reach unexported names; the existing `ingest_test.go` is `package imports_test`, so this is a NEW internal test file):

```go
package imports

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

func TestNormalizeText(t *testing.T) {
	for in, want := range map[string]string{
		"  STARBUCKS   #123 \t SEATTLE\n": "STARBUCKS #123 SEATTLE",
		"":                                "",
		"a":                               "a",
	} {
		if got := NormalizeText(in); got != want {
			t.Errorf("NormalizeText(%q) = %q, want %q", in, got, want)
		}
	}
}

func rule(action, field, typ, value string, cs bool) model.ImportRule {
	return model.ImportRule{ID: vo.NewId(), Action: action, MatchField: field, MatchType: typ, MatchValue: value, IsCaseSensitive: cs}
}

func TestMatchRule(t *testing.T) {
	ev := model.IngestEvent{Payee: "  Starbucks  #123", Description: "CARD PURCHASE STARBUCKS #123 SEATTLE"}
	cases := []struct {
		name string
		r    model.ImportRule
		want bool
	}{
		{"contains, case-folded", rule("classify", "external_payee", "contains", "starbucks", false), true},
		{"contains, case-sensitive miss", rule("classify", "external_payee", "contains", "starbucks", true), false},
		{"contains, case-sensitive hit", rule("classify", "external_payee", "contains", "Starbucks", true), true},
		{"exact after normalization", rule("classify", "external_payee", "exact", "starbucks #123", false), true},
		{"exact miss", rule("classify", "external_payee", "exact", "starbucks", false), false},
		{"prefix", rule("classify", "external_payee", "prefix", "STAR", false), true},
		{"prefix miss", rule("classify", "external_payee", "prefix", "#123", false), false},
		{"description field", rule("skip", "description", "prefix", "card purchase", false), true},
		{"match value is normalized too", rule("classify", "external_payee", "exact", "  starbucks   #123 ", false), true},
	}
	for _, tc := range cases {
		if got := matchRule(tc.r, ev); got != tc.want {
			t.Errorf("%s: got %v", tc.name, got)
		}
	}
	// description falls back to the payee when the provider sends none
	if !matchRule(rule("classify", "description", "contains", "starbucks", false), model.IngestEvent{Payee: "Starbucks"}) {
		t.Error("description must fall back to payee")
	}
}

func TestRuleSet_SkipBeatsClassifyAndScopesBySource(t *testing.T) {
	src, other := vo.NewId(), vo.NewId()
	cat := vo.NewId()
	classify := rule("classify", "external_payee", "contains", "payment", false)
	classify.Priority, classify.TargetCategoryID = 0, &cat
	skip := rule("skip", "external_payee", "prefix", "PAYMENT - THANK YOU", false)
	skip.Priority = 99
	scoped := rule("skip", "external_payee", "contains", "payment", false)
	scoped.SourceID = &other
	rs := newRuleSet([]model.ImportRule{classify, skip, scoped}, src)
	if len(rs.rules) != 2 {
		t.Fatalf("source scoping: %d rules", len(rs.rules))
	}
	ev := model.IngestEvent{Payee: "PAYMENT - THANK YOU"}
	if got := rs.skip(ev); got == nil || *got != skip.ID {
		t.Fatalf("skip must win regardless of priority: %v", got)
	}
	if got := rs.skip(model.IngestEvent{Payee: "Autopay payment"}); got != nil {
		t.Fatalf("no skip rule matches: %v", got)
	}
}

func TestRuleSet_ClassifyFirstWinsPerFieldAndUnionsLabels(t *testing.T) {
	src := vo.NewId()
	catA, catB, payee, tag := vo.NewId(), vo.NewId(), vo.NewId(), vo.NewId()
	labels := make([]vo.Id, 0, 12)
	for i := 0; i < 12; i++ {
		labels = append(labels, vo.NewId())
	}
	now := time.Now()
	first := rule("classify", "external_payee", "contains", "star", false)
	first.Priority, first.TargetCategoryID, first.LabelIDs, first.CreatedAt = 1, &catA, labels[:6], now
	second := rule("classify", "external_payee", "contains", "bucks", false)
	second.Priority, second.TargetCategoryID, second.TargetPayeeID, second.LabelIDs, second.CreatedAt = 2, &catB, &payee, labels[4:12], now
	third := rule("classify", "external_payee", "exact", "nomatch", false)
	third.TargetTagID = &tag
	rs := newRuleSet([]model.ImportRule{second, first, third}, src)
	got := rs.classify(model.IngestEvent{Payee: "Starbucks"})
	if got.CategoryID == nil || *got.CategoryID != catA {
		t.Errorf("category must come from the higher-priority rule: %v", got.CategoryID)
	}
	if got.PayeeID == nil || *got.PayeeID != payee {
		t.Errorf("payee is filled by the first rule that sets it: %v", got.PayeeID)
	}
	if got.TagID != nil {
		t.Errorf("no matching rule sets a tag: %v", got.TagID)
	}
	if got.RuleID == nil || *got.RuleID != first.ID {
		t.Errorf("applied_rule_id is the first contributing rule: %v", got.RuleID)
	}
	if len(got.LabelIDs) != model.MaxImportRuleLabels {
		t.Fatalf("labels must be unioned (dedup 4,5) and capped: %d", len(got.LabelIDs))
	}
	for i := 0; i < 6; i++ {
		if got.LabelIDs[i] != labels[i] {
			t.Fatalf("label order follows priority: %v", got.LabelIDs)
		}
	}
	if empty := rs.classify(model.IngestEvent{Payee: "nothing"}); empty.RuleID != nil || empty.CategoryID != nil || len(empty.LabelIDs) != 0 {
		t.Errorf("no match must be empty: %+v", empty)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/imports/ -run 'TestNormalizeText|TestMatchRule|TestRuleSet' -v`
Expected: FAIL to compile — `undefined: NormalizeText`.

- [ ] **Step 3: Implement**

`internal/imports/rules.go`:

```go
package imports

import (
	"sort"
	"strings"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// NormalizeText trims and collapses whitespace runs to one space. Case
// folding is applied separately (matchRule), so a case-sensitive rule still
// sees normalized spacing. web/src/lib/importMatch.ts mirrors this exactly:
// the count the rule prompt shows must equal what the server later matches.
func NormalizeText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// ruleText picks the event text a match_field reads. external_payee is the
// text that becomes the transaction description; description is the
// provider's memo, which only SimpleFIN supplies — Apple Wallet events fall
// back to the payee so a description rule still has something to match.
func ruleText(field string, ev model.IngestEvent) string {
	if field == model.ImportRuleMatchFieldDescription && ev.Description != "" {
		return ev.Description
	}
	return ev.Payee
}

func matchRule(r model.ImportRule, ev model.IngestEvent) bool {
	text, value := NormalizeText(ruleText(r.MatchField, ev)), NormalizeText(r.MatchValue)
	if !r.IsCaseSensitive {
		text, value = strings.ToLower(text), strings.ToLower(value)
	}
	if value == "" {
		return false
	}
	switch r.MatchType {
	case model.ImportRuleMatchTypeExact:
		return text == value
	case model.ImportRuleMatchTypePrefix:
		return strings.HasPrefix(text, value)
	case model.ImportRuleMatchTypeContains:
		return strings.Contains(text, value)
	}
	return false
}

type ruleSet struct{ rules []model.ImportRule }

func newRuleSet(rules []model.ImportRule, sourceID vo.Id) ruleSet {
	kept := make([]model.ImportRule, 0, len(rules))
	for _, r := range rules {
		if r.SourceID == nil || *r.SourceID == sourceID {
			kept = append(kept, r)
		}
	}
	sort.SliceStable(kept, func(i, j int) bool {
		a, b := kept[i], kept[j]
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.Before(b.CreatedAt)
		}
		return a.ID.String() < b.ID.String()
	})
	return ruleSet{rules: kept}
}

// skip returns the first matching skip rule. Skip beats classify whatever
// the priorities: skipping makes classification moot (spec, Part 2).
func (rs ruleSet) skip(ev model.IngestEvent) *vo.Id {
	for _, r := range rs.rules {
		if r.Action == model.ImportRuleActionSkip && matchRule(r, ev) {
			id := r.ID
			return &id
		}
	}
	return nil
}

// classify folds every matching classify rule in priority order: the first
// rule to set a field wins it, labels are unioned (a set), and applied_rule_id
// is the first rule that contributed anything.
func (rs ruleSet) classify(ev model.IngestEvent) model.ImportClassification {
	var out model.ImportClassification
	seen := map[vo.Id]bool{}
	for i := range rs.rules {
		r := &rs.rules[i]
		if r.Action != model.ImportRuleActionClassify || !matchRule(*r, ev) {
			continue
		}
		contributed := false
		if out.CategoryID == nil && r.TargetCategoryID != nil {
			out.CategoryID, contributed = r.TargetCategoryID, true
		}
		if out.PayeeID == nil && r.TargetPayeeID != nil {
			out.PayeeID, contributed = r.TargetPayeeID, true
		}
		if out.TagID == nil && r.TargetTagID != nil {
			out.TagID, contributed = r.TargetTagID, true
		}
		for _, l := range r.LabelIDs {
			if seen[l] || len(out.LabelIDs) >= model.MaxImportRuleLabels {
				continue
			}
			seen[l] = true
			out.LabelIDs = append(out.LabelIDs, l)
			contributed = true
		}
		if contributed && out.RuleID == nil {
			id := r.ID
			out.RuleID = &id
		}
	}
	return out
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/imports/ -run 'TestNormalizeText|TestMatchRule|TestRuleSet' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/imports/rules.go internal/imports/rules_test.go
git commit -m "feat(imports): rule matching and rule-set folding

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Ports, glue adapters, and `NewService` wiring (no behaviour change)

**Files:**
- Modify: `internal/imports/ports.go`
- Modify: `internal/imports/service.go` (struct + `NewService` + `SetCompleter`)
- Modify: `internal/server/glue_imports.go`, `internal/server/server.go:344-366`
- Modify: `internal/imports/ingest_test.go:196,203`, `internal/imports/api/harness_test.go:109`
- Test: `internal/server/glue_imports_test.go` (add cases to the existing file if present, else create)

**Interfaces (produces):**

```go
// internal/imports/ports.go
// ClassificationLister is the owner's current classification vocabulary —
// what a rule may target and what the pipeline may write. Every id a rule
// carries is re-checked against these lists before it reaches
// CreateTransaction, so a category deleted after the rule was saved never
// fails an import.
type ClassificationLister interface {
	CategoriesByOwner(ctx context.Context, ownerID vo.Id) ([]model.ImportNamed, error)
	PayeesByOwner(ctx context.Context, ownerID vo.Id) ([]model.ImportNamed, error)
	TagsByOwner(ctx context.Context, ownerID vo.Id) ([]model.ImportNamed, error)
	LabelsByOwner(ctx context.Context, ownerID vo.Id) ([]model.ImportNamed, error)
}

type TransactionLister interface {
	ListByAccount(ctx context.Context, accountID vo.Id, from, to time.Time) ([]*model.Transaction, error)
	// GetByID returns the transaction with LabelIDs populated; errs.NotFound when missing.
	GetByID(ctx context.Context, id vo.Id) (*model.Transaction, error)
}

type TransactionWriter interface {
	CreateTransaction(ctx context.Context, userID vo.Id, req model.CreateTransactionRequest) (*model.CreateTransactionResult, error)
	UpdateTransaction(ctx context.Context, userID vo.Id, req model.UpdateTransactionRequest) (*model.UpdateTransactionResult, error)                 // labels preserved (tip amount fix)
	UpdateTransactionReplacingLabels(ctx context.Context, userID vo.Id, req model.UpdateTransactionRequest) (*model.UpdateTransactionResult, error) // req.LabelIds replaces the set (apply-rule)
}

// Completer is one chat completion. nil = AI disabled.
type Completer interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

const RateScopeSuggestRules = "import-suggest"

// internal/imports/service.go
func NewService(repo Repository, accounts AccountReader, converter CurrencyConverter, txns TransactionWriter, lister TransactionLister, entities ClassificationLister, limiter AttemptLimiter, tx port.TxRunner, clk port.Clock, cfg MatcherConfig) *Service
func (s *Service) SetCompleter(c Completer)

// internal/server/glue_imports.go
type ImportsClassificationLister struct{ categories, payees, tags, labels importsNamedByOwner }
func NewImportsClassificationLister(categories, payees, tags, labels importsNamedByOwner) *ImportsClassificationLister
```

- [ ] **Step 1: Extend the ports**

In `internal/imports/ports.go` add `ClassificationLister`, `Completer`, `RateScopeSuggestRules`, and the two new methods on `TransactionLister` / `TransactionWriter` exactly as in the Interfaces block (keep the existing doc comments; add one line to `TransactionWriter`'s: "UpdateTransactionReplacingLabels is apply-rule's path: a rule's label set is a real edit, not an amount fix.").

- [ ] **Step 2: Extend the service**

In `internal/imports/service.go`:

```go
type Service struct {
	repo      Repository
	accounts  AccountReader
	converter CurrencyConverter
	txns      TransactionWriter
	lister    TransactionLister
	entities  ClassificationLister
	limiter   AttemptLimiter
	completer Completer
	tx        port.TxRunner
	clk       port.Clock
	cfg       MatcherConfig
	providers map[string]Provider
	parsers   map[string]EventParser
}

func NewService(repo Repository, accounts AccountReader, converter CurrencyConverter, txns TransactionWriter, lister TransactionLister, entities ClassificationLister, limiter AttemptLimiter, tx port.TxRunner, clk port.Clock, cfg MatcherConfig) *Service {
	return &Service{repo: repo, accounts: accounts, converter: converter, txns: txns, lister: lister, entities: entities, limiter: limiter, tx: tx, clk: clk, cfg: cfg, providers: map[string]Provider{}, parsers: map[string]EventParser{}}
}

// SetCompleter enables suggest-rules; leaving it unset keeps the endpoint
// answering import.ai_disabled (Task 11).
func (s *Service) SetCompleter(c Completer) { s.completer = c }
```

(Keep whatever other fields the current struct has — this lists the shape, not a rewrite of unrelated lines.)

- [ ] **Step 3: Glue adapters**

In `internal/server/glue_imports.go`:

```go
type importsNamedByOwner func(ctx context.Context, ownerID vo.Id) ([]model.ImportNamed, error)

// ImportsClassificationLister composes the four per-entity owner lists the
// CSV importer already exposes into the one port the rules engine reads.
type ImportsClassificationLister struct {
	categories, payees, tags, labels importsNamedByOwner
}

func NewImportsClassificationLister(categories, payees, tags, labels importsNamedByOwner) *ImportsClassificationLister {
	return &ImportsClassificationLister{categories: categories, payees: payees, tags: tags, labels: labels}
}

func (l *ImportsClassificationLister) CategoriesByOwner(ctx context.Context, ownerID vo.Id) ([]model.ImportNamed, error) {
	return l.categories(ctx, ownerID)
}
func (l *ImportsClassificationLister) PayeesByOwner(ctx context.Context, ownerID vo.Id) ([]model.ImportNamed, error) {
	return l.payees(ctx, ownerID)
}
func (l *ImportsClassificationLister) TagsByOwner(ctx context.Context, ownerID vo.Id) ([]model.ImportNamed, error) {
	return l.tags(ctx, ownerID)
}
func (l *ImportsClassificationLister) LabelsByOwner(ctx context.Context, ownerID vo.Id) ([]model.ImportNamed, error) {
	return l.labels(ctx, ownerID)
}

var _ imports.ClassificationLister = (*ImportsClassificationLister)(nil)
```

Extend the lister source + adapter:

```go
type importsTransactionSource interface {
	ListByAccountIDs(ctx context.Context, accountIDs []vo.Id, filter model.TransactionFilter) ([]*model.Transaction, error)
	GetByID(ctx context.Context, id vo.Id) (*model.Transaction, error)
	LabelsByTransactionIDs(ctx context.Context, ids []vo.Id) (map[string][]string, error)
}

func (l *ImportsTransactionLister) GetByID(ctx context.Context, id vo.Id) (*model.Transaction, error) {
	t, err := l.txns.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	labels, err := l.txns.LabelsByTransactionIDs(ctx, []vo.Id{id})
	if err != nil {
		return nil, err
	}
	t.LabelIDs = t.LabelIDs[:0]
	for _, raw := range labels[id.String()] {
		lid, err := vo.ParseId(raw)
		if err != nil {
			return nil, err
		}
		t.LabelIDs = append(t.LabelIDs, lid)
	}
	return t, nil
}
```

(Check first whether the transaction repo's `GetByID` already fills `LabelIDs` — `grep -n LabelIDs internal/transaction/repo/repo.go`. If it does, the adapter is a plain passthrough and the `LabelsByTransactionIDs` requirement is dropped from the interface.)

Extend the writer source + adapter:

```go
type importsTransactionWriterSource interface {
	CreateTransaction(ctx context.Context, userID vo.Id, req model.CreateTransactionRequest) (*model.CreateTransactionResult, error)
	UpdateTransaction(ctx context.Context, userID vo.Id, req model.UpdateTransactionRequest) (*model.UpdateTransactionResult, error)
	UpdateTransactionPreservingLabels(ctx context.Context, userID vo.Id, req model.UpdateTransactionRequest) (*model.UpdateTransactionResult, error)
}

func (a importsTransactionAdapter) UpdateTransactionReplacingLabels(ctx context.Context, userID vo.Id, req model.UpdateTransactionRequest) (*model.UpdateTransactionResult, error) {
	return a.svc.UpdateTransaction(ctx, userID, req)
}
```

In `internal/server/server.go` the `NewService` call becomes:

```go
	importsSvc := appimports.NewService(
		importsRepo,
		NewImportsAccountReader(accountSvc, currencyLookup),
		NewImportsCurrencyConverter(currencyLookup, rateProvider, convertor),
		NewImportsTransactionWriter(transactionSvc),
		NewImportsTransactionLister(transactionRepo),
		NewImportsClassificationLister(txImportCategories.CategoriesByOwner, txImportPayees.PayeesByOwner, txImportTags.TagsByOwner, txImportLabels.LabelsByOwner),
		authLimiter, txm, clk,
		appimports.MatcherConfig{ /* unchanged */ },
	)
```

Note the `txImport*` adapters are constructed at `server.go:324-328`, above this call — no reordering needed.

- [ ] **Step 4: Update the test harnesses**

`internal/imports/ingest_test.go`: the harness gains `entities *fakeEntities` and both `NewService` calls (lines ~196 and ~203) pass `h.entities` after `h.txns` (the lister arg). Add to the file:

```go
// fakeEntities is the owner's vocabulary as the rules engine sees it. Tests
// register ids they want a rule to be allowed to target.
type fakeEntities struct {
	categories, payees, tags, labels map[vo.Id][]model.ImportNamed // by owner
}

func newFakeEntities() *fakeEntities {
	return &fakeEntities{categories: map[vo.Id][]model.ImportNamed{}, payees: map[vo.Id][]model.ImportNamed{}, tags: map[vo.Id][]model.ImportNamed{}, labels: map[vo.Id][]model.ImportNamed{}}
}

func (f *fakeEntities) add(m map[vo.Id][]model.ImportNamed, owner vo.Id, id, name string) {
	m[owner] = append(m[owner], model.ImportNamed{ID: id, Name: name, OwnerID: owner.String()})
}

func (f *fakeEntities) CategoriesByOwner(_ context.Context, o vo.Id) ([]model.ImportNamed, error) { return f.categories[o], nil }
func (f *fakeEntities) PayeesByOwner(_ context.Context, o vo.Id) ([]model.ImportNamed, error)     { return f.payees[o], nil }
func (f *fakeEntities) TagsByOwner(_ context.Context, o vo.Id) ([]model.ImportNamed, error)       { return f.tags[o], nil }
func (f *fakeEntities) LabelsByOwner(_ context.Context, o vo.Id) ([]model.ImportNamed, error)     { return f.labels[o], nil }
```

`fakeTxns` gains:

```go
func (f *fakeTxns) GetByID(ctx context.Context, id vo.Id) (*model.Transaction, error)
func (f *fakeTxns) UpdateTransactionReplacingLabels(ctx context.Context, userID vo.Id, req model.UpdateTransactionRequest) (*model.UpdateTransactionResult, error)
```

`GetByID` reads the row `fakeTxns` inserted (it writes real `transactions` rows — read back `id, type, account_id, amount, category_id, payee_id, tag_id, description, spent_at` with `db.Raw.QueryRowContext` + `db.Rebind`, and the label ids from `transactions_labels WHERE transaction_id = ?`), returning `errs.NewNotFound("Transaction not found")` on `sql.ErrNoRows`. `UpdateTransactionReplacingLabels` appends to a new `replaced []model.UpdateTransactionRequest` slice, rewrites the row's `category_id/payee_id/tag_id/description`, and replaces `transactions_labels` rows (DELETE then INSERT per id) so `GetByID` observes the change; `CreateTransaction` must already persist `CategoryId/PayeeId/TagId/LabelIds` from the request the same way — extend it if it drops them today (check `grep -n "INSERT INTO transactions" internal/imports/ingest_test.go`).

`internal/imports/api/harness_test.go:109`: pass a `fakeEntities` between `txns` and `nil`, and keep it on the harness (`entities *fakeEntities` field, `entities: entities` in the returned struct) so the rule-route tests (Task 10) can declare owned ids:

```go
// owned ids as plain slices; every id listed counts as user A's
type fakeEntities struct{ categories, payees, tags, labels []string }

func named(ids []string) []model.ImportNamed {
	out := make([]model.ImportNamed, 0, len(ids))
	for _, id := range ids {
		out = append(out, model.ImportNamed{ID: id, Name: id, OwnerID: userA})
	}
	return out
}

func (f *fakeEntities) CategoriesByOwner(context.Context, vo.Id) ([]model.ImportNamed, error) { return named(f.categories), nil }
func (f *fakeEntities) PayeesByOwner(context.Context, vo.Id) ([]model.ImportNamed, error)     { return named(f.payees), nil }
func (f *fakeEntities) TagsByOwner(context.Context, vo.Id) ([]model.ImportNamed, error)       { return named(f.tags), nil }
func (f *fakeEntities) LabelsByOwner(context.Context, vo.Id) ([]model.ImportNamed, error)     { return named(f.labels), nil }
```
 `txns` there also needs the two new methods (same approach — that file's fake is smaller; give `GetByID` a real read and `UpdateTransactionReplacingLabels` a recording no-op plus row update).

- [ ] **Step 5: Build and run the existing suites**

Run: `go build ./... && go vet ./internal/imports/... ./internal/server/... && go test ./internal/imports/... ./internal/server/... ./internal/test/archtest/...`
Expected: PASS — this task changes wiring only; every existing test keeps passing (the golden suites are untouched).

- [ ] **Step 6: Commit**

```bash
git add internal/imports/ports.go internal/imports/service.go internal/imports/ingest_test.go internal/imports/api/harness_test.go internal/server
git commit -m "feat(imports): classification, lister, and completer ports for rules

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Skip rules in the pipeline

**Files:**
- Modify: `internal/imports/rules.go` (add `loadRules`)
- Modify: `internal/imports/ingest.go` (`resolution`, `resolve`, `applyEvent`)
- Modify: `internal/imports/accountlink.go` (`convertQueued`)
- Test: `internal/imports/ingest_test.go`

**Interfaces:**
- Consumes: `ruleSet`, `newRuleSet` (Task 4); `Repository.ListRulesByUser` (Task 3); `ClassificationLister` (Task 5).
- Produces:

```go
// loadRules reads the owner's rules once per request and filters every
// target/label against the owner's current vocabulary.
func (s *Service) loadRules(ctx context.Context, src *model.ImportSource) (ruleSet, error)

type resolution struct {
	accountID  vo.Id
	amount     string
	status     string
	skipRuleID *vo.Id // set with status=Skipped when a skip rule fired
}
func (s *Service) resolve(ctx context.Context, src *model.ImportSource, ev model.IngestEvent, rules ruleSet) (resolution, error)
func (s *Service) applyEvent(ctx context.Context, src *model.ImportSource, eventID vo.Id, ev model.IngestEvent, runID *vo.Id, correctAmount bool, rules ruleSet) (status string, amountUpdated bool, err error)
```

Callers: `processEvent` (ingest + retry) loads rules then calls `applyEvent`; `Sync` loads rules once before the account loop and `syncAccount` takes a `rules ruleSet` parameter; `convertQueued` loads once before its loop.

- [ ] **Step 1: Write the failing tests**

Append to `internal/imports/ingest_test.go`:

```go
func TestIngest_SkipRuleSkipsAfterMapping(t *testing.T) {
	h := setup(t)
	h.mapCard(t, "Apple Card")
	h.f.ImportRule(fixture.ImportRule{UserID: userA, Action: "skip", MatchField: "external_payee", MatchType: "prefix", MatchValue: "payment - thank you"})
	body := strings.Replace(tap, `"payee":"Blue Bottle"`, `"payee":"PAYMENT - THANK YOU"`, 1)
	res := ingest(t, h, body)
	if res.Status != model.ImportIngestStatusSkipped {
		t.Fatalf("status = %s", res.Status)
	}
	if len(h.txns.created) != 0 {
		t.Fatalf("a skipped event must create nothing: %+v", h.txns.created)
	}
	links, _ := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	if len(links) != 1 || links[0].Status != model.ImportLinkStatusSkipped || links[0].AppliedRuleID == nil {
		t.Fatalf("ledger row must be skipped with the rule recorded: %+v", links)
	}
}

func TestIngest_SkipRuleDoesNotFireOnUnmappedCard(t *testing.T) {
	h := setup(t)
	h.f.ImportRule(fixture.ImportRule{UserID: userA, Action: "skip", MatchField: "external_payee", MatchType: "contains", MatchValue: "blue bottle"})
	res := ingest(t, h, tap) // card never mapped
	if res.Status != model.ImportIngestStatusQueued {
		t.Fatalf("unmapped card queues even when a skip rule would match: %s", res.Status)
	}
}

func TestIngest_SkipRuleScopedToAnotherSourceIsIgnored(t *testing.T) {
	h := setup(t)
	h.mapCard(t, "Apple Card")
	other := h.f.ImportSource(fixture.ImportSource{UserID: userA, Provider: model.ImportProviderSimpleFIN, Name: "Bank"})
	h.f.ImportRule(fixture.ImportRule{UserID: userA, SourceID: other, Action: "skip", MatchField: "external_payee", MatchType: "contains", MatchValue: "blue bottle"})
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("rule scoped to another source must not fire: %s", res.Status)
	}
}

func TestLinkAccount_ReplayAppliesSkipRules(t *testing.T) {
	h := setup(t)
	h.f.ImportRule(fixture.ImportRule{UserID: userA, Action: "skip", MatchField: "external_payee", MatchType: "contains", MatchValue: "blue bottle"})
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusQueued {
		t.Fatalf("queued first: %s", res.Status)
	}
	res, err := h.svc.LinkAccount(context.Background(), vo.MustParseId(userA), model.LinkImportAccountRequest{SourceId: source, ExternalAccountId: "Apple Card", AccountId: acct1})
	if err != nil {
		t.Fatal(err)
	}
	if res.Run == nil || res.Run.SkippedCount != 1 || res.Run.ImportedCount != 0 {
		t.Fatalf("replay must skip via the rule: %+v", res.Run)
	}
	links, _ := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	if links[0].Status != model.ImportLinkStatusSkipped || links[0].AppliedRuleID == nil {
		t.Fatalf("replayed row: %+v", links[0])
	}
}
```

`userA`, `userB`, `acct1`, `source`, `now` and `tap` are the package-level constants the existing `ingest_test.go` harness declares (the tap fixture's payee is `Blue Bottle`, its dedupe key is `eventId`). `h.mapCard` only seeds an `import_account_links` fixture row — it does not replay the queue; `LinkAccount` does.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/imports/ -run 'TestIngest_SkipRule|TestLinkAccount_ReplayAppliesSkipRules' -v`
Expected: FAIL — status `created` where `skipped` is expected (rules are never read yet).

- [ ] **Step 3: Implement `loadRules`**

Append to `internal/imports/rules.go`:

```go
// loadRules reads the owner's rules once per request. Targets and labels
// are filtered against the owner's current vocabulary so an id deleted
// after the rule was saved never reaches CreateTransaction (which would
// reject the whole import for one stale category).
func (s *Service) loadRules(ctx context.Context, src *model.ImportSource) (ruleSet, error) {
	rules, err := s.repo.ListRulesByUser(ctx, src.UserID)
	if err != nil {
		return ruleSet{}, err
	}
	if len(rules) == 0 {
		return ruleSet{}, nil
	}
	own, err := s.ownedEntities(ctx, src.UserID)
	if err != nil {
		return ruleSet{}, err
	}
	for i := range rules {
		r := &rules[i]
		if r.TargetCategoryID != nil && !own.categories[*r.TargetCategoryID] {
			r.TargetCategoryID = nil
		}
		if r.TargetPayeeID != nil && !own.payees[*r.TargetPayeeID] {
			r.TargetPayeeID = nil
		}
		if r.TargetTagID != nil && !own.tags[*r.TargetTagID] {
			r.TargetTagID = nil
		}
		kept := r.LabelIDs[:0]
		for _, l := range r.LabelIDs {
			if own.labels[l] {
				kept = append(kept, l)
			}
		}
		r.LabelIDs = kept
	}
	return newRuleSet(rules, src.ID), nil
}

// ownedIDs is the owner's vocabulary as id sets.
type ownedIDs struct {
	categories, payees, tags, labels map[vo.Id]bool
}

func (s *Service) ownedEntities(ctx context.Context, userID vo.Id) (ownedIDs, error) {
	var out ownedIDs
	var err error
	if out.categories, err = idSet(s.entities.CategoriesByOwner(ctx, userID)); err != nil {
		return out, err
	}
	if out.payees, err = idSet(s.entities.PayeesByOwner(ctx, userID)); err != nil {
		return out, err
	}
	if out.tags, err = idSet(s.entities.TagsByOwner(ctx, userID)); err != nil {
		return out, err
	}
	if out.labels, err = idSet(s.entities.LabelsByOwner(ctx, userID)); err != nil {
		return out, err
	}
	return out, nil
}

func idSet(items []model.ImportNamed, err error) (map[vo.Id]bool, error) {
	if err != nil {
		return nil, err
	}
	out := make(map[vo.Id]bool, len(items))
	for _, it := range items {
		id, err := vo.ParseId(it.ID)
		if err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, nil
}
```

- [ ] **Step 4: Thread the rule set through `resolve` and `applyEvent`**

In `internal/imports/ingest.go`:

```go
type resolution struct {
	accountID  vo.Id
	amount     string
	status     string  // "" = import; ImportIngestStatusQueued / ImportIngestStatusSkipped otherwise
	skipRuleID *vo.Id  // with status=Skipped: the skip rule that fired (nil = ignored card)
}

func (s *Service) resolve(ctx context.Context, src *model.ImportSource, ev model.IngestEvent, rules ruleSet) (resolution, error) {
	// ... unchanged through the currency conversion ...
	// A skip rule acts only on an event that could otherwise import: the
	// card is mapped and the amount converted. Unmapped/no-rate events queue
	// as before so the user still sees them.
	if id := rules.skip(ev); id != nil {
		return resolution{status: model.ImportIngestStatusSkipped, skipRuleID: id}, nil
	}
	return resolution{accountID: *al.AccountID, amount: amount}, nil
}
```

In `applyEvent` (signature gains the trailing `rules ruleSet`), the skipped branch becomes:

```go
	case model.ImportIngestStatusSkipped:
		link.Status = model.ImportLinkStatusSkipped
		link.AppliedRuleID = r.skipRuleID
		return r.status, false, s.repo.InsertLink(ctx, link)
```

`processEvent`:

```go
	rules, err := s.loadRules(ctx, src)
	if err != nil {
		return "", err
	}
	status, _, err := s.applyEvent(ctx, src, ev.ID, parsed, nil, false, rules)
```

`sync.go`: after `pullSource` succeeds, `rules, err := s.loadRules(ctx, src)` (return the error), `syncAccount(fin, src, run, a, byAccount[a.ID], rules)` and inside it `s.applyEvent(ctx, src, ev.ID, parsed, &run.ID, true, rules)`.

`accountlink.go` `convertQueued`: load once before the loop, pass to `s.resolve(ctx, src, ev, rules)`, and turn the existing `r.status != ""` branch into:

```go
		r, err := s.resolve(ctx, src, ev, rules)
		if err != nil {
			return nil, err
		}
		if r.status == model.ImportIngestStatusSkipped && r.skipRuleID != nil {
			l.Status, l.AppliedRuleID, l.RunID = model.ImportLinkStatusSkipped, r.skipRuleID, &run.ID
			if err := s.repo.UpdateLink(ctx, &l); err != nil {
				return nil, err
			}
			run.SkippedCount++
			continue
		}
		if r.status != "" {
			run.SkippedCount++
			continue
		}
```

(The pre-existing "still not importable" rows keep their status — only a rule-skip rewrites the row.)

Search for every other caller: `grep -n "s.resolve(\|s.applyEvent(" internal/imports/*.go` — all must compile with the new argument.

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/imports/... -v -run 'SkipRule|ReplayAppliesSkipRules'` then `go test ./internal/imports/... ./internal/test/apiparity/`
Expected: PASS; goldens unchanged (no scenario has rules yet).

- [ ] **Step 6: Commit**

```bash
git add internal/imports
git commit -m "feat(imports): skip rules in the ingest pipeline

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: Classify rules and the applied-classification snapshot

**Files:**
- Modify: `internal/imports/ingest.go` (`place`, `applyEvent`)
- Modify: `internal/imports/accountlink.go` (`convertQueued`)
- Modify: `internal/imports/queue.go` (`ImportQueuedEvent`, `GetTransactionImportList`)
- Test: `internal/imports/ingest_test.go`, `internal/imports/queue_test.go` (or wherever `GetTransactionImportList` is tested — `grep -rn GetTransactionImportList internal/imports/*_test.go`)

**Interfaces:**
- Consumes: `ruleSet.classify`, `TransactionLister.GetByID`, `Repository.ReplaceLinkAppliedLabels/ListLinkAppliedLabels`, `model.ImportClassification`.
- Produces:

```go
func (s *Service) place(ctx context.Context, src *model.ImportSource, ev model.IngestEvent, r resolution, correctAmount bool, rules ruleSet) (txID vo.Id, adopted bool, amountUpdated bool, applied model.ImportClassification, err error)
// writeApplied snapshots what the row's transaction carries as of this import.
func (s *Service) writeApplied(ctx context.Context, link *model.ImportTransactionLink, c model.ImportClassification) error
// snapshotOf reads a live transaction's current classification (adopted rows, apply-rule "already edited" checks).
func (s *Service) snapshotOf(ctx context.Context, txID vo.Id) (model.ImportClassification, error)
```

- [ ] **Step 1: Write the failing tests**

Append to `internal/imports/ingest_test.go`:

```go
func TestIngest_ClassifyRuleFillsCreatedTransaction(t *testing.T) {
	h := setup(t)
	h.mapCard(t, "Apple Card")
	cat := h.f.Category(fixture.Category{UserID: userA, Name: "Coffee"})
	label := h.f.Label(fixture.Label{UserID: userA, Name: "Work"})
	stale := vo.NewId().String() // a payee id the owner does not have (deleted after the rule was saved)
	h.entities.add(h.entities.categories, vo.MustParseId(userA), cat, "Coffee")
	h.entities.add(h.entities.labels, vo.MustParseId(userA), label, "Work")
	ruleID := h.f.ImportRule(fixture.ImportRule{UserID: userA, MatchValue: "blue bottle", CategoryID: cat, PayeeID: stale, LabelIDs: []string{label}})
	res := ingest(t, h, tap)
	if res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("status = %s", res.Status)
	}
	req := h.txns.created[0]
	if req.CategoryId == nil || *req.CategoryId != cat || req.PayeeId != nil || len(req.LabelIds) != 1 || req.LabelIds[0] != label {
		t.Fatalf("created request must carry the rule's live targets only: %+v", req)
	}
	links, _ := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	l := links[0]
	if l.AppliedCategoryID == nil || l.AppliedCategoryID.String() != cat || l.AppliedRuleID == nil || l.AppliedRuleID.String() != ruleID {
		t.Fatalf("applied snapshot: %+v", l)
	}
	applied, _ := h.repo.ListLinkAppliedLabels(context.Background(), l.ID)
	if len(applied) != 1 || applied[0].String() != label {
		t.Fatalf("applied labels: %v", applied)
	}
}

func TestIngest_AdoptedTransactionSnapshotsItsOwnClassification(t *testing.T) {
	h := setup(t)
	h.mapCard(t, "Apple Card")
	cat := h.f.Category(fixture.Category{UserID: userA, Name: "Coffee"})
	// a hand-entered transaction the matcher will adopt (same amount, same day)
	txID := h.txns.seed(t, acct1, "expense", "4.75", now, "Blue Bottle")
	h.db.Raw.ExecContext(context.Background(), h.db.Rebind(`UPDATE transactions SET category_id = ? WHERE id = ?`), cat, txID.String())
	res := ingest(t, h, tap)
	if res.Status != model.ImportIngestStatusMatched {
		t.Fatalf("status = %s", res.Status)
	}
	l := linkByExternalID(t, h, "evt-1") // seed() adds its own linked row on the same source; pick the ingested one
	if l.AppliedCategoryID == nil || l.AppliedCategoryID.String() != cat || l.AppliedRuleID != nil {
		t.Fatalf("adopted row snapshots the transaction's current classification, no rule: %+v", l)
	}
}

func TestImportQueuedEvent_RecordsLabels(t *testing.T) {
	h := setup(t)
	label := h.f.Label(fixture.Label{UserID: userA, Name: "Trip"})
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusQueued {
		t.Fatal("expected queued")
	}
	links, _ := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	_, err := h.svc.ImportQueuedEvent(context.Background(), vo.MustParseId(userA), model.ImportQueuedEventRequest{
		LinkId: links[0].ID.String(),
		Transaction: model.CreateTransactionRequest{Id: vo.NewId().String(), Type: "expense", Amount: vo.NewFlexString("4.75"), AccountId: acct1, Date: now.Format(datetime.Layout), LabelIds: []string{label}},
	})
	if err != nil {
		t.Fatalf("ImportQueuedEvent: %v", err)
	}
	applied, _ := h.repo.ListLinkAppliedLabels(context.Background(), links[0].ID)
	if len(applied) != 1 || applied[0].String() != label {
		t.Fatalf("applied labels: %v", applied)
	}
	list, _ := h.svc.GetTransactionImportList(context.Background(), vo.MustParseId(userA), model.TransactionImportListRequest{TransactionId: h.txns.created[0].Id})
	if len(list.Items) != 1 || len(list.Items[0].AppliedLabelIds) != 1 || list.Items[0].AppliedLabelIds[0] != label {
		t.Fatalf("provenance must expose applied labels: %+v", list.Items)
	}
}
```

Add the helper the adopt test uses (append to `ingest_test.go`):

```go
func linkByExternalID(t *testing.T, h *harness, externalTxID string) model.ImportTransactionLink {
	t.Helper()
	links, err := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range links {
		if l.ExternalTransactionID == externalTxID {
			return l
		}
	}
	t.Fatalf("no link for %s in %+v", externalTxID, links)
	return model.ImportTransactionLink{}
}
```

`ImportQueuedEvent` needs the link still `queued`: `h.mapCard` only seeds a fixture account link (no replay), and `ImportQueuedEvent` names the account explicitly anyway, so the test above maps no card at all.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/imports/ -run 'ClassifyRule|Snapshots|RecordsLabels' -v`
Expected: FAIL — created request carries no category; `AppliedLabelIds` empty.

- [ ] **Step 3: Implement**

`internal/imports/ingest.go` — `place` grows a `rules ruleSet` parameter and a fourth result:

```go
	// adopt branch, both the amount-corrected and the plain adopt returns:
	applied, err := s.snapshotOf(ctx, m.TransactionID)
	if err != nil {
		return vo.Id{}, false, false, model.ImportClassification{}, err
	}
	return m.TransactionID, true, amountUpdated, applied, nil

	// create branch:
	c := rules.classify(ev)
	res, err := s.txns.CreateTransaction(ctx, src.UserID, model.CreateTransactionRequest{
		Id: vo.NewId().String(), Type: ev.Type.Alias(), Amount: vo.NewFlexString(r.amount), AccountId: r.accountID.String(),
		Date: at.Format(datetime.Layout), Description: optionalString(ev.Payee),
		CategoryId: idString2(c.CategoryID), PayeeId: idString2(c.PayeeID), TagId: idString2(c.TagID), LabelIds: idStrings(c.LabelIDs),
	})
```

Add to `service.go`:

```go
func idStrings(ids []vo.Id) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	return out
}

func (s *Service) snapshotOf(ctx context.Context, txID vo.Id) (model.ImportClassification, error) {
	t, err := s.lister.GetByID(ctx, txID)
	if err != nil {
		return model.ImportClassification{}, err
	}
	return model.ImportClassification{CategoryID: t.CategoryID, PayeeID: t.PayeeID, TagID: t.TagID, LabelIDs: t.LabelIDs}, nil
}

// writeApplied stores the classification the row's transaction carries as
// of this import. It is the baseline the rule prompt and apply-rule's
// "already edited" check diff against, so it must reflect what was written,
// not what a rule asked for.
func (s *Service) writeApplied(ctx context.Context, link *model.ImportTransactionLink, c model.ImportClassification) error {
	link.AppliedCategoryID, link.AppliedPayeeID, link.AppliedTagID = c.CategoryID, c.PayeeID, c.TagID
	if c.RuleID != nil {
		link.AppliedRuleID = c.RuleID
	}
	return s.repo.ReplaceLinkAppliedLabels(ctx, link.ID, c.LabelIDs)
}
```

`applyEvent` linked branch:

```go
	txID, adopted, amountUpdated, applied, err := s.place(ctx, src, ev, r, correctAmount, rules)
	if err != nil {
		return "", false, err
	}
	link.Status = model.ImportLinkStatusLinked
	link.TransactionID = &txID
	link.AppliedCategoryID, link.AppliedPayeeID, link.AppliedTagID, link.AppliedRuleID = applied.CategoryID, applied.PayeeID, applied.TagID, applied.RuleID
	if err := s.repo.InsertLink(ctx, link); err != nil {
		return "", false, err
	}
	if err := s.repo.ReplaceLinkAppliedLabels(ctx, link.ID, applied.LabelIDs); err != nil {
		return "", false, err
	}
	// status as before
```

(`InsertLink` must precede the label rows — FK on `link_id`.)

`accountlink.go` `convertQueued`: the `place` call gains `rules` and its `applied` result; after `UpdateLink` call `s.repo.ReplaceLinkAppliedLabels(ctx, l.ID, applied.LabelIDs)` and set `l.AppliedCategoryID/PayeeID/TagID/AppliedRuleID` before the `UpdateLink`.

`queue.go` `ImportQueuedEvent`: after the three `parseOptionalIDField` lines, add label handling:

```go
		labelIDs := make([]vo.Id, 0, len(req.Transaction.LabelIds))
		for _, raw := range req.Transaction.LabelIds {
			if id := parseOptionalIDField(&raw); id != nil {
				labelIDs = append(labelIDs, *id)
			}
		}
		if err := s.repo.UpdateLink(ctx, link); err != nil {
			return err
		}
		if err := s.repo.ReplaceLinkAppliedLabels(ctx, link.ID, labelIDs); err != nil {
			return err
		}
```

`GetTransactionImportList`: build the result through a shared helper used by Task 9 too:

```go
func (s *Service) linkResult(ctx context.Context, src *model.ImportSource, l *model.ImportTransactionLink) (model.TransactionImportLinkResult, error) {
	labels, err := s.repo.ListLinkAppliedLabels(ctx, l.ID)
	if err != nil {
		return model.TransactionImportLinkResult{}, err
	}
	return model.TransactionImportLinkResult{
		Id: l.ID.String(), SourceId: src.ID.String(), Provider: src.Provider, SourceName: src.Name, RunId: idString(l.RunID),
		ExternalAccountId: l.ExternalAccountID, ExternalTransactionId: l.ExternalTransactionID,
		ExternalPayee: l.ExternalPayee, ExternalDescription: l.ExternalDescription,
		ExternalAmount: vo.NewDecimal(l.ExternalAmount).String(), ExternalCurrency: derefString(l.ExternalCurrency),
		ExternalPostedAt: l.ExternalPostedAt.Format(datetime.Layout), Status: l.Status, ImportedAt: l.ImportedAt.Format(datetime.Layout),
		AppliedCategoryId: idString(l.AppliedCategoryID), AppliedPayeeId: idString(l.AppliedPayeeID), AppliedTagId: idString(l.AppliedTagID),
		AppliedLabelIds: idStrings(labels), AppliedRuleId: idString(l.AppliedRuleID),
	}, nil
}
```

- [ ] **Step 4: Run the tests and regenerate the provenance golden**

Run: `go test ./internal/imports/... && UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ && git diff --stat internal/test/apiparity/testdata`
Expected: imports tests PASS; ONLY the `get-transaction-import-list` golden(s) change, each gaining the seven new keys (`runId`, `externalDescription`, `appliedCategoryId`, `appliedPayeeId`, `appliedTagId`, `appliedLabelIds: []`, `appliedRuleId`). Inspect the diff; any other golden change is a bug.

- [ ] **Step 5: Commit**

```bash
git add internal/imports internal/test/apiparity/testdata
git commit -m "feat(imports): classify rules on create and snapshot applied classification

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 8: Rule CRUD use cases

**Files:**
- Create: `internal/imports/rule.go`
- Modify: `internal/shared/errs/codes.go` (codes + `AllCodes`), `locales/*.json` (11 files, `errors.import.rule_skip_not_applicable`, `errors.import.ai_disabled`, `errors.import.ai_unavailable`)
- Test: `internal/imports/rule_test.go` (package `imports_test`, reusing the `ingest_test.go` harness)

**Interfaces:**
- Consumes: `Repository.InsertRule/UpdateRule/DeleteRule/GetRule/ListRulesByUser` (Task 3), `ClassificationLister` (Task 5), `model.ImportRuleSpec.Validate()` (Task 2), `ownedSource`.
- Produces:

```go
func (s *Service) GetRuleList(ctx context.Context, userID vo.Id) (*model.GetImportRuleListResult, error)
func (s *Service) CreateRule(ctx context.Context, userID vo.Id, req model.CreateImportRuleRequest) (*model.ImportRuleResult, error)
func (s *Service) UpdateRule(ctx context.Context, userID vo.Id, req model.UpdateImportRuleRequest) (*model.ImportRuleResult, error)
func (s *Service) DeleteRule(ctx context.Context, userID vo.Id, req model.DeleteImportRuleRequest) error
func (s *Service) ownedRule(ctx context.Context, userID vo.Id, rawID string) (*model.ImportRule, error)   // uncoded errs.NewNotFound("Import rule not found") — NotFoundError maps to HTTP 400, like every other import lookup
func (s *Service) ruleFromSpec(ctx context.Context, userID vo.Id, spec model.ImportRuleSpec) (*model.ImportRule, error) // ownership checks; coded field errors
func ruleResult(r *model.ImportRule) model.ImportRuleResult
```

Error codes (add to `internal/shared/errs/codes.go` after `CodeImportAccessUrlInvalid`, and to `AllCodes`):

```go
	CodeImportRuleSkipNotApplicable = "import.rule_skip_not_applicable"
	CodeImportAiDisabled            = "import.ai_disabled"
	CodeImportAiUnavailable         = "import.ai_unavailable"
```

Catalogue text (`en`; translate for the other ten — the i18ntest parity guard fails until every catalogue has all four keys, so add all three now even though the `ai_*` ones are only raised in Task 11). Not-found stays UNCODED (`errs.NewNotFound`, no catalogue key), exactly like `Import source not found` today:

```json
"errors": {
  "import": {
    "rule_skip_not_applicable": "A skip rule cannot be applied to existing transactions",
    "ai_disabled": "AI suggestions are not enabled on this server",
    "ai_unavailable": "The AI service is unavailable. Try again later."
  }
}
```

(Nest them under the existing `errors.import` object — check how the current `errors.import.source_not_found` key is laid out in `locales/en.json` and follow it exactly.)

- [ ] **Step 1: Write the failing tests**

`internal/imports/rule_test.go`:

```go
package imports_test

import (
	"context"
	"errors"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

func classifyReq(h *harness, cat string) model.CreateImportRuleRequest {
	return model.CreateImportRuleRequest{Id: vo.NewId().String(), ImportRuleSpec: model.ImportRuleSpec{
		Action: model.ImportRuleActionClassify, MatchField: model.ImportRuleMatchFieldExternalPayee, MatchType: model.ImportRuleMatchTypeContains,
		MatchValue: "Blue Bottle", CategoryId: &cat, Priority: 5,
	}}
}

func TestRules_CreateListUpdateDelete(t *testing.T) {
	h := setup(t)
	uid := vo.MustParseId(userA)
	cat := h.f.Category(fixture.Category{UserID: userA, Name: "Coffee"})
	label := h.f.Label(fixture.Label{UserID: userA, Name: "Work"})
	h.entities.add(h.entities.categories, uid, cat, "Coffee")
	h.entities.add(h.entities.labels, uid, label, "Work")

	req := classifyReq(h, cat)
	req.LabelIds = []string{label}
	created, err := h.svc.CreateRule(context.Background(), uid, req)
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}
	if created.Id != req.Id || created.CategoryId != cat || len(created.LabelIds) != 1 || created.SourceId != "" {
		t.Fatalf("created: %+v", created)
	}

	list, err := h.svc.GetRuleList(context.Background(), uid)
	if err != nil || len(list.Items) != 1 {
		t.Fatalf("list: %v %+v", err, list)
	}

	upd := model.UpdateImportRuleRequest{Id: req.Id, ImportRuleSpec: req.ImportRuleSpec}
	upd.MatchValue = "STARBUCKS COFFEE"
	upd.LabelIds = nil
	updated, err := h.svc.UpdateRule(context.Background(), uid, upd)
	if err != nil || updated.MatchValue != "STARBUCKS COFFEE" || len(updated.LabelIds) != 0 {
		t.Fatalf("update: %v %+v", err, updated)
	}

	if err := h.svc.DeleteRule(context.Background(), uid, model.DeleteImportRuleRequest{Id: req.Id}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	list, _ = h.svc.GetRuleList(context.Background(), uid)
	if len(list.Items) != 0 {
		t.Fatalf("rule must be gone: %+v", list.Items)
	}
}

func TestRules_RejectsForeignTargets(t *testing.T) {
	h := setup(t)
	uid := vo.MustParseId(userA)
	foreignCat := h.f.Category(fixture.Category{UserID: userB, Name: "Theirs"})
	h.entities.add(h.entities.categories, vo.MustParseId(userB), foreignCat, "Theirs")
	_, err := h.svc.CreateRule(context.Background(), uid, classifyReq(h, foreignCat))
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || !hasField(ve, "categoryId") {
		t.Fatalf("a category the caller does not own must be a categoryId validation error, got %v", err)
	}
	other := h.f.ImportSource(fixture.ImportSource{UserID: userB, Provider: model.ImportProviderSimpleFIN, Name: "Theirs"})
	req := classifyReq(h, "")
	req.CategoryId = nil
	req.TagId = nil
	req.SourceId = &other
	req.Action = model.ImportRuleActionSkip
	if _, err := h.svc.CreateRule(context.Background(), uid, req); !isNotFound(err) {
		t.Fatalf("another user's source must be not-found, got %v", err)
	}
}

func TestRules_OtherUsersRuleIsNotFound(t *testing.T) {
	h := setup(t)
	id := h.f.ImportRule(fixture.ImportRule{UserID: userB, MatchValue: "x"})
	err := h.svc.DeleteRule(context.Background(), vo.MustParseId(userA), model.DeleteImportRuleRequest{Id: id})
	if !isNotFound(err) {
		t.Fatalf("got %v", err)
	}
	list, _ := h.svc.GetRuleList(context.Background(), vo.MustParseId(userA))
	if len(list.Items) != 0 {
		t.Fatal("must not list another user's rule")
	}
}
```

`userA`/`userB` are the package-level id constants of `ingest_test.go`. `internal/shared/errs` exports no `IsNotFound`; `NotFoundError` is a plain uncoded type and `ValidationError.Fields` is a `[]errs.FieldError` slice (`Key`, `Message`, `Code`). Add these two helpers to `rule_test.go` (later test files in this package reuse them):

```go
func isNotFound(err error) bool {
	var nf *errs.NotFoundError
	return errors.As(err, &nf)
}

func hasField(ve *errs.ValidationError, key string) bool {
	for _, f := range ve.Fields {
		if f.Key == key {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/imports/ -run TestRules_ -v`
Expected: FAIL — `h.svc.CreateRule undefined`.

- [ ] **Step 3: Implement**

`internal/imports/rule.go`:

```go
package imports

import (
	"context"
	"strings"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

const ruleNotFoundMessage = "Import rule not found"

func (s *Service) GetRuleList(ctx context.Context, userID vo.Id) (*model.GetImportRuleListResult, error) {
	rules, err := s.repo.ListRulesByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := &model.GetImportRuleListResult{Items: make([]model.ImportRuleResult, 0, len(rules))}
	for i := range rules {
		out.Items = append(out.Items, ruleResult(&rules[i]))
	}
	return out, nil
}

func (s *Service) CreateRule(ctx context.Context, userID vo.Id, req model.CreateImportRuleRequest) (*model.ImportRuleResult, error) {
	id, err := vo.ParseId(strings.TrimSpace(req.Id))
	if err != nil {
		return nil, errs.NewValidation("Validation failed", errs.FieldError{Key: "id", Message: "This value is not valid.", Code: errs.CodeInvalidUUID})
	}
	r, err := s.ruleFromSpec(ctx, userID, req.ImportRuleSpec)
	if err != nil {
		return nil, err
	}
	now := s.clk.Now().UTC()
	r.ID, r.UserID, r.CreatedAt, r.UpdatedAt = id, userID, now, now
	reqctx.AddLogAttr(ctx, "rule_id", id.String())
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error { return s.repo.InsertRule(ctx, r) }); err != nil {
		return nil, err
	}
	out := ruleResult(r)
	return &out, nil
}

func (s *Service) UpdateRule(ctx context.Context, userID vo.Id, req model.UpdateImportRuleRequest) (*model.ImportRuleResult, error) {
	existing, err := s.ownedRule(ctx, userID, req.Id)
	if err != nil {
		return nil, err
	}
	r, err := s.ruleFromSpec(ctx, userID, req.ImportRuleSpec)
	if err != nil {
		return nil, err
	}
	r.ID, r.UserID, r.CreatedAt, r.UpdatedAt = existing.ID, userID, existing.CreatedAt, s.clk.Now().UTC()
	reqctx.AddLogAttr(ctx, "rule_id", r.ID.String())
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error { return s.repo.UpdateRule(ctx, r) }); err != nil {
		return nil, err
	}
	out := ruleResult(r)
	return &out, nil
}

func (s *Service) DeleteRule(ctx context.Context, userID vo.Id, req model.DeleteImportRuleRequest) error {
	r, err := s.ownedRule(ctx, userID, req.Id)
	if err != nil {
		return err
	}
	reqctx.AddLogAttr(ctx, "rule_id", r.ID.String())
	return s.repo.DeleteRule(ctx, r.ID)
}

func (s *Service) ownedRule(ctx context.Context, userID vo.Id, rawID string) (*model.ImportRule, error) {
	id, err := vo.ParseId(strings.TrimSpace(rawID))
	if err != nil {
		return nil, errs.NewNotFound(ruleNotFoundMessage)
	}
	r, err := s.repo.GetRule(ctx, id)
	if err != nil {
		return nil, err
	}
	if r.UserID != userID {
		return nil, errs.NewNotFound(ruleNotFoundMessage)
	}
	return r, nil
}

// ruleFromSpec turns a validated spec into an entity, checking that every
// referenced id is the caller's: the source must be owned (not-found
// otherwise, like every other source lookup), and a category/payee/tag/label
// the caller does not own is a per-field validation error.
func (s *Service) ruleFromSpec(ctx context.Context, userID vo.Id, spec model.ImportRuleSpec) (*model.ImportRule, error) {
	r := &model.ImportRule{
		Action: spec.Action, MatchField: spec.MatchField, MatchType: spec.MatchType,
		MatchValue: strings.TrimSpace(spec.MatchValue), IsCaseSensitive: spec.IsCaseSensitive, Priority: spec.Priority,
	}
	if spec.SourceId != nil && strings.TrimSpace(*spec.SourceId) != "" {
		src, err := s.ownedSource(ctx, userID, *spec.SourceId)
		if err != nil {
			return nil, err
		}
		r.SourceID = &src.ID
	}
	if spec.Action == model.ImportRuleActionSkip {
		return r, nil
	}
	own, err := s.ownedEntities(ctx, userID)
	if err != nil {
		return nil, err
	}
	var fields []errs.FieldError
	pick := func(field string, raw *string, set map[vo.Id]bool) *vo.Id {
		if raw == nil || strings.TrimSpace(*raw) == "" {
			return nil
		}
		id, err := vo.ParseId(strings.TrimSpace(*raw))
		if err != nil || !set[id] {
			fields = append(fields, errs.FieldError{Key: field, Message: "This value is not valid.", Code: errs.CodeInvalidChoice})
			return nil
		}
		return &id
	}
	r.TargetCategoryID = pick("categoryId", spec.CategoryId, own.categories)
	r.TargetPayeeID = pick("payeeId", spec.PayeeId, own.payees)
	r.TargetTagID = pick("tagId", spec.TagId, own.tags)
	for _, raw := range spec.LabelIds {
		if id := pick("labelIds", &raw, own.labels); id != nil {
			r.LabelIDs = append(r.LabelIDs, *id)
		}
	}
	if len(fields) > 0 {
		return nil, errs.NewValidation("Validation failed", fields...)
	}
	return r, nil
}

func ruleResult(r *model.ImportRule) model.ImportRuleResult {
	return model.ImportRuleResult{
		Id: r.ID.String(), SourceId: idString(r.SourceID), Action: r.Action, MatchField: r.MatchField, MatchType: r.MatchType,
		MatchValue: r.MatchValue, IsCaseSensitive: r.IsCaseSensitive,
		CategoryId: idString(r.TargetCategoryID), PayeeId: idString(r.TargetPayeeID), TagId: idString(r.TargetTagID),
		LabelIds: idStrings(r.LabelIDs), Priority: r.Priority,
		CreatedAt: r.CreatedAt.Format(datetime.Layout), UpdatedAt: r.UpdatedAt.Format(datetime.Layout),
	}
}
```

These constructors are the real `internal/shared/errs` API: `errs.NewNotFound(msg)` (uncoded; `httpx` renders it as HTTP 400), `errs.NewValidation(msg, fields...)` with `errs.FieldError{Key, Message, Code}`, and the common codes `errs.CodeInvalidUUID` / `errs.CodeInvalidChoice`. `"Validation failed"` + `"This value is not valid."` is the wording `internal/model/imports_dto.go` already uses for field errors. `idString`/`idStrings` come from `service.go` (Task 7 added `idStrings`); `datetime` is `internal/shared/datetime`.

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/imports/... ./internal/test/i18ntest/...`
Expected: PASS (i18ntest passes only once all 11 catalogues carry the three new `errors.import.*` keys).

- [ ] **Step 5: Commit**

```bash
git add internal/imports/rule.go internal/imports/rule_test.go internal/shared/errs/codes.go locales
git commit -m "feat(imports): rule CRUD use cases

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 9: Preview and apply a rule against existing imports

**Files:**
- Create: `internal/imports/ruleapply.go`
- Test: `internal/imports/ruleapply_test.go`

**Interfaces:**
- Consumes: `Repository.ListLinksByRun/ListLinksBySource/ListLinksByUser/GetRun/UpdateLink/ReplaceLinkAppliedLabels/ListLinkAppliedLabels`, `matchRule`, `TransactionLister.GetByID`, `TransactionWriter.UpdateTransactionReplacingLabels`, `snapshotOf`, `ownedRule`, `ruleFromSpec`.
- Produces:

```go
func (s *Service) PreviewRule(ctx context.Context, userID vo.Id, req model.PreviewImportRuleRequest) (*model.PreviewImportRuleResult, error)
func (s *Service) ApplyRule(ctx context.Context, userID vo.Id, req model.ApplyImportRuleRequest) (*model.ApplyImportRuleResult, error)
// scopedLinks resolves scope=run|source|all to the caller's LINKED rows (status linked, transaction set).
func (s *Service) scopedLinks(ctx context.Context, userID vo.Id, scope, runID, sourceID string) ([]model.ImportTransactionLink, error)
// linkEvent rebuilds the matcher's view of a stored row.
func linkEvent(l *model.ImportTransactionLink) model.IngestEvent
// edited reports whether the live transaction's classification differs from the row's applied snapshot.
func edited(applied model.ImportClassification, live *model.Transaction) bool
```

Semantics (spec Part 7):
- Preview counts every scoped row whose `linkEvent` matches the spec; `alreadyEdited` is the subset whose live transaction differs from `applied_*`. A skip spec previews fine (counts only).
- Apply refuses a skip rule (`import.rule_skip_not_applicable`, 400). For each matching row it skips already-edited ones unless `IncludeEdited`; otherwise writes the rule's targets over the live transaction (only the fields the rule sets; labels = union of live + rule, capped at `MaxImportRuleLabels`), then updates the row's `applied_*`, applied labels and `applied_rule_id`. `Updated`/`Skipped` count rows written / rows left alone because edited.
- A row whose transaction no longer exists (`errs.NotFound` from `GetByID`) is ignored in both.

- [ ] **Step 1: Write the failing tests**

`internal/imports/ruleapply_test.go`:

```go
package imports_test

import (
	"context"
	"errors"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

// two imported Blue Bottle rows on the seeded source, one of them hand-edited afterwards
func seedTwoImports(t *testing.T, h *harness) (edited, untouched vo.Id) {
	t.Helper()
	h.mapCard(t, "Apple Card")
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("first: %s", res.Status)
	}
	second := strings.Replace(tap, `"eventId":"evt-1"`, `"eventId":"evt-2"`, 1)
	if res := ingest(t, h, second); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("second: %s", res.Status)
	}
	links, _ := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	other := h.f.Category(fixture.Category{UserID: userA, Name: "Groceries"})
	h.db.Raw.ExecContext(context.Background(), h.db.Rebind(`UPDATE transactions SET category_id = ? WHERE id = ?`), other, links[0].TransactionID.String())
	return links[0].ID, links[1].ID
}

func TestPreviewRule_CountsMatchesAndEdited(t *testing.T) {
	h := setup(t)
	seedTwoImports(t, h)
	cat := h.f.Category(fixture.Category{UserID: userA, Name: "Coffee"})
	h.entities.add(h.entities.categories, vo.MustParseId(userA), cat, "Coffee")
	res, err := h.svc.PreviewRule(context.Background(), vo.MustParseId(userA), model.PreviewImportRuleRequest{
		ImportRuleSpec: classifyReq(h, cat).ImportRuleSpec, Scope: model.ImportRuleScopeSource, ScopeSourceId: source,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Matched != 2 || res.AlreadyEdited != 1 {
		t.Fatalf("got %+v", res)
	}
	none, _ := h.svc.PreviewRule(context.Background(), vo.MustParseId(userA), model.PreviewImportRuleRequest{
		ImportRuleSpec: model.ImportRuleSpec{Action: "skip", MatchField: "external_payee", MatchType: "exact", MatchValue: "nothing"},
		Scope: model.ImportRuleScopeAll,
	})
	if none.Matched != 0 {
		t.Fatalf("got %+v", none)
	}
}

func TestApplyRule_SkipsEditedUnlessIncluded(t *testing.T) {
	h := setup(t)
	editedID, untouchedID := seedTwoImports(t, h)
	uid := vo.MustParseId(userA)
	cat := h.f.Category(fixture.Category{UserID: userA, Name: "Coffee"})
	label := h.f.Label(fixture.Label{UserID: userA, Name: "Work"})
	h.entities.add(h.entities.categories, uid, cat, "Coffee")
	h.entities.add(h.entities.labels, uid, label, "Work")
	req := classifyReq(h, cat)
	req.LabelIds = []string{label}
	rule, err := h.svc.CreateRule(context.Background(), uid, req)
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.svc.ApplyRule(context.Background(), uid, model.ApplyImportRuleRequest{RuleId: rule.Id, Scope: model.ImportRuleScopeAll})
	if err != nil || res.Updated != 1 || res.Skipped != 1 {
		t.Fatalf("first apply: %v %+v", err, res)
	}
	if len(h.txns.replaced) != 1 || *h.txns.replaced[0].CategoryId != cat || len(h.txns.replaced[0].LabelIds) != 1 {
		t.Fatalf("untouched row rewritten with the rule's targets: %+v", h.txns.replaced)
	}
	l, _ := h.repo.GetLink(context.Background(), untouchedID)
	if l.AppliedCategoryID == nil || l.AppliedCategoryID.String() != cat || l.AppliedRuleID == nil || l.AppliedRuleID.String() != rule.Id {
		t.Fatalf("applied snapshot after apply: %+v", l)
	}
	res, err = h.svc.ApplyRule(context.Background(), uid, model.ApplyImportRuleRequest{RuleId: rule.Id, Scope: model.ImportRuleScopeAll, IncludeEdited: true})
	if err != nil || res.Updated != 2 || res.Skipped != 0 {
		t.Fatalf("include-edited apply: %v %+v", err, res)
	}
	e, _ := h.repo.GetLink(context.Background(), editedID)
	if e.AppliedCategoryID == nil || e.AppliedCategoryID.String() != cat {
		t.Fatalf("edited row now applied: %+v", e)
	}
}

func TestApplyRule_RefusesSkipRulesAndForeignScope(t *testing.T) {
	h := setup(t)
	uid := vo.MustParseId(userA)
	skip := h.f.ImportRule(fixture.ImportRule{UserID: userA, Action: "skip", MatchValue: "x"})
	_, err := h.svc.ApplyRule(context.Background(), uid, model.ApplyImportRuleRequest{RuleId: skip, Scope: model.ImportRuleScopeAll})
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.MsgCode != errs.CodeImportRuleSkipNotApplicable {
		t.Fatalf("got %v", err)
	}
	theirs := h.f.ImportSource(fixture.ImportSource{UserID: userB, Provider: model.ImportProviderSimpleFIN, Name: "Theirs"})
	rule := h.f.ImportRule(fixture.ImportRule{UserID: userA, MatchValue: "x"})
	if _, err := h.svc.ApplyRule(context.Background(), uid, model.ApplyImportRuleRequest{RuleId: rule, Scope: model.ImportRuleScopeSource, ScopeSourceId: theirs}); !isNotFound(err) {
		t.Fatalf("foreign source scope must be not-found: %v", err)
	}
}
```

`h.repo.GetLink(ctx, id)` is the repo's single-link getter (`repository.go`). The tap fixture is keyed by `"eventId":"evt-1"` (that string, no space after the colon), which is what the `strings.Replace` needle above targets. `isNotFound`/`hasField` are the helpers Task 8 added to `rule_test.go`. Add `"strings"` to the imports.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/imports/ -run 'TestPreviewRule|TestApplyRule' -v`
Expected: FAIL — `PreviewRule`/`ApplyRule` undefined.

- [ ] **Step 3: Implement**

`internal/imports/ruleapply.go`:

```go
package imports

import (
	"context"
	"errors"
	"strings"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *Service) PreviewRule(ctx context.Context, userID vo.Id, req model.PreviewImportRuleRequest) (*model.PreviewImportRuleResult, error) {
	r, err := s.ruleFromSpec(ctx, userID, req.ImportRuleSpec)
	if err != nil {
		return nil, err
	}
	links, err := s.scopedLinks(ctx, userID, req.Scope, req.RunId, req.ScopeSourceId)
	if err != nil {
		return nil, err
	}
	out := &model.PreviewImportRuleResult{}
	for i := range links {
		l := &links[i]
		if !matchRule(*r, linkEvent(l)) {
			continue
		}
		live, err := s.lister.GetByID(ctx, *l.TransactionID)
		if isNotFoundErr(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out.Matched++
		if edited(s.appliedOf(ctx, l), live) {
			out.AlreadyEdited++
		}
	}
	return out, nil
}

func (s *Service) ApplyRule(ctx context.Context, userID vo.Id, req model.ApplyImportRuleRequest) (*model.ApplyImportRuleResult, error) {
	rule, err := s.ownedRule(ctx, userID, req.RuleId)
	if err != nil {
		return nil, err
	}
	if rule.Action == model.ImportRuleActionSkip {
		return nil, &errs.ValidationError{Msg: "A skip rule cannot be applied to existing transactions", MsgCode: errs.CodeImportRuleSkipNotApplicable}
	}
	reqctx.AddLogAttr(ctx, "rule_id", rule.ID.String())
	// Filter targets the same way the pipeline does, so a stale id never
	// reaches the transaction service.
	set, err := s.loadRules(ctx, &model.ImportSource{UserID: userID})
	if err != nil {
		return nil, err
	}
	for i := range set.rules {
		if set.rules[i].ID == rule.ID {
			rule = &set.rules[i]
		}
	}
	links, err := s.scopedLinks(ctx, userID, req.Scope, req.RunId, req.ScopeSourceId)
	if err != nil {
		return nil, err
	}
	out := &model.ApplyImportRuleResult{}
	for i := range links {
		l := &links[i]
		if !matchRule(*rule, linkEvent(l)) {
			continue
		}
		live, err := s.lister.GetByID(ctx, *l.TransactionID)
		if isNotFoundErr(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !req.IncludeEdited && edited(s.appliedOf(ctx, l), live) {
			out.Skipped++
			continue
		}
		next := applyTargets(rule, live)
		err = s.tx.WithTx(ctx, func(ctx context.Context) error {
			if _, err := s.txns.UpdateTransactionReplacingLabels(ctx, userID, updateRequest(live, next)); err != nil {
				return err
			}
			next.RuleID = &rule.ID
			l.AppliedCategoryID, l.AppliedPayeeID, l.AppliedTagID, l.AppliedRuleID = next.CategoryID, next.PayeeID, next.TagID, next.RuleID
			if err := s.repo.UpdateLink(ctx, l); err != nil {
				return err
			}
			return s.repo.ReplaceLinkAppliedLabels(ctx, l.ID, next.LabelIDs)
		})
		if err != nil {
			return nil, err
		}
		out.Updated++
	}
	reqctx.AddLogAttr(ctx, "updated", out.Updated)
	reqctx.AddLogAttr(ctx, "skipped", out.Skipped)
	return out, nil
}

// scopedLinks narrows to rows that are linked to a live transaction: a
// queued or skipped row has nothing to rewrite and a preview must not count it.
func (s *Service) scopedLinks(ctx context.Context, userID vo.Id, scope, runID, sourceID string) ([]model.ImportTransactionLink, error) {
	var links []model.ImportTransactionLink
	var err error
	switch scope {
	case model.ImportRuleScopeRun:
		id, perr := vo.ParseId(strings.TrimSpace(runID))
		if perr != nil {
			return nil, errs.NewNotFound("Import run not found")
		}
		run, gerr := s.repo.GetRun(ctx, id)
		if gerr != nil {
			return nil, gerr
		}
		if run.UserID != userID {
			return nil, errs.NewNotFound("Import run not found")
		}
		links, err = s.repo.ListLinksByRun(ctx, run.ID)
	case model.ImportRuleScopeSource:
		src, serr := s.ownedSource(ctx, userID, sourceID)
		if serr != nil {
			return nil, serr
		}
		links, err = s.repo.ListLinksBySource(ctx, src.ID)
	default:
		links, err = s.repo.ListLinksByUser(ctx, userID)
	}
	if err != nil {
		return nil, err
	}
	linked := links[:0]
	for _, l := range links {
		if l.Status == model.ImportLinkStatusLinked && l.TransactionID != nil {
			linked = append(linked, l)
		}
	}
	return linked, nil
}

func linkEvent(l *model.ImportTransactionLink) model.IngestEvent {
	return model.IngestEvent{Payee: l.ExternalPayee, Description: l.ExternalDescription}
}

func (s *Service) appliedOf(ctx context.Context, l *model.ImportTransactionLink) model.ImportClassification {
	labels, _ := s.repo.ListLinkAppliedLabels(ctx, l.ID) // a read failure reads as "no labels applied": the diff then reports edited, the safe side
	return model.ImportClassification{CategoryID: l.AppliedCategoryID, PayeeID: l.AppliedPayeeID, TagID: l.AppliedTagID, LabelIDs: labels, RuleID: l.AppliedRuleID}
}

// edited is the "already edited" test: any classification field the user
// changed since the import means a rule must not silently overwrite it.
func edited(applied model.ImportClassification, live *model.Transaction) bool {
	return !sameID(applied.CategoryID, live.CategoryID) || !sameID(applied.PayeeID, live.PayeeID) || !sameID(applied.TagID, live.TagID) || !sameIDs(applied.LabelIDs, live.LabelIDs)
}

// isNotFoundErr: a linked transaction the user has since deleted is simply
// not a candidate, not a failure of the whole preview/apply.
func isNotFoundErr(err error) bool {
	var nf *errs.NotFoundError
	return errors.As(err, &nf)
}

func sameID(a, b *vo.Id) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func sameIDs(a, b []vo.Id) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[vo.Id]bool, len(a))
	for _, id := range a {
		seen[id] = true
	}
	for _, id := range b {
		if !seen[id] {
			return false
		}
	}
	return true
}

// applyTargets is what the transaction looks like after the rule: only the
// fields the rule sets change; labels union, capped.
func applyTargets(r *model.ImportRule, live *model.Transaction) model.ImportClassification {
	next := model.ImportClassification{CategoryID: live.CategoryID, PayeeID: live.PayeeID, TagID: live.TagID, LabelIDs: append([]vo.Id(nil), live.LabelIDs...)}
	if r.TargetCategoryID != nil {
		next.CategoryID = r.TargetCategoryID
	}
	if r.TargetPayeeID != nil {
		next.PayeeID = r.TargetPayeeID
	}
	if r.TargetTagID != nil {
		next.TagID = r.TargetTagID
	}
	seen := make(map[vo.Id]bool, len(next.LabelIDs))
	for _, id := range next.LabelIDs {
		seen[id] = true
	}
	for _, id := range r.LabelIDs {
		if !seen[id] && len(next.LabelIDs) < model.MaxImportRuleLabels {
			seen[id] = true
			next.LabelIDs = append(next.LabelIDs, id)
		}
	}
	return next
}

func updateRequest(live *model.Transaction, next model.ImportClassification) model.UpdateTransactionRequest {
	req := model.UpdateTransactionRequest{
		Id: live.ID.String(), Type: live.Type.Alias(), AccountId: live.AccountID.String(), Amount: vo.NewFlexString(live.Amount),
		Date: live.SpentAt.Format(datetime.Layout), Description: optionalString(live.Description),
		CategoryId: idString2(next.CategoryID), PayeeId: idString2(next.PayeeID), TagId: idString2(next.TagID), LabelIds: idStrings(next.LabelIDs),
		AccountRecipientId: idString2(live.AccountRecipID),
	}
	if live.AmountRecipient != nil {
		req.AmountRecipient = vo.NewFlexString(*live.AmountRecipient)
	}
	return req
}
```

Add `"github.com/econumo/econumo/internal/shared/datetime"` to the imports. `updateRequest` mirrors the tip-adopt request in `place` (`internal/imports/ingest.go`) exactly — copy the field mapping from there if any name differs (`AmountRecipient` type, `Type.Alias()`), the two must stay in step. `ImportRuleScopeAll` is the default branch so `validateRuleScope` (Task 2) is the only gate on the scope literal.

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/imports/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/imports/ruleapply.go internal/imports/ruleapply_test.go
git commit -m "feat(imports): preview and apply rules to existing imports

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 10: HTTP edge — seven rule routes, goldens, OpenAPI

**Files:**
- Create: `internal/imports/api/rule.go`
- Modify: `internal/imports/api/routes.go` (21 → 28 routes)
- Modify: `internal/test/apiparity/scenarios_imports.go` (or wherever the `import/` scenarios live — `grep -rln "get-transaction-import-list" internal/test/apiparity/*.go`)
- Modify: `internal/test/apiparity/guard_test.go` (route/scenario floor counts)
- Test: `internal/imports/api/rule_test.go`
- Regenerate: `docs/` OpenAPI via `make swagger`; goldens via `UPDATE_GOLDEN=1`

**Interfaces:**
- Consumes: Task 8/9 service methods; `endpoint.Handle`, `endpoint.HandleNoBody`; the `Handlers` struct in `internal/imports/api/handlers.go`.
- Produces routes:

| Method | Path | Handler | Request → Result |
|---|---|---|---|
| GET | `/api/v1/import/get-rule-list` | `GetRuleList` | – → `GetImportRuleListResult` |
| POST | `/api/v1/import/create-rule` | `CreateRule` | `CreateImportRuleRequest` → `ImportRuleResult` |
| POST | `/api/v1/import/update-rule` | `UpdateRule` | `UpdateImportRuleRequest` → `ImportRuleResult` |
| POST | `/api/v1/import/delete-rule` | `DeleteRule` | `DeleteImportRuleRequest` → `{}` |
| POST | `/api/v1/import/preview-rule` | `PreviewRule` | `PreviewImportRuleRequest` → `PreviewImportRuleResult` |
| POST | `/api/v1/import/apply-rule` | `ApplyRule` | `ApplyImportRuleRequest` → `ApplyImportRuleResult` |
| POST | `/api/v1/import/suggest-rules` | `SuggestRules` | `SuggestImportRulesRequest` → `SuggestImportRulesResult` (service method lands in Task 11; register the route now with a handler that calls `h.svc.SuggestRules` — Task 11 adds the method; until then the handler calls a stub `func (s *Service) SuggestRules(ctx, userID, req) (*model.SuggestImportRulesResult, error) { return nil, &errs.ValidationError{Msg: "AI suggestions are not enabled on this server", MsgCode: errs.CodeImportAiDisabled} }` added in THIS task to `internal/imports/suggest.go`, which Task 11 replaces) |

`preview-rule` is a POST *read* by the same rule as `list-external-accounts`: the whole rule spec travels in the body. Every route is authenticated, full-scope (the ingest scope allowlist is unchanged), and subject to the 402 readonly gate like every other POST.

- [ ] **Step 1: Write the failing handler tests**

`internal/imports/api/rule_test.go`. Requests go through the existing `call(t, h, method, path, body) (status int, env map[string]any)` helper in `sync_test.go` (bearer = user A); a Go `NotFoundError` renders as HTTP **400** in this codebase (`httpx`), so the second delete asserts 400:

```go
package api_test

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/fixture"
)

func TestRuleRoutes_CreateListDelete(t *testing.T) {
	h := newHarness(t)
	cat := h.f.Category(fixture.Category{UserID: userA, Name: "Coffee"})
	h.entities.categories = []string{cat}
	id := "0192b1e4-0000-7000-8000-000000000001"
	status, env := call(t, h, "POST", "/api/v1/import/create-rule", map[string]any{
		"id": id, "action": "classify", "matchField": "external_payee", "matchType": "contains", "matchValue": "Blue Bottle", "categoryId": cat, "priority": 1,
	})
	if status != http.StatusOK {
		t.Fatalf("create: %d %v", status, env)
	}
	status, env = call(t, h, "GET", "/api/v1/import/get-rule-list", nil)
	items := env["data"].(map[string]any)["items"].([]any)
	if status != http.StatusOK || len(items) != 1 || items[0].(map[string]any)["id"] != id {
		t.Fatalf("list: %d %v", status, env)
	}
	if status, env = call(t, h, "POST", "/api/v1/import/delete-rule", map[string]any{"id": id}); status != http.StatusOK {
		t.Fatalf("delete: %d %v", status, env)
	}
	status, env = call(t, h, "POST", "/api/v1/import/delete-rule", map[string]any{"id": id})
	if status != http.StatusBadRequest || env["message"] != "Import rule not found" {
		t.Fatalf("second delete: %d %v", status, env)
	}
}

func TestRuleRoutes_ValidationAndScope(t *testing.T) {
	h := newHarness(t)
	status, env := call(t, h, "POST", "/api/v1/import/create-rule", map[string]any{
		"id": "0192b1e4-0000-7000-8000-000000000002", "action": "skip", "matchField": "external_payee", "matchType": "contains", "matchValue": "x", "categoryId": "anything",
	})
	if status != http.StatusBadRequest || env["errors"].(map[string]any)["categoryId"] == nil {
		t.Fatalf("skip with a target must fail on categoryId: %d %v", status, env)
	}
	status, env = call(t, h, "POST", "/api/v1/import/preview-rule", map[string]any{
		"action": "classify", "matchField": "description", "matchType": "prefix", "matchValue": "x", "scope": "run",
	})
	if status != http.StatusBadRequest || env["errors"].(map[string]any)["runId"] == nil {
		t.Fatalf("scope=run without runId: %d %v", status, env)
	}
}
```

(Ingest-scope gating is enforced by the auth middleware, not per route, and is already covered by the ingest tests — no assertion here.)

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/imports/api/ -run TestRuleRoutes -v`
Expected: FAIL — 404 (routes not registered).

- [ ] **Step 3: Implement handlers and routes**

`internal/imports/api/rule.go` (swag blocks follow the existing handlers in this package, e.g. `run.go`; copy their `@Tags`, `@Security`, envelope `@Success`/`@Failure` lines verbatim and change the summary/route/types):

```go
package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/web/endpoint"
)

// GetRuleList godoc
// @Summary Import rules
// @Tags import
// @Security BearerAuth
// @Produce json
// @Success 200 {object} httpx.Envelope{data=model.GetImportRuleListResult}
// @Router /api/v1/import/get-rule-list [get]
func (h *Handlers) GetRuleList(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.svc.GetRuleList)
}

// CreateRule godoc
// @Summary Create an import rule
// @Tags import
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body model.CreateImportRuleRequest true "rule"
// @Success 200 {object} httpx.Envelope{data=model.ImportRuleResult}
// @Router /api/v1/import/create-rule [post]
func (h *Handlers) CreateRule(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.CreateRule)
}

// UpdateRule godoc
// @Summary Update an import rule
// @Tags import
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body model.UpdateImportRuleRequest true "rule"
// @Success 200 {object} httpx.Envelope{data=model.ImportRuleResult}
// @Router /api/v1/import/update-rule [post]
func (h *Handlers) UpdateRule(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.UpdateRule)
}

// DeleteRule godoc
// @Summary Delete an import rule
// @Tags import
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body model.DeleteImportRuleRequest true "id"
// @Success 200 {object} httpx.Envelope{data=object}
// @Router /api/v1/import/delete-rule [post]
func (h *Handlers) DeleteRule(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, func(ctx context.Context, userID vo.Id, req model.DeleteImportRuleRequest) (struct{}, error) {
		return struct{}{}, h.svc.DeleteRule(ctx, userID, req)
	})
}

// PreviewRule godoc
// @Summary Count the existing imports a rule would touch
// @Tags import
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body model.PreviewImportRuleRequest true "rule spec + scope"
// @Success 200 {object} httpx.Envelope{data=model.PreviewImportRuleResult}
// @Router /api/v1/import/preview-rule [post]
func (h *Handlers) PreviewRule(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.PreviewRule)
}

// ApplyRule godoc
// @Summary Apply a saved rule to existing imports
// @Tags import
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body model.ApplyImportRuleRequest true "rule id + scope"
// @Success 200 {object} httpx.Envelope{data=model.ApplyImportRuleResult}
// @Router /api/v1/import/apply-rule [post]
func (h *Handlers) ApplyRule(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.ApplyRule)
}

// SuggestRules godoc
// @Summary Ask the configured AI for rule suggestions
// @Tags import
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body model.SuggestImportRulesRequest true "scope"
// @Success 200 {object} httpx.Envelope{data=model.SuggestImportRulesResult}
// @Router /api/v1/import/suggest-rules [post]
func (h *Handlers) SuggestRules(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.SuggestRules)
}
```

Check how the existing delete-style handlers in this package return an empty payload (`grep -n "struct{}" internal/imports/api/*.go`) and match it; if `DeleteImportRuleRequest` needs a `Validate()` for `endpoint.Handle`, Task 2 gave it one. Add `context`/`vo` imports as needed.

`routes.go`: after the last existing `mux.Handle...` line, register the seven routes with the same helper the others use (same method-prefix style, e.g. `"GET /api/v1/import/get-rule-list"`). Update the route-count comment/doc if one exists in the file.

`internal/imports/suggest.go` (stub, replaced in Task 11):

```go
package imports

func (s *Service) SuggestRules(ctx context.Context, userID vo.Id, req model.SuggestImportRulesRequest) (*model.SuggestImportRulesResult, error) {
	return nil, &errs.ValidationError{Msg: "AI suggestions are not enabled on this server", MsgCode: errs.CodeImportAiDisabled}
}
```

- [ ] **Step 4: Run the handler tests**

Run: `go test ./internal/imports/api/`
Expected: PASS.

- [ ] **Step 5: apiparity scenarios**

Every route needs a scenario or the guard test fails. Add to the imports scenario file (mirror the neighbouring scenario structs — the seeded user/source ids are in that file's fixture setup; `preview`/`apply` scenarios must reference a source the seeded user owns):

- `import/get-rule-list` (GET, empty list)
- `import/create-rule` (classify, `contains`, category from the fixture, fixed id `0192b1e4-0000-7000-8000-00000000c001`)
- `import/update-rule` (same id, new match value)
- `import/preview-rule` (scope `all`)
- `import/apply-rule` (rule id above, scope `all` → `{updated:0, skipped:0}`)
- `import/delete-rule`
- `import/suggest-rules` (→ 400 `import.ai_disabled` envelope)
- plus the two error scenarios that make coded errors visible in the goldens: `create-rule` with a skip action carrying `categoryId` (400, field `categoryId`) and `delete-rule` with an unknown id (400 — `NotFoundError` renders as 400 here — message `Import rule not found`).

Order matters: the parity runner replays scenarios in file order, so `create-rule` precedes `update/apply/delete`.

Then raise the floors in `internal/test/apiparity/guard_test.go` (route count +7, scenario count +9 — the exact constants are named there; read the failing assertion message and set them to the printed actual values), regenerate, and inspect:

Run: `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ && git status --short internal/test/apiparity/testdata && go test ./internal/test/apiparity/`
Expected: exactly nine new golden files, no modified ones; the second run PASSES.

- [ ] **Step 6: OpenAPI**

Run: `make swagger && git status --short docs/`
Expected: the committed OpenAPI docs gain the seven paths + the new DTO schemas; `make go-lint` passes (docs fresh).

- [ ] **Step 7: Commit**

```bash
git add internal/imports internal/test/apiparity docs
git commit -m "feat(imports): rule routes with parity scenarios and OpenAPI

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 11: AI-suggested rules (`ECONUMO_AI_DSN`, completion client, `suggest-rules`)

**Files:**
- Create: `internal/infra/ai/client.go`, `internal/infra/ai/client_test.go`
- Modify: `internal/config/config.go` (struct fields, `Load`, rate-limit table, new `parseAIDSN`), `internal/config/config_test.go`
- Modify: `internal/server/server.go` (rate scope + `SetCompleter` wiring)
- Modify: `internal/web/router/router.go:200-230` (`AI_ENABLED` key), `internal/web/router/router_test.go` (goldens), `web/public/econumo-config.js`, `web/src/lib/config.ts`
- Modify: `.env.example`
- Replace: `internal/imports/suggest.go` (the Task 10 stub); create `internal/imports/suggest_test.go`

**Interfaces:**
- Consumes: `Completer` + `Service.SetCompleter` + `Service.completer` + `RateScopeSuggestRules` (Task 5); `scopedLinks`, `isNotFoundErr` (Task 9); `ownedEntities`/`ownedIDs` (Task 6); `model.SuggestImportRulesRequest`, `model.SuggestImportRulesResult{Items []model.ImportRuleSuggestion}`, `model.ImportRuleSpec.Validate()` (Task 2); `errs.CodeImportAiDisabled`, `errs.CodeImportAiUnavailable` (Task 8); `seedTwoImports`, `linkByExternalID`, `withLimiter` (Tasks 7/9 test helpers).
- Produces: `ai.Config{Endpoint, APIKey, Model string}`, `ai.New(cfg) *ai.Client`, `(*ai.Client).Complete(ctx, system, user string) (string, error)`; `config.Config.{AIDSN, AIEnabled, AIEndpoint, AIAPIKey, AIModel, RateLimitSuggestRules}`; `config.parseAIDSN`; served config key `AI_ENABLED` (bool, always present); `isAiEnabled()` in `web/src/lib/config.ts` (Task 13 hides the "Suggest rules" button on it).

Design rulings (from the spec, Part 7): the model sees names and ids of the user's categories/payees/tags/labels plus imported payee/description strings with what the user chose — nothing else (no amounts, no account names, no dates); exactly ONE completion request per call; the response is transient (nothing persists; accepted rows go through `create-rule`); every proposed row is re-validated server-side and rows with hallucinated ids, unknown match types, or `action != classify` are dropped silently. The chat-completions request sends no `response_format` (Ollama/vLLM/older gateways reject it); JSON is requested in the prompt and parsed leniently (first `{` … last `}`). The client has NO SSRF guard: the endpoint is admin-configured through the environment, not user input.

- [ ] **Step 1: Write the failing client test**

`internal/infra/ai/client_test.go`:

```go
package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestComplete_PostsChatCompletionAndReturnsContent(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"{\"rules\":[]}"}}]}`)
	}))
	defer srv.Close()

	c := New(Config{Endpoint: srv.URL + "/v1", APIKey: "sk-test", Model: "gpt-5-mini"})
	out, err := c.Complete(context.Background(), "SYS", "USER")
	if err != nil {
		t.Fatal(err)
	}
	if out != `{"rules":[]}` {
		t.Fatalf("content = %q", out)
	}
	if gotPath != "/v1/chat/completions" || gotAuth != "Bearer sk-test" {
		t.Fatalf("path=%q auth=%q", gotPath, gotAuth)
	}
	if gotBody["model"] != "gpt-5-mini" {
		t.Fatalf("model = %v", gotBody["model"])
	}
	msgs := gotBody["messages"].([]any)
	if len(msgs) != 2 || msgs[0].(map[string]any)["role"] != "system" || msgs[1].(map[string]any)["content"] != "USER" {
		t.Fatalf("messages = %v", msgs)
	}
	if _, has := gotBody["response_format"]; has {
		t.Fatal("response_format must not be sent (local servers reject it)")
	}
}

func TestComplete_KeylessSendsNoAuthorization(t *testing.T) {
	var gotAuth string
	var hasAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, hasAuth = r.Header.Get("Authorization"), len(r.Header.Values("Authorization")) > 0
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer srv.Close()
	if _, err := New(Config{Endpoint: srv.URL, Model: "llama3"}).Complete(context.Background(), "s", "u"); err != nil {
		t.Fatal(err)
	}
	if hasAuth || gotAuth != "" {
		t.Fatalf("keyless client sent Authorization %q", gotAuth)
	}
}

func TestComplete_ErrorsNeverCarryTheBodyOrKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"bad key sk-test leaked"}}`)
	}))
	defer srv.Close()
	_, err := New(Config{Endpoint: srv.URL, APIKey: "sk-test", Model: "m"}).Complete(context.Background(), "s", "u")
	if err == nil {
		t.Fatal("expected an error on 401")
	}
	if strings.Contains(err.Error(), "sk-test") || strings.Contains(err.Error(), "leaked") {
		t.Fatalf("error leaks response body or key: %v", err)
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("error should name the status: %v", err)
	}
}

func TestComplete_MalformedAndEmptyReplies(t *testing.T) {
	for _, body := range []string{`not json`, `{"choices":[]}`} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, body)
		}))
		_, err := New(Config{Endpoint: srv.URL, Model: "m"}).Complete(context.Background(), "s", "u")
		srv.Close()
		if err == nil {
			t.Fatalf("body %q: expected an error", body)
		}
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/infra/ai/`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Write the client**

`internal/infra/ai/client.go`:

```go
// Package ai is the OpenAI-compatible chat-completions client behind
// import-rule suggestions. It speaks the lowest common denominator of the
// API (model + messages + temperature, no response_format, no tools) so
// the same client works against api.openai.com, Ollama, vLLM, and gateways.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// maxResponseBytes bounds what a misbehaving endpoint can make us buffer.
const maxResponseBytes = 1 << 20

type Config struct {
	Endpoint string // base URL including the API prefix, e.g. https://api.openai.com/v1
	APIKey   string // empty for keyless local servers
	Model    string
}

type Client struct {
	cfg  Config
	http *http.Client
}

func New(cfg Config) *Client {
	return &Client{cfg: cfg, http: &http.Client{Timeout: 90 * time.Second}}
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type request struct {
	Model       string    `json:"model"`
	Messages    []message `json:"messages"`
	Temperature float64   `json:"temperature"`
}

type response struct {
	Choices []struct {
		Message message `json:"message"`
	} `json:"choices"`
}

// Complete sends one system+user exchange and returns the assistant text.
// Errors carry the HTTP status only: the response body may echo the prompt
// (the user's payee strings) or the key, and errors end up in logs.
func (c *Client) Complete(ctx context.Context, system, user string) (string, error) {
	body, err := json.Marshal(request{Model: c.cfg.Model, Messages: []message{{Role: "system", Content: system}, {Role: "user", Content: user}}})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.cfg.Endpoint, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("ai: request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return "", fmt.Errorf("ai: reading response: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("ai: completion endpoint returned HTTP %d", resp.StatusCode)
	}
	var out response
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", errors.New("ai: malformed completion response")
	}
	if len(out.Choices) == 0 {
		return "", errors.New("ai: completion returned no choices")
	}
	return out.Choices[0].Message.Content, nil
}
```

Note: `http.Client.Do` wraps the URL into `*url.Error`, which prints the full URL — and the URL never contains the key (it travels in the header), so `%w` there is safe.

- [ ] **Step 4: Run the client tests**

Run: `go test ./internal/infra/ai/`
Expected: PASS (4 tests).

- [ ] **Step 5: Write the failing config tests**

Append to `internal/config/config_test.go` (next to `TestParseMailerDSN`):

```go
func TestParseAIDSN(t *testing.T) {
	cases := []struct {
		name, dsn, endpoint, apiKey, model string
		wantErr                            bool
	}{
		{name: "empty disables", dsn: ""},
		{name: "openai with key", dsn: "openai://sk-abc@api.openai.com?model=gpt-5-mini", endpoint: "https://api.openai.com/v1", apiKey: "sk-abc", model: "gpt-5-mini"},
		{name: "scheme is case-insensitive", dsn: "OpenAI://sk-abc@api.openai.com?model=m", endpoint: "https://api.openai.com/v1", apiKey: "sk-abc", model: "m"},
		{name: "keyless loopback is plain http", dsn: "openai://localhost:11434?model=llama3", endpoint: "http://localhost:11434/v1", model: "llama3"},
		{name: "127.0.0.1 is plain http", dsn: "openai://127.0.0.1:8000?model=m", endpoint: "http://127.0.0.1:8000/v1", model: "m"},
		{name: "custom prefix kept, trailing slash trimmed", dsn: "openai://k@gateway.example/openai/v1/?model=m", endpoint: "https://gateway.example/openai/v1", apiKey: "k", model: "m"},
		{name: "insecure flag forces http on a LAN host", dsn: "openai://10.0.0.5:8080?model=m&insecure=true", endpoint: "http://10.0.0.5:8080/v1", model: "m"},
		{name: "unknown scheme", dsn: "anthropic://k@api.anthropic.com?model=m", wantErr: true},
		{name: "missing model", dsn: "openai://k@api.openai.com", wantErr: true},
		{name: "missing host", dsn: "openai://k@?model=m", wantErr: true},
		{name: "not a url", dsn: "::nope", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			endpoint, apiKey, model, err := parseAIDSN(tc.dsn)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if endpoint != tc.endpoint || apiKey != tc.apiKey || model != tc.model {
				t.Fatalf("got (%q, %q, %q), want (%q, %q, %q)", endpoint, apiKey, model, tc.endpoint, tc.apiKey, tc.model)
			}
		})
	}
}

func TestLoad_AIDSN(t *testing.T) {
	t.Setenv("DATABASE_URL", "sqlite:///tmp/x.sqlite")
	t.Setenv("ECONUMO_AI_DSN", "openai://sk-abc@api.openai.com?model=gpt-5-mini")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !c.AIEnabled || c.AIEndpoint != "https://api.openai.com/v1" || c.AIAPIKey != "sk-abc" || c.AIModel != "gpt-5-mini" {
		t.Fatalf("ai config = %+v", c)
	}
	t.Setenv("ECONUMO_AI_DSN", "")
	c, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.AIEnabled {
		t.Fatal("empty DSN must disable AI")
	}
	t.Setenv("ECONUMO_AI_DSN", "smtp://x")
	if _, err := Load(); err == nil {
		t.Fatal("bad scheme must fail boot")
	}
}
```

And extend `TestLoad_RateLimitDefaults` (line ~118): change its assertion to also cover the new scope:

```go
	if c.RateLimitClaimSetupToken != 5 || c.RateLimitSync != 10 || c.RateLimitSuggestRules != 3 {
		t.Fatalf("import defaults = %d/%d/%d", c.RateLimitClaimSetupToken, c.RateLimitSync, c.RateLimitSuggestRules)
	}
```

(Keep whatever the existing assertion already checks; only add the `RateLimitSuggestRules != 3` clause and its `%d` — read the current lines first.)

- [ ] **Step 6: Run the config tests to verify they fail**

Run: `go test ./internal/config/ -run 'TestParseAIDSN|TestLoad_AIDSN|TestLoad_RateLimitDefaults'`
Expected: FAIL — `parseAIDSN`, `AIEnabled`, `RateLimitSuggestRules` undefined.

- [ ] **Step 7: Add the config fields, parser, and rate limit**

In `internal/config/config.go`:

1. In the `Config` struct, after the `RateLimitSync` field (line ~68), add:

```go
	RateLimitSuggestRules    int // ECONUMO_RATE_LIMIT_SUGGEST_RULES (every call is a paid completion)
```

2. After the mail block (`MailReplyTo`, line ~77), add:

```go
	// AI — DERIVED from ECONUMO_AI_DSN (openai://<key>@host[:port][/prefix]?model=…).
	// Empty disables import-rule suggestions: suggest-rules answers a coded
	// 400 and the SPA hides the action (AI_ENABLED in econumo-config.js).
	AIDSN      string
	AIEnabled  bool
	AIEndpoint string // https://host/v1 (plain http for loopback hosts or ?insecure=true)
	AIAPIKey   string
	AIModel    string
```

3. In `Load`, right after the `parseMailerDSN` block (line ~144-148), add:

```go
	c.AIDSN = getEnv("ECONUMO_AI_DSN", "")
	if c.AIEndpoint, c.AIAPIKey, c.AIModel, err = parseAIDSN(c.AIDSN); err != nil {
		return nil, err
	}
	c.AIEnabled = c.AIEndpoint != ""
```

(If `Load` declares `err` later with `:=`, hoist a `var err error` or use a local `:=` here — match the surrounding style; the mail block above it shows how the existing code does it.)

4. In the rate-limit table (line ~280-292), after `{&c.RateLimitSync, "ECONUMO_RATE_LIMIT_SYNC", 10},` add:

```go
		{&c.RateLimitSuggestRules, "ECONUMO_RATE_LIMIT_SUGGEST_RULES", 3},
```

5. Below `parseMailerDSN` (line ~372), add:

```go
// parseAIDSN maps ECONUMO_AI_DSN to the chat-completions base endpoint the
// way parseMailerDSN maps MAILER_DSN: the scheme picks the API dialect (only
// the OpenAI-compatible one exists), the userinfo is the key, the host is
// the server, and the model is mandatory because no default is right for
// both a hosted vendor and a local runtime.
//
//	(empty)                                            -> disabled (all empty)
//	openai://<api-key>@api.openai.com?model=gpt-5-mini -> https://api.openai.com/v1, key, model
//	openai://localhost:11434?model=llama3              -> http://localhost:11434/v1, keyless (loopback = plain http)
//	openai://host/custom/v1?model=m&insecure=true      -> http://host/custom/v1 (insecure forces plain http elsewhere)
func parseAIDSN(dsn string) (endpoint, apiKey, model string, err error) {
	if dsn == "" {
		return "", "", "", nil
	}
	u, err := url.Parse(dsn)
	if err != nil {
		return "", "", "", fmt.Errorf("ECONUMO_AI_DSN: %w", err)
	}
	if strings.ToLower(u.Scheme) != "openai" {
		return "", "", "", fmt.Errorf("ECONUMO_AI_DSN: unsupported scheme %q (want openai://)", u.Scheme)
	}
	if u.Host == "" || u.Hostname() == "" {
		return "", "", "", errors.New("ECONUMO_AI_DSN: host is required")
	}
	q := u.Query()
	model = strings.TrimSpace(q.Get("model"))
	if model == "" {
		return "", "", "", errors.New("ECONUMO_AI_DSN: model query parameter is required")
	}
	if u.User != nil {
		apiKey = u.User.Username()
	}
	scheme := "https"
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || host == "127.0.0.1" || host == "::1" || q.Get("insecure") == "true" {
		scheme = "http"
	}
	path := strings.TrimRight(u.Path, "/")
	if path == "" {
		path = "/v1"
	}
	return scheme + "://" + u.Host + path, apiKey, model, nil
}
```

Add `"errors"` to the file's imports if it is not already there (`fmt`, `net/url`, `strings` are — `parseMailerDSN` uses them).

- [ ] **Step 8: Run the config tests**

Run: `go test ./internal/config/`
Expected: PASS.

- [ ] **Step 9: Document the variables in `.env.example`**

After the line `#ECONUMO_RATE_LIMIT_SYNC=10              # bank syncs per user per rate-limit window (0 = off)` (line ~88) add:

```
#ECONUMO_RATE_LIMIT_SUGGEST_RULES=3      # AI rule suggestions per user per rate-limit window (0 = off)

# AI-suggested import rules (optional). The DSN selects an OpenAI-compatible
# chat-completions server the way MAILER_DSN selects the mail provider:
# openai://<api-key>@<host>[:port][/prefix]?model=<model>. Unset = the feature
# is off (suggest-rules returns a coded error, the SPA hides the button). The
# server sends the model ONLY the names/ids of your categories, payees, tags
# and labels plus imported payee/description strings paired with what you
# chose — never amounts, accounts, or dates.
#ECONUMO_AI_DSN=openai://<api-key>@api.openai.com?model=gpt-5-mini
# Local runtimes need no key; loopback hosts are dialed over plain http:
#ECONUMO_AI_DSN=openai://localhost:11434?model=llama3.1
```

- [ ] **Step 10: Wire the server: rate scope, completer, served config**

`internal/server/server.go`:

1. In the `ratelimit.Config{Limits: map[string]int{...}}` literal (line ~176-193), after `appimports.RateScopeSync: cfg.RateLimitSync,` add:

```go
			appimports.RateScopeSuggestRules: cfg.RateLimitSuggestRules,
```

2. After `importsSvc := appimports.NewService(...)` and its `RegisterParser`/`RegisterProvider` lines (before `importsHandlers := handlerimports.NewHandlers(importsSvc)`, line ~366), add:

```go
	if cfg.AIEnabled {
		importsSvc.SetCompleter(ai.New(ai.Config{Endpoint: cfg.AIEndpoint, APIKey: cfg.AIAPIKey, Model: cfg.AIModel}))
	}
```

with the import `"github.com/econumo/econumo/internal/infra/ai"`.

`internal/web/router/router.go` (the `overrides` map, line ~200-230): add the key

```go
		"AI_ENABLED": deps.Cfg.AIEnabled,
```

(`deps.Cfg` is the `*config.Config` the router already reads `BILLING_URL`/`ALLOW_REGISTRATION` from — use whatever field name that map's neighbours use.)

`internal/web/router/router_test.go`:
- `TestRuntimeConfigOverrides` (line ~200-232): the `want` literal is a JSON object with keys in alphabetical order; `AI_ENABLED` sorts before `ALLOW_CUSTOM_API`, so the string becomes `window.econumoConfig = {"AI_ENABLED":false,"ALLOW_CUSTOM_API":false,"ALLOW_REGISTRATION":false,...` (insert `"AI_ENABLED":false,` right after the `{`). If the test's config fixture sets `AIEnabled: true`, use `true` instead — it does not, so `false`.
- `TestRuntimeConfigOverrides_UnsetKeysGetDefaults` (line ~260): add `"AI_ENABLED":false` to its `want` list.

`web/public/econumo-config.js` (the object at lines 12-21): add `AI_ENABLED: false,` before `ALLOW_CUSTOM_API: true,` — the fallback baseline for `pnpm dev` without a backend and the app's bundled WebView; a served instance always overwrites it.

`web/src/lib/config.ts`:
- In `EconumoConfig` add `AI_ENABLED?: boolean` (next to `BILLING_URL?: string`, line ~18).
- Next to `getBillingUrl()` (line ~136-140) add:

```ts
export function isAiEnabled(): boolean {
  return window.econumoConfig?.AI_ENABLED === true
}
```

- [ ] **Step 11: Run the router + server + config gates**

Run: `go build ./... && go test ./internal/web/router/ ./internal/server/ ./internal/config/`
Expected: PASS. If `TestRuntimeConfigOverrides` still fails, compare the printed `got` against `want`: the only diff must be the new leading `"AI_ENABLED":false,` key.

- [ ] **Step 12: Write the failing suggest tests**

`internal/imports/suggest_test.go`:

```go
package imports

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

type fakeCompleter struct {
	reply    string
	err      error
	calls    int
	lastUser string
}

func (f *fakeCompleter) Complete(_ context.Context, _ string, user string) (string, error) {
	f.calls++
	f.lastUser = user
	return f.reply, f.err
}

func codeOf(t *testing.T, err error) string {
	t.Helper()
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *errs.ValidationError, got %T: %v", err, err)
	}
	return ve.MsgCode
}

// classify files a seeded import under a category the way the user would
// from the transaction dialog; fakeTxns.GetByID reads the real row back.
func classify(t *testing.T, h *harness, txID vo.Id, categoryID string) {
	t.Helper()
	if _, err := h.db.Raw.ExecContext(context.Background(), h.db.Rebind("UPDATE transactions SET category_id = ? WHERE id = ?"), categoryID, txID.String()); err != nil {
		t.Fatal(err)
	}
}

func TestSuggestRules_DisabledWithoutCompleter(t *testing.T) {
	h := setup(t)
	_, err := h.svc.SuggestRules(context.Background(), vo.MustParseId(userA), model.SuggestImportRulesRequest{})
	if got := codeOf(t, err); got != errs.CodeImportAiDisabled {
		t.Fatalf("code = %q", got)
	}
}

func TestSuggestRules_NoClassifiedImportsSkipsTheModel(t *testing.T) {
	h := setup(t)
	fc := &fakeCompleter{reply: `{"rules":[]}`}
	h.svc.SetCompleter(fc)
	// seedTwoImports writes two Blue Bottle rows; neither carries a category yet
	seedTwoImports(t, h)
	res, err := h.svc.SuggestRules(context.Background(), vo.MustParseId(userA), model.SuggestImportRulesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 0 || fc.calls != 0 {
		t.Fatalf("items=%d calls=%d — an empty history must not spend a completion", len(res.Items), fc.calls)
	}
}

func TestSuggestRules_ValidatesEveryProposedRow(t *testing.T) {
	h := setup(t)
	uid := vo.MustParseId(userA)
	cat := h.f.Category(fixture.Category{UserID: userA, Name: "Coffee"})
	h.entities.add(h.entities.categories, uid, cat, "Coffee")
	edited, _ := seedTwoImports(t, h)
	classify(t, h, edited, cat)

	hallucinated := vo.NewId().String()
	fc := &fakeCompleter{reply: "```json\n" + `{"rules":[
		{"matchField":"external_payee","matchType":"contains","matchValue":"Blue Bottle","categoryId":"` + cat + `","reason":"3 coffee purchases"},
		{"matchField":"external_payee","matchType":"contains","matchValue":"Blue Bottle","categoryId":"` + cat + `","reason":"duplicate of the first"},
		{"matchField":"external_payee","matchType":"exact","matchValue":"Ghost","categoryId":"` + hallucinated + `"},
		{"matchField":"external_payee","matchType":"regex","matchValue":"B.*","categoryId":"` + cat + `"},
		{"matchField":"external_payee","matchType":"exact","matchValue":"Nothing to set"},
		{"matchField":"external_payee","matchType":"prefix","matchValue":"  Skip me  ","action":"skip","categoryId":"` + cat + `"}
	]}` + "\n```"}
	h.svc.SetCompleter(fc)

	res, err := h.svc.SuggestRules(context.Background(), uid, model.SuggestImportRulesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if fc.calls != 1 {
		t.Fatalf("calls = %d, want exactly one completion", fc.calls)
	}
	if len(res.Items) != 2 {
		t.Fatalf("items = %+v, want the deduped Blue Bottle row and the skip row re-typed as classify", res.Items)
	}
	first := res.Items[0]
	if first.Action != model.ImportRuleActionClassify || first.MatchType != model.ImportRuleMatchTypeContains || first.MatchValue != "Blue Bottle" || first.CategoryId == nil || *first.CategoryId != cat || first.Reason != "3 coffee purchases" {
		t.Fatalf("first = %+v", first)
	}
	if second := res.Items[1]; second.MatchValue != "Skip me" || second.Action != model.ImportRuleActionClassify {
		t.Fatalf("second = %+v (value trimmed, action forced to classify)", second)
	}
	if !strings.Contains(fc.lastUser, "Blue Bottle") || !strings.Contains(fc.lastUser, `"Coffee"`) {
		t.Fatalf("prompt must carry the payee strings and entity names: %s", fc.lastUser)
	}
	if strings.Contains(fc.lastUser, "4.75") {
		t.Fatalf("prompt must not carry amounts: %s", fc.lastUser)
	}
}

func TestSuggestRules_ModelFailuresAreCodedNot500(t *testing.T) {
	h := setup(t)
	uid := vo.MustParseId(userA)
	cat := h.f.Category(fixture.Category{UserID: userA, Name: "Coffee"})
	h.entities.add(h.entities.categories, uid, cat, "Coffee")
	edited, _ := seedTwoImports(t, h)
	classify(t, h, edited, cat)

	for name, fc := range map[string]*fakeCompleter{
		"transport error": {err: errors.New("dial tcp: connection refused")},
		"not json":        {reply: "Sure! Here are some rules: none."},
		"wrong shape":     {reply: `{"rules":"nope"}`},
	} {
		h.svc.SetCompleter(fc)
		_, err := h.svc.SuggestRules(context.Background(), uid, model.SuggestImportRulesRequest{})
		if got := codeOf(t, err); got != errs.CodeImportAiUnavailable {
			t.Fatalf("%s: code = %q", name, got)
		}
	}
}

func TestSuggestRules_RateLimited(t *testing.T) {
	h := setup(t)
	h.withLimiter()
	h.svc.SetCompleter(&fakeCompleter{reply: `{"rules":[]}`})
	uid := vo.MustParseId(userA)
	for i := 0; i < 2; i++ {
		if _, err := h.svc.SuggestRules(context.Background(), uid, model.SuggestImportRulesRequest{}); err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
	}
	_, err := h.svc.SuggestRules(context.Background(), uid, model.SuggestImportRulesRequest{})
	var tooMany *errs.TooManyRequestsError
	if !errors.As(err, &tooMany) {
		t.Fatalf("third call: want 429, got %v", err)
	}
	if h.lim.fail != 2 {
		t.Fatalf("every allowed call must count: fail=%d", h.lim.fail)
	}
}
```

Harness facts this test relies on: `h.db` is the `*dbtest.DB` on the ingest harness (`Raw` is the `*sql.DB`, `Rebind` handles `?`→`$N` on pgsql); `fakeTxns.GetByID` (Task 5) reads the real `transactions` row, so the SQL update above is what the service observes; `errs.TooManyRequestsError` is the concrete type behind `errs.NewTooManyRequests` (`internal/shared/errs/errs.go`).

- [ ] **Step 13: Run the suggest tests to verify they fail**

Run: `go test ./internal/imports/ -run TestSuggestRules`
Expected: FAIL — the stub returns `CodeImportAiDisabled` for every call, so every test but the first fails.

- [ ] **Step 14: Replace the stub with the real `SuggestRules`**

`internal/imports/suggest.go` (overwrite the Task 10 stub entirely):

```go
package imports

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

const (
	maxSuggestSamples = 200 // classified imports shown to the model
	maxSuggestions    = 50  // rows returned to the client
	maxSuggestReason  = 200 // runes kept from the model's free-text reason
)

const suggestSystemPrompt = `You help a user of a personal finance app write import rules.
The user message is a JSON object with the user's categories, payees, tags and labels (each with id and name)
and "samples": imported transactions (payee, description) paired with the classification the user chose.
Propose rules that would reproduce those choices for future imports.
Reply with ONLY a JSON object of the shape
{"rules":[{"matchField":"external_payee"|"description","matchType":"exact"|"contains"|"prefix",
"matchValue":string,"isCaseSensitive":bool,"categoryId":string|null,"payeeId":string|null,"tagId":string|null,
"labelIds":[string],"reason":string}]}.
Rules:
- Use ONLY ids that appear in the input; never invent ids.
- Prefer "contains" on the shortest stable token of the payee (drop store numbers, cities, card suffixes).
- One rule per distinct merchant; skip samples that contradict each other.
- "reason" is one short sentence for the user, e.g. "4 purchases were filed under Coffee".
- Return at most 30 rules. Return {"rules":[]} when nothing is consistent enough.`

type suggestNamed struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type suggestSample struct {
	Payee       string   `json:"payee"`
	Description string   `json:"description"`
	CategoryId  string   `json:"categoryId,omitempty"`
	PayeeId     string   `json:"payeeId,omitempty"`
	TagId       string   `json:"tagId,omitempty"`
	LabelIds    []string `json:"labelIds,omitempty"`
}

type suggestInput struct {
	Categories []suggestNamed  `json:"categories"`
	Payees     []suggestNamed  `json:"payees"`
	Tags       []suggestNamed  `json:"tags"`
	Labels     []suggestNamed  `json:"labels"`
	Samples    []suggestSample `json:"samples"`
}

type suggestRow struct {
	MatchField      string   `json:"matchField"`
	MatchType       string   `json:"matchType"`
	MatchValue      string   `json:"matchValue"`
	IsCaseSensitive bool     `json:"isCaseSensitive"`
	CategoryId      string   `json:"categoryId"`
	PayeeId         string   `json:"payeeId"`
	TagId           string   `json:"tagId"`
	LabelIds        []string `json:"labelIds"`
	Reason          string   `json:"reason"`
}

type suggestOutput struct {
	Rules []suggestRow `json:"rules"`
}

// SuggestRules asks the configured model for classify rules that reproduce
// the user's past choices on imported rows. Transient by design: nothing is
// stored, and every proposed row is re-validated here — ids the model made
// up, match types it invented, and skip rows are dropped, never surfaced.
func (s *Service) SuggestRules(ctx context.Context, userID vo.Id, req model.SuggestImportRulesRequest) (*model.SuggestImportRulesResult, error) {
	if s.completer == nil {
		return nil, &errs.ValidationError{Msg: "AI suggestions are not enabled on this server", MsgCode: errs.CodeImportAiDisabled}
	}
	if s.limiter != nil {
		if err := s.limiter.Allow(RateScopeSuggestRules, userID.String()); err != nil {
			return nil, err
		}
		s.limiter.Fail(RateScopeSuggestRules, userID.String()) // every call is a paid completion
	}
	in, err := s.suggestInput(ctx, userID, req)
	if err != nil {
		return nil, err
	}
	res := &model.SuggestImportRulesResult{Items: []model.ImportRuleSuggestion{}}
	if len(in.Samples) == 0 {
		return res, nil // nothing to learn from — and nothing leaves the instance
	}
	payload, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	reply, err := s.completer.Complete(ctx, suggestSystemPrompt, string(payload))
	if err != nil {
		// Type only: the error may quote the endpoint's response body.
		reqctx.AddLogAttr(ctx, "ai_error_type", fmt.Sprintf("%T", err))
		return nil, aiUnavailable()
	}
	rows, ok := parseSuggestions(reply)
	if !ok {
		reqctx.AddLogAttr(ctx, "ai_malformed_reply", true)
		return nil, aiUnavailable()
	}
	own, err := s.ownedEntities(ctx, userID)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, row := range rows {
		spec, ok := suggestionSpec(row, own)
		if !ok {
			continue
		}
		key := spec.MatchField + "\x00" + spec.MatchType + "\x00" + strings.ToLower(spec.MatchValue)
		if seen[key] {
			continue
		}
		seen[key] = true
		res.Items = append(res.Items, model.ImportRuleSuggestion{ImportRuleSpec: spec, Reason: clipRunes(strings.TrimSpace(row.Reason), maxSuggestReason)})
		if len(res.Items) == maxSuggestions {
			break
		}
	}
	reqctx.AddLogAttr(ctx, "suggested_count", len(res.Items))
	return res, nil
}

func aiUnavailable() error {
	return &errs.ValidationError{Msg: "The AI service is unavailable. Try again later.", MsgCode: errs.CodeImportAiUnavailable}
}

// suggestInput is the whole payload the model sees: entity names/ids plus
// (payee, description, chosen classification) per imported row that the
// user classified. Amounts, dates, and account names are deliberately absent.
func (s *Service) suggestInput(ctx context.Context, userID vo.Id, req model.SuggestImportRulesRequest) (*suggestInput, error) {
	in := &suggestInput{Categories: []suggestNamed{}, Payees: []suggestNamed{}, Tags: []suggestNamed{}, Labels: []suggestNamed{}, Samples: []suggestSample{}}
	lists := []struct {
		dst  *[]suggestNamed
		list func(context.Context, vo.Id) ([]model.ImportNamed, error)
	}{
		{&in.Categories, s.entities.CategoriesByOwner},
		{&in.Payees, s.entities.PayeesByOwner},
		{&in.Tags, s.entities.TagsByOwner},
		{&in.Labels, s.entities.LabelsByOwner},
	}
	for _, l := range lists {
		rows, err := l.list(ctx, userID)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			*l.dst = append(*l.dst, suggestNamed{ID: r.ID, Name: r.Name})
		}
	}
	links, err := s.scopedLinks(ctx, userID, req.Scope, req.RunId, req.ScopeSourceId)
	if err != nil {
		return nil, err
	}
	for _, link := range links {
		if len(in.Samples) == maxSuggestSamples {
			break
		}
		live, err := s.lister.GetByID(ctx, *link.TransactionID)
		if err != nil {
			if isNotFoundErr(err) {
				continue
			}
			return nil, err
		}
		if live.CategoryID == nil && live.PayeeID == nil && live.TagID == nil && len(live.LabelIDs) == 0 {
			continue // unclassified rows teach the model nothing
		}
		ev := linkEvent(link)
		sample := suggestSample{Payee: ev.Payee, Description: ev.Description}
		if live.CategoryID != nil {
			sample.CategoryId = live.CategoryID.String()
		}
		if live.PayeeID != nil {
			sample.PayeeId = live.PayeeID.String()
		}
		if live.TagID != nil {
			sample.TagId = live.TagID.String()
		}
		for _, id := range live.LabelIDs {
			sample.LabelIds = append(sample.LabelIds, id.String())
		}
		in.Samples = append(in.Samples, sample)
	}
	return in, nil
}

// parseSuggestions tolerates the usual model habits — prose around the
// JSON, a ```json fence — by parsing from the first '{' to the last '}'.
func parseSuggestions(reply string) ([]suggestRow, bool) {
	start, end := strings.Index(reply, "{"), strings.LastIndex(reply, "}")
	if start < 0 || end <= start {
		return nil, false
	}
	var out suggestOutput
	if err := json.Unmarshal([]byte(reply[start:end+1]), &out); err != nil {
		return nil, false
	}
	return out.Rules, true
}

// suggestionSpec re-types one model row as a classify rule spec and applies
// the same checks create-rule would: DTO validation plus target ownership.
// Anything the model got wrong drops the row; it never fails the request.
func suggestionSpec(row suggestRow, own *ownedIDs) (model.ImportRuleSpec, bool) {
	spec := model.ImportRuleSpec{
		Action:          model.ImportRuleActionClassify,
		MatchField:      row.MatchField,
		MatchType:       row.MatchType,
		MatchValue:      strings.TrimSpace(row.MatchValue),
		IsCaseSensitive: row.IsCaseSensitive,
		CategoryId:      optionalID(row.CategoryId),
		PayeeId:         optionalID(row.PayeeId),
		TagId:           optionalID(row.TagId),
		LabelIds:        []string{},
	}
	for _, id := range row.LabelIds {
		if id = strings.TrimSpace(id); id != "" {
			spec.LabelIds = append(spec.LabelIds, id)
		}
	}
	if err := spec.Validate(); err != nil {
		return spec, false
	}
	owned := func(set map[vo.Id]bool, raw *string) bool {
		if raw == nil {
			return true
		}
		id, err := vo.ParseId(*raw)
		return err == nil && set[id]
	}
	if !owned(own.categories, spec.CategoryId) || !owned(own.payees, spec.PayeeId) || !owned(own.tags, spec.TagId) {
		return spec, false
	}
	for _, raw := range spec.LabelIds {
		if !owned(own.labels, &raw) {
			return spec, false
		}
	}
	return spec, true
}

func optionalID(raw string) *string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	return &raw
}

func clipRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
```

Reconcile against what Tasks 2/6/9 actually produced before compiling:
- `model.ImportRuleSpec.Validate()` must reject a classify rule with no category/payee/tag/label target (Task 2 specifies this — the "Nothing to set" row relies on it) and an unknown `matchType` (`regex`).
- `linkEvent(link)` is the package-level Task 9 helper (`ruleapply.go`) returning a `model.IngestEvent` built from the link's `ExternalPayee`/`ExternalDescription`.
- `vo.ParseId` — if the value-object package only exposes `MustParseId` plus a non-panicking variant under a different name, use that one.
- `model.ImportRuleSuggestion` embeds `ImportRuleSpec` and carries `Reason string` (Task 2). If Task 2 declared `Reason` on a wrapper instead, adjust the composite literal.

- [ ] **Step 15: Run the suggest tests**

Run: `go test ./internal/imports/ -run TestSuggestRules -v`
Expected: PASS (5 tests). If `TestSuggestRules_ValidatesEveryProposedRow` returns 1 item instead of 2, the "Skip me" row was dropped: check that `suggestionSpec` ignores `row.Action` (the `suggestRow` struct has no `Action` field on purpose — an extra JSON key is ignored by `encoding/json`, so the row is re-typed as classify).

- [ ] **Step 16: Regression gates**

Run:

```bash
go build ./... && go vet ./... && gofmt -l . && go test ./internal/imports/... ./internal/config/ ./internal/server/ ./internal/web/... ./internal/test/apiparity/ ./internal/test/archtest/
```

Expected: all PASS; `gofmt -l` prints nothing. The `apiparity` `suggest-rules` golden is unchanged (the parity server has no completer, so the scenario still gets `import.ai_disabled`). `archtest` must stay green: `internal/infra/ai` imports nothing internal, and `internal/imports` still does not import it (the concrete client is injected in `internal/server`).

- [ ] **Step 17: Commit**

```bash
git add internal/infra/ai internal/config internal/server/server.go internal/imports/suggest.go internal/imports/suggest_test.go internal/web/router web/public/econumo-config.js web/src/lib/config.ts .env.example
git commit -m "feat(imports): AI-suggested rules behind ECONUMO_AI_DSN

One OpenAI-compatible completion per call; every proposed row is
re-validated (ownership, match type, classify only) before it reaches
the client. AI_ENABLED in the served config hides the action when unset.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 12: SPA API layer — DTOs, client, query hooks, metrics, msw fixtures

**Files:**
- Modify: `web/src/api/dto/imports.ts` (append rule DTOs; extend `TransactionImportLinkDto` at the bottom of the file)
- Modify: `web/src/api/imports.ts` (seven new functions)
- Modify: `web/src/app/queryKeys.ts` (`importRules` key)
- Modify: `web/src/features/imports/queries.ts` (seven hooks)
- Modify: `web/src/lib/metrics.ts` (three keys after `IMPORT_SYNC`)
- Modify: `web/src/test/fixtures.ts` (`importRules` default + `get-rule-list` handler)
- Test: `web/src/features/imports/queries.test.tsx` (append)

**Interfaces:**
- Consumes: the wire shapes from Task 2 (`ImportRuleResult` json tags, `PreviewImportRuleResult{matched, alreadyEdited}`, `ApplyImportRuleResult{updated, skipped}`, `SuggestImportRulesResult{items}`, the seven routes from Task 10) and the extended `TransactionImportLinkResult` (Task 9).
- Produces: `ImportRuleDto`, `ImportRuleSpecDto`, `ImportRuleScope`, `ImportRuleSuggestionDto`, `PreviewImportRuleDto`, `ApplyImportRuleDto`; `importsApi.{getImportRuleList, createImportRule, updateImportRule, deleteImportRule, previewImportRule, applyImportRule, suggestImportRules}`; hooks `useImportRules`, `useCreateImportRule`, `useUpdateImportRule`, `useDeleteImportRule`, `usePreviewImportRule`, `useApplyImportRule`, `useSuggestImportRules`; `METRICS.IMPORT_RULE_CREATE / IMPORT_RULE_APPLY / IMPORT_RULES_SUGGEST`; `queryKeys.importRules`; fixture key `importRules`.

- [ ] **Step 1: DTOs**

Append to `web/src/api/dto/imports.ts`:

```ts
export type ImportRuleAction = 'classify' | 'skip'
export type ImportRuleMatchField = 'description' | 'external_payee'
export type ImportRuleMatchType = 'exact' | 'contains' | 'prefix'
export type ImportRuleScope = 'run' | 'source' | 'all'

export interface ImportRuleSpecDto {
  /** '' = every source */
  sourceId: Id | ''
  action: ImportRuleAction
  matchField: ImportRuleMatchField
  matchType: ImportRuleMatchType
  matchValue: string
  isCaseSensitive: boolean
  categoryId: Id | ''
  payeeId: Id | ''
  tagId: Id | ''
  labelIds: Id[]
  priority: number
}

export interface ImportRuleDto extends ImportRuleSpecDto {
  id: Id
  createdAt: string
  updatedAt: string
}

export interface ImportRuleScopeDto {
  scope: ImportRuleScope
  runId: Id | ''
  scopeSourceId: Id | ''
}

export interface PreviewImportRuleDto {
  matched: number
  alreadyEdited: number
}

export interface ApplyImportRuleDto {
  updated: number
  skipped: number
}

export interface ImportRuleSuggestionDto extends ImportRuleSpecDto {
  reason: string
}
```

Then extend the EXISTING `TransactionImportLinkDto` (same file) — insert after `sourceName: string`:

```ts
  runId: Id | ''
```

and after `externalPayee: string`:

```ts
  externalDescription: string
```

and after `importedAt: string`:

```ts
  /** classification snapshot written at import time; '' / [] when nothing was applied */
  appliedCategoryId: Id | ''
  appliedPayeeId: Id | ''
  appliedTagId: Id | ''
  appliedLabelIds: Id[]
  appliedRuleId: Id | ''
```

- [ ] **Step 2: Client functions**

In `web/src/api/imports.ts`, add to the type import list: `ApplyImportRuleDto, ImportRuleDto, ImportRuleScopeDto, ImportRuleSpecDto, ImportRuleSuggestionDto, PreviewImportRuleDto`. Append:

```ts
export async function getImportRuleList(): Promise<ImportRuleDto[]> {
  const response = await api.get<Envelope<{ items: ImportRuleDto[] }>>(apiUrl('/api/v1/import/get-rule-list'))
  return response.data.data.items
}

export async function createImportRule(spec: ImportRuleSpecDto, id?: Id): Promise<ImportRuleDto> {
  const body: Record<string, unknown> = { ...spec }
  if (id) {
    body.id = id
  }
  const response = await api.post<Envelope<ImportRuleDto>>(apiUrl('/api/v1/import/create-rule'), body)
  return response.data.data
}

export async function updateImportRule(id: Id, spec: ImportRuleSpecDto): Promise<ImportRuleDto> {
  const response = await api.post<Envelope<ImportRuleDto>>(apiUrl('/api/v1/import/update-rule'), { id, ...spec })
  return response.data.data
}

export async function deleteImportRule(id: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/import/delete-rule'), { id })
}

export async function previewImportRule(spec: ImportRuleSpecDto, scope: ImportRuleScopeDto): Promise<PreviewImportRuleDto> {
  const response = await api.post<Envelope<PreviewImportRuleDto>>(apiUrl('/api/v1/import/preview-rule'), { ...spec, ...scope })
  return response.data.data
}

export async function applyImportRule(ruleId: Id, scope: ImportRuleScopeDto, includeEdited: boolean): Promise<ApplyImportRuleDto> {
  const response = await api.post<Envelope<ApplyImportRuleDto>>(apiUrl('/api/v1/import/apply-rule'), { ruleId, ...scope, includeEdited })
  return response.data.data
}

export async function suggestImportRules(scope: ImportRuleScopeDto): Promise<ImportRuleSuggestionDto[]> {
  const response = await api.post<Envelope<{ items: ImportRuleSuggestionDto[] }>>(apiUrl('/api/v1/import/suggest-rules'), scope)
  return response.data.data.items
}
```

- [ ] **Step 3: Query key, metrics, fixtures**

`web/src/app/queryKeys.ts` — after `importSources: ['importSources'] as const,`:

```ts
  importRules: ['importRules'] as const,
```

`web/src/lib/metrics.ts` — after `IMPORT_SYNC: 'appImportSync',`:

```ts
  IMPORT_RULE_CREATE: 'appImportRuleCreate',
  IMPORT_RULE_APPLY: 'appImportRuleApply',
  IMPORT_RULES_SUGGEST: 'appImportRulesSuggest',
```

`web/src/test/fixtures.ts` — in the `data` defaults after `importRuns: [] as unknown[],`:

```ts
    importRules: [] as unknown[],
```

and in the handler list after the `get-run-list` handler:

```ts
    http.get('*/api/v1/import/get-rule-list', () => envelope({ items: data.importRules })),
```

- [ ] **Step 4: Write the failing hook tests**

Append to `web/src/features/imports/queries.test.tsx` (add `useApplyImportRule, useCreateImportRule, useImportRules, useSuggestImportRules` to the `./queries` import; add `import type { ImportRuleDto } from '@/api/dto/imports'` — or extend the existing dto type import):

```tsx
const wireRule: ImportRuleDto = {
  id: 'rule1', sourceId: '', action: 'classify', matchField: 'external_payee', matchType: 'contains', matchValue: 'BLUE BOTTLE',
  isCaseSensitive: false, categoryId: 'c1', payeeId: '', tagId: '', labelIds: [], priority: 0,
  createdAt: '2026-09-08 10:00:00', updatedAt: '2026-09-08 10:00:00',
}

it('useImportRules fetches the rule list', async () => {
  server.use(http.get('*/api/v1/import/get-rule-list', () =>
    HttpResponse.json({ success: true, message: '', data: { items: [wireRule] } })))
  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => useImportRules(), { wrapper })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(result.current.data![0].matchValue).toBe('BLUE BOTTLE')
})

it('useCreateImportRule posts the spec, appends to the cache, and reports the action', async () => {
  let body: Record<string, unknown> | null = null
  server.use(http.post('*/api/v1/import/create-rule', async ({ request }) => {
    body = await request.json() as Record<string, unknown>
    return HttpResponse.json({ success: true, message: '', data: { ...wireRule, action: body.action } })
  }))
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData<ImportRuleDto[]>(queryKeys.importRules, [])
  const { result } = renderHook(() => useCreateImportRule(), { wrapper })
  const { id: _id, createdAt: _c, updatedAt: _u, ...spec } = wireRule
  await act(async () => { await result.current.mutateAsync({ spec: { ...spec, action: 'skip' } }) })
  expect(body).toMatchObject({ action: 'skip', matchValue: 'BLUE BOTTLE' })
  expect(body).not.toHaveProperty('id')
  expect(queryClient.getQueryData<ImportRuleDto[]>(queryKeys.importRules)).toHaveLength(1)
  expect(trackEventMock).toHaveBeenCalledWith(METRICS.IMPORT_RULE_CREATE, { action: 'skip' })
})

it('useApplyImportRule posts the scope and invalidates the ledger caches', async () => {
  let body: unknown
  server.use(http.post('*/api/v1/import/apply-rule', async ({ request }) => {
    body = await request.json()
    return HttpResponse.json({ success: true, message: '', data: { updated: 3, skipped: 2 } })
  }))
  const { queryClient, wrapper } = makeWrapper()
  const invalidate = vi.spyOn(queryClient, 'invalidateQueries')
  const { result } = renderHook(() => useApplyImportRule(), { wrapper })
  let out: unknown
  await act(async () => {
    out = await result.current.mutateAsync({ ruleId: 'rule1', scope: { scope: 'run', runId: 'r1', scopeSourceId: '' }, includeEdited: false })
  })
  expect(body).toEqual({ ruleId: 'rule1', scope: 'run', runId: 'r1', scopeSourceId: '', includeEdited: false })
  expect(out).toEqual({ updated: 3, skipped: 2 })
  expect(invalidate).toHaveBeenCalledWith({ queryKey: queryKeys.transactions })
  expect(invalidate).toHaveBeenCalledWith({ queryKey: queryKeys.budget })
  expect(trackEventMock).toHaveBeenCalledWith(METRICS.IMPORT_RULE_APPLY, { updated: 3, skipped: 2 })
})

it('useSuggestImportRules returns the model proposals and fires the metric with the count', async () => {
  server.use(http.post('*/api/v1/import/suggest-rules', () =>
    HttpResponse.json({ success: true, message: '', data: { items: [{ ...wireRule, reason: 'Every Blue Bottle tap was categorised Coffee' }] } })))
  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => useSuggestImportRules(), { wrapper })
  let items: unknown[] = []
  await act(async () => { items = await result.current.mutateAsync({ scope: 'all', runId: '', scopeSourceId: '' }) })
  expect(items).toHaveLength(1)
  expect(trackEventMock).toHaveBeenCalledWith(METRICS.IMPORT_RULES_SUGGEST, { count: 1 })
})
```

- [ ] **Step 5: Run to verify they fail**

Run: `pnpm --dir web exec vitest run src/features/imports/queries.test.tsx`
Expected: FAIL — `useImportRules` (and the others) are not exported.

- [ ] **Step 6: Hooks**

Append to `web/src/features/imports/queries.ts` (extend the dto type import with `ImportRuleDto, ImportRuleScopeDto, ImportRuleSpecDto`):

```ts
export function useImportRules() {
  return useQuery({ queryKey: queryKeys.importRules, queryFn: importsApi.getImportRuleList, staleTime: TEN_MINUTES })
}

export function useCreateImportRule() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ spec, id }: { spec: ImportRuleSpecDto; id?: Id }) => importsApi.createImportRule(spec, id),
    onSuccess: (rule) => {
      queryClient.setQueryData<ImportRuleDto[]>(queryKeys.importRules, (prev = []) => [...prev.filter((r) => r.id !== rule.id), rule])
      trackEvent(METRICS.IMPORT_RULE_CREATE, { action: rule.action })
    },
  })
}

export function useUpdateImportRule() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, spec }: { id: Id; spec: ImportRuleSpecDto }) => importsApi.updateImportRule(id, spec),
    onSuccess: (rule) => {
      queryClient.setQueryData<ImportRuleDto[]>(queryKeys.importRules, (prev = []) => prev.map((r) => (r.id === rule.id ? rule : r)))
    },
  })
}

export function useDeleteImportRule() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: Id) => importsApi.deleteImportRule(id),
    onSuccess: (_void, id) => {
      queryClient.setQueryData<ImportRuleDto[]>(queryKeys.importRules, (prev = []) => prev.filter((r) => r.id !== id))
    },
  })
}

// A preview is a read that travels as a POST (the spec is the body); it is
// never cached — the count must reflect the value the user is typing.
export function usePreviewImportRule() {
  return useMutation({
    mutationFn: ({ spec, scope }: { spec: ImportRuleSpecDto; scope: ImportRuleScopeDto }) => importsApi.previewImportRule(spec, scope),
  })
}

export function useApplyImportRule() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ ruleId, scope, includeEdited }: { ruleId: Id; scope: ImportRuleScopeDto; includeEdited: boolean }) =>
      importsApi.applyImportRule(ruleId, scope, includeEdited),
    onSuccess: (result) => {
      trackEvent(METRICS.IMPORT_RULE_APPLY, { updated: result.updated, skipped: result.skipped })
      void queryClient.invalidateQueries({ queryKey: queryKeys.transactions })
      void queryClient.invalidateQueries({ queryKey: queryKeys.accounts })
      void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
    },
  })
}

export function useSuggestImportRules() {
  return useMutation({
    mutationFn: (scope: ImportRuleScopeDto) => importsApi.suggestImportRules(scope),
    onSuccess: (items) => trackEvent(METRICS.IMPORT_RULES_SUGGEST, { count: items.length }),
  })
}
```

- [ ] **Step 7: Run the tests and the type-check**

Run: `pnpm --dir web exec vitest run src/features/imports/queries.test.tsx src/lib/metrics-coverage.test.ts && pnpm --dir web exec tsc -b`
Expected: the hook tests PASS; `tsc -b` is clean (any existing `TransactionImportLinkDto` literal in a test — grep `externalPostedAt` under `web/src` — must gain the new fields: `runId: ''`, `externalDescription: ''`, `appliedCategoryId: ''`, `appliedPayeeId: ''`, `appliedTagId: ''`, `appliedLabelIds: []`, `appliedRuleId: ''`). `metrics-coverage.test.ts` still passes because every new key is referenced from `queries.ts`.

- [ ] **Step 8: Commit**

```bash
git add web/src/api/dto/imports.ts web/src/api/imports.ts web/src/app/queryKeys.ts web/src/features/imports/queries.ts web/src/features/imports/queries.test.tsx web/src/lib/metrics.ts web/src/test/fixtures.ts
git commit -m "feat(web): import rule API client, hooks, and metrics

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 13: Client-side matcher mirror + the post-edit "make this a rule?" prompt

**Files:**
- Create: `web/src/lib/importMatch.ts`, `web/src/lib/importMatch.test.ts`
- Create: `web/src/features/imports/RulePromptDialog.tsx`, `web/src/features/imports/RulePromptDialog.test.tsx`
- Modify: `web/src/app/uiStore.ts` (`rulePrompt` slice)
- Modify: `web/src/features/transactions/TransactionDialog.tsx` (update branch of `submit`, ~line 211)
- Modify: `web/src/app/layouts/ApplicationLayout.tsx` (mount next to `<TransactionDialog />`, line 286)
- Modify: `web/src/features/transactions/TransactionDialog.test.tsx` (one case)
- Modify: `locales/en.json` (the `imports.rules.prompt.*` keys used here — the other 10 catalogues are filled in Task 15; `internal/test/i18ntest` fails until then, which is expected mid-branch)

**Interfaces:**
- Consumes: `ImportRuleSpecDto`, `ImportRuleScopeDto`, `TransactionImportLinkDto` (Task 12); hooks `usePreviewImportRule`, `useCreateImportRule`, `useUpdateImportRule`, `useApplyImportRule`, `useImportRules`, `useTransactionImportLinks` (Task 12 / existing); `CreateTransactionDto`; `ResponsiveDialog`, `Button`, `Input`, `Checkbox`, `NativeSelect`.
- Produces: `normalizeMatchText(s)`, `matchesRule(spec, text)`, `suggestMatchValue(raw)`, `ruleDiff(payload, link)`, `latestImportLink(links)`, `RuleDiff`, `RulePromptParams`; store `rulePrompt` + `setRulePrompt`; `<RulePromptDialog />`.

**Spec rules this task encodes (Part 7):** the prompt appears ONLY when the saved value differs from the MOST RECENT link's `applied*` snapshot — re-editing an already corrected transaction does not re-prompt, and amount/date/description edits never prompt. The match value is editable with a live count. When the link carries `appliedRuleId`, the prompt offers to update that rule instead of creating another. Backfill goes through preview → apply, skips already-edited rows by default (with the count shown and an opt-in override), defaults to the current run, and offers "all imports from this source" as a clearly labelled second step.

- [ ] **Step 1: Write the failing matcher tests**

`web/src/lib/importMatch.test.ts`:

```ts
import type { CreateTransactionDto } from '@/api/dto/transaction'
import type { ImportRuleSpecDto, TransactionImportLinkDto } from '@/api/dto/imports'
import { latestImportLink, matchesRule, normalizeMatchText, ruleDiff, suggestMatchValue } from './importMatch'

const spec = (over: Partial<ImportRuleSpecDto> = {}): ImportRuleSpecDto => ({
  sourceId: '', action: 'classify', matchField: 'external_payee', matchType: 'contains', matchValue: 'blue bottle',
  isCaseSensitive: false, categoryId: 'c1', payeeId: '', tagId: '', labelIds: [], priority: 0, ...over,
})

const link = (over: Partial<TransactionImportLinkDto> = {}): TransactionImportLinkDto => ({
  id: 'l1', sourceId: 's1', runId: 'r1', provider: 'apple-wallet', sourceName: 'iPhone', externalAccountId: 'wallet',
  externalTransactionId: 'tap-1', externalPayee: 'BLUE BOTTLE COFFEE', externalDescription: '', externalAmount: '4.75',
  externalCurrency: 'USD', externalPostedAt: '2026-09-01 10:00:00', status: 'created', importedAt: '2026-09-01 10:00:05',
  appliedCategoryId: '', appliedPayeeId: '', appliedTagId: '', appliedLabelIds: [], appliedRuleId: '', ...over,
})

const payload = (over: Partial<CreateTransactionDto> = {}): CreateTransactionDto => ({
  id: 't1', type: 'expense', accountId: 'a1', accountRecipientId: null, amount: '4.75', amountRecipient: '4.75',
  categoryId: null, description: 'BLUE BOTTLE COFFEE', payeeId: null, tagId: null, labelIds: [], date: '2026-09-01 10:00:00', ...over,
})

describe('normalizeMatchText', () => {
  it('trims and collapses whitespace runs exactly like the Go NormalizeText', () => {
    expect(normalizeMatchText('  BLUE   BOTTLE\t COFFEE \n')).toBe('BLUE BOTTLE COFFEE')
    expect(normalizeMatchText('')).toBe('')
  })
})

describe('matchesRule', () => {
  it('folds case unless the rule is case-sensitive', () => {
    expect(matchesRule(spec(), { payee: 'Blue Bottle Coffee', description: '' })).toBe(true)
    expect(matchesRule(spec({ isCaseSensitive: true }), { payee: 'Blue Bottle Coffee', description: '' })).toBe(false)
    expect(matchesRule(spec({ isCaseSensitive: true, matchValue: 'Blue Bottle' }), { payee: 'Blue Bottle Coffee', description: '' })).toBe(true)
  })
  it('exact and prefix compare normalized text; an empty value never matches', () => {
    expect(matchesRule(spec({ matchType: 'exact', matchValue: 'blue bottle coffee' }), { payee: ' BLUE  BOTTLE COFFEE', description: '' })).toBe(true)
    expect(matchesRule(spec({ matchType: 'exact' }), { payee: 'BLUE BOTTLE COFFEE', description: '' })).toBe(false)
    expect(matchesRule(spec({ matchType: 'prefix' }), { payee: 'BLUE BOTTLE COFFEE', description: '' })).toBe(true)
    expect(matchesRule(spec({ matchType: 'prefix', matchValue: 'coffee' }), { payee: 'BLUE BOTTLE COFFEE', description: '' })).toBe(false)
    expect(matchesRule(spec({ matchValue: '   ' }), { payee: 'BLUE BOTTLE COFFEE', description: '' })).toBe(false)
  })
  it('description rules read the description and fall back to the payee when the provider sent none', () => {
    expect(matchesRule(spec({ matchField: 'description', matchValue: 'memo' }), { payee: 'BLUE BOTTLE', description: 'card memo' })).toBe(true)
    expect(matchesRule(spec({ matchField: 'description' }), { payee: 'BLUE BOTTLE', description: 'card memo' })).toBe(false)
    expect(matchesRule(spec({ matchField: 'description' }), { payee: 'BLUE BOTTLE', description: '' })).toBe(true)
  })
})

describe('suggestMatchValue', () => {
  it.each([
    ['BLUE BOTTLE COFFEE #142 SAN FRANCISCO CA', 'BLUE BOTTLE COFFEE'],
    ['SQ *BLUE BOTTLE', 'BLUE BOTTLE'],
    ['AMZN Mktp US*2K4L19XZ0', 'AMZN Mktp'],
    ['UBER TRIP 09/08', 'UBER TRIP'],
    ['  Apple   Pay  ', 'Apple Pay'],
    ['IKEA', 'IKEA'],
    ['1234', '1234'],
    ['', ''],
  ])('%s -> %s', (raw, want) => {
    expect(suggestMatchValue(raw)).toBe(want)
  })
})

describe('ruleDiff', () => {
  it('reports classification fields whose saved value differs from the applied snapshot', () => {
    expect(ruleDiff(payload({ categoryId: 'c1' }), link())).toEqual({ categoryId: 'c1' })
    expect(ruleDiff(payload({ categoryId: 'c1', tagId: 't1', labelIds: ['l2', 'l1'] }), link({ appliedCategoryId: 'c1', appliedLabelIds: ['l1', 'l2'] })))
      .toEqual({ tagId: 't1' })
  })
  it('is null when nothing classification-related changed — amount, date, notes edits never prompt', () => {
    expect(ruleDiff(payload({ amount: '5.00', description: 'edited', date: '2026-09-02 10:00:00', categoryId: 'c1' }), link({ appliedCategoryId: 'c1' }))).toBeNull()
    expect(ruleDiff(payload(), link())).toBeNull()
  })
  it('clearing a field is not a rule-worthy change', () => {
    expect(ruleDiff(payload({ categoryId: null }), link({ appliedCategoryId: 'c1' }))).toBeNull()
  })
})

describe('latestImportLink', () => {
  it('picks the most recently imported link', () => {
    const older = link({ id: 'old', importedAt: '2026-08-01 00:00:00' })
    const newer = link({ id: 'new', importedAt: '2026-09-01 00:00:00' })
    expect(latestImportLink([older, newer])?.id).toBe('new')
    expect(latestImportLink([newer, older])?.id).toBe('new')
    expect(latestImportLink([])).toBeNull()
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `pnpm --dir web exec vitest run src/lib/importMatch.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement `importMatch.ts`**

```ts
import type { CreateTransactionDto } from '@/api/dto/transaction'
import type { ImportRuleSpecDto, TransactionImportLinkDto } from '@/api/dto/imports'
import type { Id } from '@/api/types'

// Mirror of internal/imports/rules.go (NormalizeText + matchRule). The count
// the prompt shows must equal what the server later matches, so the two
// implementations must stay identical.
export function normalizeMatchText(s: string): string {
  return s.split(/\s+/).filter(Boolean).join(' ')
}

export interface MatchText {
  payee: string
  description: string
}

export function matchesRule(spec: Pick<ImportRuleSpecDto, 'matchField' | 'matchType' | 'matchValue' | 'isCaseSensitive'>, text: MatchText): boolean {
  const raw = spec.matchField === 'description' && text.description !== '' ? text.description : text.payee
  let subject = normalizeMatchText(raw)
  let value = normalizeMatchText(spec.matchValue)
  if (!spec.isCaseSensitive) {
    subject = subject.toLowerCase()
    value = value.toLowerCase()
  }
  if (value === '') {
    return false
  }
  switch (spec.matchType) {
    case 'exact':
      return subject === value
    case 'prefix':
      return subject.startsWith(value)
    case 'contains':
      return subject.includes(value)
  }
  return false
}

const REGION_CODE = /^[A-Z]{2}$/
const hasLetter = (token: string) => /\p{L}/u.test(token)

// A first guess at the merchant part of a bank/wallet payee string: drop the
// processor prefix ("SQ *"), the reference/store/date tokens, and a trailing
// region code. It is only a prefill — the user edits it with a live count.
export function suggestMatchValue(raw: string): string {
  const normalized = normalizeMatchText(raw)
  if (normalized === '') {
    return ''
  }
  const parts = normalized.split('*').map((p) => p.trim()).filter(Boolean)
  let part = parts[0] ?? normalized
  for (const candidate of parts) {
    if (candidate.replace(/[^\p{L}]/gu, '').length > part.replace(/[^\p{L}]/gu, '').length) {
      part = candidate
    }
  }
  const tokens: string[] = []
  for (const token of part.split(' ')) {
    if (!hasLetter(token) || token.startsWith('#')) {
      break
    }
    tokens.push(token)
  }
  while (tokens.length > 1 && REGION_CODE.test(tokens[tokens.length - 1])) {
    tokens.pop()
  }
  const suggestion = tokens.join(' ')
  return suggestion.length >= 3 ? suggestion : normalized
}

export interface RuleDiff {
  categoryId?: Id
  payeeId?: Id
  tagId?: Id
  labelIds?: Id[]
}

const sameSet = (a: Id[], b: Id[]) => a.length === b.length && [...a].sort().every((v, i) => v === [...b].sort()[i])

// Only a NEW non-empty value that differs from what the import applied is a
// rule-worthy change: clearing a field produces no classification to encode,
// and re-saving an already corrected transaction must not prompt again.
export function ruleDiff(payload: CreateTransactionDto, link: TransactionImportLinkDto): RuleDiff | null {
  const diff: RuleDiff = {}
  if (payload.categoryId && payload.categoryId !== link.appliedCategoryId) {
    diff.categoryId = payload.categoryId
  }
  if (payload.payeeId && payload.payeeId !== link.appliedPayeeId) {
    diff.payeeId = payload.payeeId
  }
  if (payload.tagId && payload.tagId !== link.appliedTagId) {
    diff.tagId = payload.tagId
  }
  if (payload.labelIds.length > 0 && !sameSet(payload.labelIds, link.appliedLabelIds)) {
    diff.labelIds = payload.labelIds
  }
  return Object.keys(diff).length > 0 ? diff : null
}

export function latestImportLink(links: TransactionImportLinkDto[]): TransactionImportLinkDto | null {
  let latest: TransactionImportLinkDto | null = null
  for (const l of links) {
    if (!latest || l.importedAt > latest.importedAt) {
      latest = l
    }
  }
  return latest
}
```

- [ ] **Step 4: Run the matcher tests**

Run: `pnpm --dir web exec vitest run src/lib/importMatch.test.ts`
Expected: PASS.

- [ ] **Step 5: Store slice**

`web/src/app/uiStore.ts`: add `import type { TransactionImportLinkDto } from '@/api/dto/imports'` and `import type { RuleDiff } from '@/lib/importMatch'`. Export, next to `OpenTransactionParams`:

```ts
export interface RulePromptParams {
  link: TransactionImportLinkDto
  diff: RuleDiff
}
```

In `UiState`, after `setSwitchAccountPrompt`:

```ts
  rulePrompt: RulePromptParams | null
  setRulePrompt: (params: RulePromptParams | null) => void
```

In the `create` body, after `setSwitchAccountPrompt: (id) => set({ switchAccountPrompt: id }),`:

```ts
  rulePrompt: null,
  setRulePrompt: (params) => set({ rulePrompt: params }),
```

- [ ] **Step 6: Write the failing TransactionDialog test**

Append to `web/src/features/transactions/TransactionDialog.test.tsx` (`chip` and `captureUpdate` already exist there):

```tsx
it('editing an imported transaction prompts for a rule only when the classification diverges from what the import applied', async () => {
  const seen = captureUpdate()
  const importLink = {
    id: 'l1', sourceId: 's1', runId: 'r1', provider: 'apple-wallet', sourceName: 'iPhone', externalAccountId: 'wallet',
    externalTransactionId: 'tap-1', externalPayee: 'BLUE BOTTLE COFFEE #142', externalDescription: '', externalAmount: '9.99',
    externalCurrency: 'USD', externalPostedAt: '2026-07-03 10:00:00', status: 'created', importedAt: '2026-07-03 10:00:05',
    appliedCategoryId: 'cat-food', appliedPayeeId: '', appliedTagId: '', appliedLabelIds: [], appliedRuleId: '',
  }
  server.use(http.get('*/api/v1/transaction/get-transaction-import-list', () =>
    HttpResponse.json({ success: true, message: '', data: { items: [importLink] } })))
  const user = userEvent.setup()
  renderDialog()
  useUiStore.setState({ rulePrompt: null })
  useUiStore.getState().openTransactionModal({ transaction: wireTxEcho({ id: 't-imported', isImported: 1 }) as unknown as TransactionDto })
  await screen.findByRole('heading', { name: 'Edit transaction' })

  // notes-only edit: category still equals the applied snapshot → no prompt
  await user.type(screen.getByLabelText('Notes'), 'x')
  await user.click(screen.getByRole('button', { name: 'Update' }))
  await waitFor(() => expect(seen.body).toBeDefined())
  expect(useUiStore.getState().rulePrompt).toBeNull()

  // adding a label diverges from the snapshot → prompt with exactly that diff
  useUiStore.getState().openTransactionModal({ transaction: wireTxEcho({ id: 't-imported', isImported: 1 }) as unknown as TransactionDto })
  await screen.findByRole('heading', { name: 'Edit transaction' })
  await waitFor(() => expect(chip('health', 'label')).toBeInTheDocument())
  await user.click(chip('health', 'label'))
  await user.click(screen.getByRole('button', { name: 'Update' }))
  await waitFor(() => expect(useUiStore.getState().rulePrompt).not.toBeNull())
  expect(useUiStore.getState().rulePrompt).toEqual({ link: importLink, diff: { labelIds: ['label1'] } })
})
```

Check the exact provenance route in `web/src/api/imports.ts` (`getTransactionImportList`) and use that path in the handler above — it is the one `useTransactionImportLinks` calls.

- [ ] **Step 7: Run to verify it fails**

Run: `pnpm --dir web exec vitest run src/features/transactions/TransactionDialog.test.tsx -t "prompts for a rule"`
Expected: FAIL — `rulePrompt` stays null after the label edit.

- [ ] **Step 8: Wire the prompt into the update path**

`web/src/features/transactions/TransactionDialog.tsx`:

- extend the imports: `import { useImportQueuedEvent, useTransactionImportLinks } from '@/features/imports/queries'` and `import { latestImportLink, ruleDiff } from '@/lib/importMatch'`.
- in `TransactionForm`, after `const setSwitchAccountPrompt = ...`:

```tsx
  const setRulePrompt = useUiStore((s) => s.setRulePrompt)
  const isImported = params.transaction?.isImported === 1
  const { data: importLinks = [] } = useTransactionImportLinks(params.transaction?.id ?? '', isImported)
```

- replace the update branch of `submit`:

```tsx
      } else {
        await updateTransaction.mutateAsync(payload)
        const link = latestImportLink(importLinks)
        const diff = link ? ruleDiff(payload, link) : null
        if (link && diff) {
          setRulePrompt({ link, diff })
        }
      }
```

- [ ] **Step 9: Run the dialog test**

Run: `pnpm --dir web exec vitest run src/features/transactions/TransactionDialog.test.tsx`
Expected: PASS (the whole file, not just the new case).

- [ ] **Step 10: Write the failing RulePromptDialog test**

`web/src/features/imports/RulePromptDialog.test.tsx`:

```tsx
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureCategories } from '@/test/fixtures'
import { useUiStore } from '@/app/uiStore'
import type { TransactionImportLinkDto } from '@/api/dto/imports'
import { RulePromptDialog } from './RulePromptDialog'

vi.mock('@/hooks/useIsCompact', () => ({ useIsCompact: () => false }))
vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))

const link = (over: Partial<TransactionImportLinkDto> = {}): TransactionImportLinkDto => ({
  id: 'l1', sourceId: 's1', runId: 'r1', provider: 'apple-wallet', sourceName: 'iPhone', externalAccountId: 'wallet',
  externalTransactionId: 'tap-1', externalPayee: 'BLUE BOTTLE COFFEE #142 SAN FRANCISCO CA', externalDescription: '', externalAmount: '4.75',
  externalCurrency: 'USD', externalPostedAt: '2026-09-01 10:00:00', status: 'created', importedAt: '2026-09-01 10:00:05',
  appliedCategoryId: '', appliedPayeeId: '', appliedTagId: '', appliedLabelIds: [], appliedRuleId: '', ...over,
})

const rule = {
  id: 'rule1', sourceId: '', action: 'classify', matchField: 'external_payee', matchType: 'contains', matchValue: 'BLUE BOTTLE',
  isCaseSensitive: false, categoryId: 'cat-salary', payeeId: '', tagId: '', labelIds: [], priority: 0,
  createdAt: '2026-09-01 00:00:00', updatedAt: '2026-09-01 00:00:00',
}

function renderPrompt(data: Record<string, unknown> = {}) {
  server.use(...coreHandlers({ categories: fixtureCategories, ...data }))
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(<QueryClientProvider client={queryClient}><RulePromptDialog /></QueryClientProvider>)
}

const posted: Record<string, unknown[]> = {}
beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({ matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn() }))
  useUiStore.setState({ rulePrompt: null })
  for (const k of Object.keys(posted)) delete posted[k]
  server.use(
    http.post('*/api/v1/import/preview-rule', async ({ request }) => {
      const body = await request.json() as Record<string, unknown>
      ;(posted.preview ??= []).push(body)
      const matched = body.scope === 'source' ? 9 : 5
      return HttpResponse.json({ success: true, message: '', data: { matched, alreadyEdited: 2 } })
    }),
    http.post('*/api/v1/import/create-rule', async ({ request }) => {
      const body = await request.json() as Record<string, unknown>
      ;(posted.create ??= []).push(body)
      return HttpResponse.json({ success: true, message: '', data: { ...rule, ...body, id: 'rule-new' } })
    }),
    http.post('*/api/v1/import/update-rule', async ({ request }) => {
      const body = await request.json() as Record<string, unknown>
      ;(posted.update ??= []).push(body)
      return HttpResponse.json({ success: true, message: '', data: { ...rule, ...body } })
    }),
    http.post('*/api/v1/import/apply-rule', async ({ request }) => {
      const body = await request.json() as Record<string, unknown>
      ;(posted.apply ??= []).push(body)
      return HttpResponse.json({ success: true, message: '', data: { updated: 3, skipped: 2 } })
    }),
  )
})

it('renders nothing without a pending prompt', () => {
  renderPrompt()
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
})

it('prefills a suggested match value, previews live, creates the rule, then applies to this import and offers the whole source', async () => {
  renderPrompt()
  useUiStore.getState().setRulePrompt({ link: link(), diff: { categoryId: 'cat-food' } })
  const user = userEvent.setup()
  expect(await screen.findByText('You set the category to Food. Create a rule for similar imports?')).toBeInTheDocument()
  const value = screen.getByLabelText('Payee contains') as HTMLInputElement
  expect(value.value).toBe('BLUE BOTTLE COFFEE')
  expect(await screen.findByText('Matches 5 transactions in this import')).toBeInTheDocument()
  expect(posted.preview![0]).toMatchObject({ matchField: 'external_payee', matchType: 'contains', matchValue: 'BLUE BOTTLE COFFEE', categoryId: 'cat-food', scope: 'run', runId: 'r1', scopeSourceId: 's1' })

  await user.clear(value)
  await user.type(value, 'BLUE BOTTLE')
  await waitFor(() => expect(posted.preview!.at(-1)).toMatchObject({ matchValue: 'BLUE BOTTLE' }))

  await user.click(screen.getByRole('button', { name: 'Create rule' }))
  expect(posted.create![0]).toMatchObject({ action: 'classify', matchValue: 'BLUE BOTTLE', categoryId: 'cat-food', sourceId: '' })
  expect(await screen.findByText('Apply this rule to 5 matching transactions in this import?')).toBeInTheDocument()
  expect(screen.getByText("2 skipped (you've edited these)")).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Apply' }))
  await waitFor(() => expect(posted.apply![0]).toEqual({ ruleId: 'rule-new', scope: 'run', runId: 'r1', scopeSourceId: 's1', includeEdited: false }))

  // clearly labelled second step: the whole source, with its own count
  expect(await screen.findByText('Also apply to all imports from iPhone? 9 transactions match.')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Apply to all imports' }))
  await waitFor(() => expect(posted.apply![1]).toEqual({ ruleId: 'rule-new', scope: 'source', runId: '', scopeSourceId: 's1', includeEdited: false }))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  expect(useUiStore.getState().rulePrompt).toBeNull()
})

it('the include-edited override is opt-in and travels on the apply call', async () => {
  renderPrompt()
  useUiStore.getState().setRulePrompt({ link: link(), diff: { categoryId: 'cat-food' } })
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Create rule' }))
  await user.click(await screen.findByRole('checkbox', { name: 'Also update the 2 transactions you edited' }))
  await user.click(screen.getByRole('button', { name: 'Apply' }))
  await waitFor(() => expect(posted.apply![0]).toMatchObject({ includeEdited: true }))
})

it('"Not now" closes without creating anything', async () => {
  renderPrompt()
  useUiStore.getState().setRulePrompt({ link: link(), diff: { categoryId: 'cat-food' } })
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Not now' }))
  await waitFor(() => expect(useUiStore.getState().rulePrompt).toBeNull())
  expect(posted.create).toBeUndefined()
})

it('a transaction classified by a rule offers to update that rule instead, keeping its match', async () => {
  renderPrompt({ importRules: [rule] })
  useUiStore.getState().setRulePrompt({ link: link({ appliedRuleId: 'rule1', appliedCategoryId: 'cat-salary' }), diff: { categoryId: 'cat-food' } })
  const user = userEvent.setup()
  expect(await screen.findByText('This transaction was classified by the rule “Payee contains BLUE BOTTLE”.')).toBeInTheDocument()
  expect(screen.queryByLabelText('Payee contains')).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Update rule' }))
  await waitFor(() => expect(posted.update![0]).toMatchObject({ id: 'rule1', matchValue: 'BLUE BOTTLE', categoryId: 'cat-food' }))
  expect(posted.create).toBeUndefined()
  expect(await screen.findByText('Apply this rule to 5 matching transactions in this import?')).toBeInTheDocument()
})
```

- [ ] **Step 11: Run to verify it fails**

Run: `pnpm --dir web exec vitest run src/features/imports/RulePromptDialog.test.tsx`
Expected: FAIL — module not found.

- [ ] **Step 12: Implement `RulePromptDialog.tsx`**

```tsx
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import type { ImportRuleMatchField, ImportRuleMatchType, ImportRuleScopeDto, ImportRuleSpecDto } from '@/api/dto/imports'
import { useUiStore } from '@/app/uiStore'
import type { RulePromptParams } from '@/app/uiStore'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { useCategories, useLabels, usePayees, useTags } from '@/features/classifications/queries'
import { apiErrorMessage } from '@/lib/apiError'
import { suggestMatchValue } from '@/lib/importMatch'
import { useApplyImportRule, useCreateImportRule, useImportRules, usePreviewImportRule, useUpdateImportRule } from './queries'

const PREVIEW_DEBOUNCE_MS = 300

type Step = 'define' | 'apply' | 'source'

export function RulePromptDialog() {
  const params = useUiStore((s) => s.rulePrompt)
  const setRulePrompt = useUiStore((s) => s.setRulePrompt)
  if (!params) {
    return null
  }
  // keyed on the link so a new prompt always starts from a fresh form state
  return <RulePrompt key={params.link.id} params={params} onDone={() => setRulePrompt(null)} />
}

// The classification the user just saved, phrased for the prompt copy.
function useDiffSummary(diff: RulePromptParams['diff']) {
  const { t } = useTranslation()
  const { data: categories = [] } = useCategories()
  const { data: payees = [] } = usePayees()
  const { data: tags = [] } = useTags()
  const { data: labels = [] } = useLabels()
  if (diff.categoryId) {
    return t('imports.rules.prompt.changed_category', { name: categories.find((c) => c.id === diff.categoryId)?.name ?? '' })
  }
  if (diff.payeeId) {
    return t('imports.rules.prompt.changed_payee', { name: payees.find((p) => p.id === diff.payeeId)?.name ?? '' })
  }
  if (diff.tagId) {
    return t('imports.rules.prompt.changed_tag', { name: tags.find((x) => x.id === diff.tagId)?.name ?? '' })
  }
  const names = (diff.labelIds ?? []).map((id) => labels.find((l) => l.id === id)?.name ?? '').filter(Boolean)
  return t('imports.rules.prompt.changed_labels', { names: names.join(', ') })
}

function RulePrompt({ params, onDone }: { params: RulePromptParams; onDone: () => void }) {
  const { t } = useTranslation()
  const { link, diff } = params
  const { data: rules = [] } = useImportRules()
  const existing = link.appliedRuleId ? rules.find((r) => r.id === link.appliedRuleId) ?? null : null
  const summary = useDiffSummary(diff)

  const [matchField, setMatchField] = useState<ImportRuleMatchField>('external_payee')
  const [matchType, setMatchType] = useState<ImportRuleMatchType>('contains')
  const [matchValue, setMatchValue] = useState(() => suggestMatchValue(link.externalPayee))
  const [step, setStep] = useState<Step>('define')
  const [includeEdited, setIncludeEdited] = useState(false)
  const [ruleId, setRuleId] = useState<string | null>(null)

  // The rule spec: the existing rule's match (when updating) or the editable
  // one, with the classification the user just chose layered over it.
  const spec: ImportRuleSpecDto = useMemo(() => {
    const base: ImportRuleSpecDto = existing
      ? { ...existing }
      : { sourceId: '', action: 'classify', matchField, matchType, matchValue, isCaseSensitive: false, categoryId: '', payeeId: '', tagId: '', labelIds: [], priority: 0 }
    return {
      ...base,
      categoryId: diff.categoryId ?? base.categoryId,
      payeeId: diff.payeeId ?? base.payeeId,
      tagId: diff.tagId ?? base.tagId,
      labelIds: diff.labelIds ?? base.labelIds,
    }
  }, [existing, matchField, matchType, matchValue, diff])

  const runScope: ImportRuleScopeDto = link.runId
    ? { scope: 'run', runId: link.runId, scopeSourceId: link.sourceId }
    : { scope: 'source', runId: '', scopeSourceId: link.sourceId }
  const sourceScope: ImportRuleScopeDto = { scope: 'source', runId: '', scopeSourceId: link.sourceId }

  const preview = usePreviewImportRule()
  const sourcePreview = usePreviewImportRule()
  const create = useCreateImportRule()
  const update = useUpdateImportRule()
  const apply = useApplyImportRule()

  useEffect(() => {
    if (step !== 'define') {
      return
    }
    const handle = setTimeout(() => preview.mutate({ spec, scope: runScope }), PREVIEW_DEBOUNCE_MS)
    return () => clearTimeout(handle)
    // eslint-disable-next-line react-hooks/exhaustive-deps -- the mutation object is stable; re-run on spec changes only
  }, [spec, step])

  const fieldLabel = matchField === 'description' ? t('imports.rules.field.description') : t('imports.rules.field.external_payee')
  const typeLabel = t(`imports.rules.type.${matchType}`)
  const matched = preview.data?.matched ?? 0
  const alreadyEdited = preview.data?.alreadyEdited ?? 0

  const save = async () => {
    try {
      const saved = existing
        ? await update.mutateAsync({ id: existing.id, spec })
        : await create.mutateAsync({ spec })
      setRuleId(saved.id)
      setStep('apply')
    } catch (err) {
      toast.error(apiErrorMessage(err))
    }
  }

  const applyTo = async (scope: ImportRuleScopeDto, next: Step | null) => {
    if (!ruleId) {
      return
    }
    try {
      const result = await apply.mutateAsync({ ruleId, scope, includeEdited })
      toast.success(t('imports.rules.prompt.applied_toast', { count: result.updated }))
      if (next && link.runId) {
        sourcePreview.mutate({ spec, scope: sourceScope })
        setStep(next)
      } else {
        onDone()
      }
    } catch (err) {
      toast.error(apiErrorMessage(err))
    }
  }

  const body = () => {
    if (step === 'apply') {
      return (
        <>
          <p className="text-sm">{t('imports.rules.prompt.apply_question', { count: matched })}</p>
          {alreadyEdited > 0 ? (
            <>
              <p className="text-sm text-muted-foreground">{t('imports.rules.prompt.skipped_edited', { count: alreadyEdited })}</p>
              <label className="flex items-center gap-2 text-sm">
                <Checkbox checked={includeEdited} onCheckedChange={(v) => setIncludeEdited(v === true)} aria-label={t('imports.rules.prompt.include_edited', { count: alreadyEdited })} />
                {t('imports.rules.prompt.include_edited', { count: alreadyEdited })}
              </label>
            </>
          ) : null}
          <div className={dialogActionsClass}>
            <Button type="button" variant="secondary" onClick={onDone}>{t('imports.rules.prompt.skip_apply')}</Button>
            <Button type="button" disabled={apply.isPending} onClick={() => void applyTo(runScope, 'source')}>{t('imports.rules.prompt.apply')}</Button>
          </div>
        </>
      )
    }
    if (step === 'source') {
      return (
        <>
          <p className="text-sm">{t('imports.rules.prompt.source_question', { source: link.sourceName, count: sourcePreview.data?.matched ?? 0 })}</p>
          <div className={dialogActionsClass}>
            <Button type="button" variant="secondary" onClick={onDone}>{t('imports.rules.prompt.done')}</Button>
            <Button type="button" disabled={apply.isPending || sourcePreview.isPending} onClick={() => void applyTo(sourceScope, null)}>{t('imports.rules.prompt.apply_source')}</Button>
          </div>
        </>
      )
    }
    return (
      <>
        <p className="text-sm">{summary}</p>
        {existing ? (
          <p className="text-sm text-muted-foreground">
            {t('imports.rules.prompt.existing_rule', { rule: `${existing.matchField === 'description' ? t('imports.rules.field.description') : t('imports.rules.field.external_payee')} ${t(`imports.rules.type.${existing.matchType}`)} ${existing.matchValue}` })}
          </p>
        ) : (
          <div className="flex flex-col gap-2">
            <div className="grid grid-cols-2 gap-2">
              <NativeSelect aria-label={t('imports.rules.editor.field')} value={matchField} onChange={(e) => setMatchField(e.target.value as ImportRuleMatchField)}>
                <NativeSelectOption value="external_payee">{t('imports.rules.field.external_payee')}</NativeSelectOption>
                <NativeSelectOption value="description">{t('imports.rules.field.description')}</NativeSelectOption>
              </NativeSelect>
              <NativeSelect aria-label={t('imports.rules.editor.type')} value={matchType} onChange={(e) => setMatchType(e.target.value as ImportRuleMatchType)}>
                <NativeSelectOption value="contains">{t('imports.rules.type.contains')}</NativeSelectOption>
                <NativeSelectOption value="prefix">{t('imports.rules.type.prefix')}</NativeSelectOption>
                <NativeSelectOption value="exact">{t('imports.rules.type.exact')}</NativeSelectOption>
              </NativeSelect>
            </div>
            <Label htmlFor="rule-prompt-value" className="sr-only">{`${fieldLabel} ${typeLabel}`}</Label>
            <Input id="rule-prompt-value" value={matchValue} onChange={(e) => setMatchValue(e.target.value)} autoComplete="off" />
          </div>
        )}
        <p className="text-sm text-muted-foreground" aria-live="polite">
          {preview.isPending ? t('common.app.modal.loading.data_loading') : t('imports.rules.prompt.match_count', { count: matched })}
        </p>
        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={onDone}>{t('imports.rules.prompt.not_now')}</Button>
          <Button type="button" disabled={create.isPending || update.isPending || spec.matchValue.trim() === ''} onClick={() => void save()}>
            {existing ? t('imports.rules.prompt.update_rule') : t('imports.rules.prompt.create_rule')}
          </Button>
        </div>
      </>
    )
  }

  return (
    <ResponsiveDialog open onOpenChange={(o) => !o && onDone()} title={t('imports.rules.prompt.title')}>
      <div className="flex flex-col gap-3">{body()}</div>
    </ResponsiveDialog>
  )
}
```

The accessible name the tests use (`Payee contains`) is the sr-only `<Label>` = `"{field} {type}"` with `imports.rules.field.external_payee = "Payee"` and `imports.rules.type.contains = "contains"`. `useDiffSummary` reads `changed_category` = `"You set the category to {name}. Create a rule for similar imports?"`.

- [ ] **Step 13: English catalogue keys used by this task**

Add to `locales/en.json` under `imports` (after `provenance`), a new `rules` object. Task 15 copies the same keys into the other ten catalogues; only `en` is needed to make the tests pass now:

```json
"rules": {
  "field": { "external_payee": "Payee", "description": "Description" },
  "type": { "contains": "contains", "prefix": "starts with", "exact": "is exactly" },
  "prompt": {
    "title": "Create an import rule",
    "changed_category": "You set the category to {name}. Create a rule for similar imports?",
    "changed_payee": "You set the payee to {name}. Create a rule for similar imports?",
    "changed_tag": "You set the tag to {name}. Create a rule for similar imports?",
    "changed_labels": "You added the labels {names}. Create a rule for similar imports?",
    "existing_rule": "This transaction was classified by the rule “{rule}”.",
    "match_count": "Matches {count} transactions in this import",
    "not_now": "Not now",
    "create_rule": "Create rule",
    "update_rule": "Update rule",
    "apply_question": "Apply this rule to {count} matching transactions in this import?",
    "skipped_edited": "{count} skipped (you've edited these)",
    "include_edited": "Also update the {count} transactions you edited",
    "skip_apply": "Skip",
    "apply": "Apply",
    "applied_toast": "Updated {count} transactions",
    "source_question": "Also apply to all imports from {source}? {count} transactions match.",
    "apply_source": "Apply to all imports",
    "done": "Done"
  },
  "editor": { "field": "Match field", "type": "Match type" }
}
```

(`editor.*`, `action.*`, `page.*` and `suggest.*` are added in Task 14; Task 15 translates the whole subtree.) Plural strings here are single-form on purpose — the count is always shown as a number; the plural-pipe convention (`pluralPick`) is reserved for strings whose noun must inflect, and "transactions" reads fine for 1 in this prompt copy.

- [ ] **Step 14: Mount the dialog**

`web/src/app/layouts/ApplicationLayout.tsx`: import `{ RulePromptDialog } from '@/features/imports/RulePromptDialog'` and render `<RulePromptDialog />` immediately after `<TransactionDialog />` (line 286).

- [ ] **Step 15: Run the tests and the type-check**

Run: `pnpm --dir web exec vitest run src/features/imports/RulePromptDialog.test.tsx src/features/transactions/TransactionDialog.test.tsx src/lib/importMatch.test.ts && pnpm --dir web exec tsc -b && pnpm --dir web lint`
Expected: PASS; tsc clean; lint adds no new warnings (the `eslint-disable` comment is harmless under oxlint — if oxlint flags it as an unknown directive, drop the comment and list `preview` in the dependency array instead: `useMutation` returns a stable `mutate`, so destructure `const { mutate: runPreview } = preview` and depend on `runPreview`).

- [ ] **Step 16: Commit**

```bash
git add web/src/lib/importMatch.ts web/src/lib/importMatch.test.ts web/src/features/imports/RulePromptDialog.tsx web/src/features/imports/RulePromptDialog.test.tsx web/src/app/uiStore.ts web/src/features/transactions/TransactionDialog.tsx web/src/features/transactions/TransactionDialog.test.tsx web/src/app/layouts/ApplicationLayout.tsx locales/en.json
git commit -m "feat(web): prompt to create an import rule after correcting an imported transaction

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 14: Rules management page — list, edit, reorder, delete, AI suggestions

**Files:**
- Create: `web/src/features/imports/ImportRulesPage.tsx`, `web/src/features/imports/ImportRulesPage.test.tsx`
- Create: `web/src/features/imports/ImportRuleDialog.tsx`
- Create: `web/src/features/imports/ruleSummary.ts` (shared "Payee contains X" + target-name formatting, used by the page, the dialog, and re-used by `RulePromptDialog`'s `existing_rule` line — replace the inline template there with `describeMatch`)
- Modify: `web/src/app/router-pages.ts` (`SETTINGS_IMPORT_RULES: '/settings/import-rules'` after `SETTINGS_SIMPLEFIN`), `web/src/app/routes.tsx` (route after line 75 `/settings/simplefin`), `web/src/features/imports/ImportsDataPage.tsx` (a `LinkRow` before the history row), `web/src/features/settings/SettingsPage.tsx` (a `MenuRow` after line 152's SimpleFIN row)
- Modify: `locales/en.json` (`imports.rules.page.*`, `imports.rules.editor.*`, `imports.rules.suggest.*`)

**Interfaces:**
- Consumes: `useImportRules`, `useCreateImportRule({spec, id?})`, `useUpdateImportRule({id, spec})`, `useDeleteImportRule(id)`, `usePreviewImportRule` (`mutate({spec, scope})`), `useSuggestImportRules` (`mutate(scope)`) (Task 12); `isAiEnabled()` (Task 11); `SortableList`, `ConfirmDialog`, `Badge`, `NativeSelect`, `Checkbox`, `Input`, `Label`, `Button`, `InfoBox`, `SettingsShell`; `useCategories/usePayees/useTags/useLabels`.
- Produces: `describeMatch(spec, t)`, `describeTargets(spec, names)` in `ruleSummary.ts`; `<ImportRuleDialog open rule? initial? onClose />`; `<ImportRulesPage />`; `RouterPage.SETTINGS_IMPORT_RULES`.

**Spec rules this task encodes (Part 9):** rules are listed in priority order and reordered by drag; skip rules are visibly distinct from classify rules; each rule can be edited and deleted; "Suggest rules" appears only when the server reports `AI_ENABLED`, and each suggestion can be accepted, edited (opens the editor prefilled), or discarded, with a live match count.

- [ ] **Step 1: Write the failing page test**

`web/src/features/imports/ImportRulesPage.test.tsx`:

```tsx
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import type { ImportRuleDto } from '@/api/dto/imports'
import { ImportRulesPage } from './ImportRulesPage'

vi.mock('@/hooks/useIsCompact', () => ({ useIsCompact: () => false }))
vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))

const rule = (over: Partial<ImportRuleDto> = {}): ImportRuleDto => ({
  id: 'rule1', sourceId: '', action: 'classify', matchField: 'external_payee', matchType: 'contains', matchValue: 'BLUE BOTTLE',
  isCaseSensitive: false, categoryId: 'cat-food', payeeId: '', tagId: '', labelIds: ['label1'], priority: 0,
  createdAt: '2026-09-01 00:00:00', updatedAt: '2026-09-01 00:00:00', ...over,
})
const skipRule = rule({ id: 'rule2', action: 'skip', matchField: 'description', matchType: 'prefix', matchValue: 'PAYMENT THANK YOU', categoryId: '', labelIds: [], priority: 1 })

const posted: Record<string, unknown[]> = {}
function renderPage(data: Record<string, unknown> = {}) {
  server.use(...coreHandlers({ importRules: [rule(), skipRule], ...data }))
  server.use(
    http.post('*/api/v1/import/preview-rule', async ({ request }) => {
      const body = await request.json() as Record<string, unknown>
      ;(posted.preview ??= []).push(body)
      return HttpResponse.json({ success: true, message: '', data: { matched: 7, alreadyEdited: 1 } })
    }),
    http.post('*/api/v1/import/create-rule', async ({ request }) => {
      const body = await request.json() as Record<string, unknown>
      ;(posted.create ??= []).push(body)
      return HttpResponse.json({ success: true, message: '', data: rule({ ...(body as Partial<ImportRuleDto>), id: `rule-${posted.create!.length}` }) })
    }),
    http.post('*/api/v1/import/update-rule', async ({ request }) => {
      const body = await request.json() as Record<string, unknown>
      ;(posted.update ??= []).push(body)
      return HttpResponse.json({ success: true, message: '', data: rule(body as Partial<ImportRuleDto>) })
    }),
    http.post('*/api/v1/import/delete-rule', async ({ request }) => {
      ;(posted.delete ??= []).push(await request.json())
      return HttpResponse.json({ success: true, message: '', data: null })
    }),
    http.post('*/api/v1/import/suggest-rules', async ({ request }) => {
      ;(posted.suggest ??= []).push(await request.json())
      return HttpResponse.json({ success: true, message: '', data: { items: [
        { sourceId: '', action: 'classify', matchField: 'external_payee', matchType: 'contains', matchValue: 'UBER', isCaseSensitive: false, categoryId: 'cat-food', payeeId: '', tagId: '', labelIds: [], priority: 0, reason: '4 rides last month' },
      ] } })
    }),
  )
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter([{ path: '/settings/import-rules', element: <ImportRulesPage /> }], { initialEntries: ['/settings/import-rules'] })
  render(<QueryClientProvider client={queryClient}><RouterProvider router={router} /></QueryClientProvider>)
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({ matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn() }))
  for (const k of Object.keys(posted)) delete posted[k]
})

it('lists rules in priority order with skip rules marked and targets named', async () => {
  renderPage()
  const rows = await screen.findAllByRole('listitem')
  expect(rows).toHaveLength(2)
  expect(within(rows[0]).getByText('Payee contains BLUE BOTTLE')).toBeInTheDocument()
  expect(within(rows[0]).getByText('Food · health')).toBeInTheDocument()
  expect(within(rows[1]).getByText('Description starts with PAYMENT THANK YOU')).toBeInTheDocument()
  expect(within(rows[1]).getByText('Skip')).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Suggest rules' })).not.toBeInTheDocument()
})

it('creates a rule from the editor with a live match count', async () => {
  renderPage()
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Add rule' }))
  const dialog = await screen.findByRole('dialog')
  await user.type(within(dialog).getByLabelText('Match value'), 'IKEA')
  await user.selectOptions(within(dialog).getByLabelText('Category'), 'cat-food')
  await user.click(within(dialog).getByRole('checkbox', { name: 'health' }))
  expect(await within(dialog).findByText('Matches 7 imported transactions')).toBeInTheDocument()
  expect(posted.preview!.at(-1)).toMatchObject({ matchValue: 'IKEA', scope: 'all', runId: '', scopeSourceId: '' })
  await user.click(within(dialog).getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(posted.create![0]).toMatchObject({ action: 'classify', matchField: 'external_payee', matchType: 'contains', matchValue: 'IKEA', categoryId: 'cat-food', labelIds: ['label1'], priority: 2 }))
  expect(await screen.findByText('Payee contains IKEA')).toBeInTheDocument()
})

it('a skip rule hides the targets and posts none', async () => {
  renderPage()
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Add rule' }))
  const dialog = await screen.findByRole('dialog')
  await user.selectOptions(within(dialog).getByLabelText('Action'), 'skip')
  expect(within(dialog).queryByLabelText('Category')).not.toBeInTheDocument()
  await user.type(within(dialog).getByLabelText('Match value'), 'TRANSFER')
  await user.click(within(dialog).getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(posted.create![0]).toMatchObject({ action: 'skip', matchValue: 'TRANSFER', categoryId: '', payeeId: '', tagId: '', labelIds: [] }))
})

it('edits and deletes a rule', async () => {
  renderPage()
  const user = userEvent.setup()
  const rows = await screen.findAllByRole('listitem')
  await user.click(within(rows[0]).getByRole('button', { name: 'Edit' }))
  const dialog = await screen.findByRole('dialog')
  const value = within(dialog).getByLabelText('Match value') as HTMLInputElement
  expect(value.value).toBe('BLUE BOTTLE')
  await user.clear(value)
  await user.type(value, 'BLUE BOTTLE COFFEE')
  await user.click(within(dialog).getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(posted.update![0]).toMatchObject({ id: 'rule1', matchValue: 'BLUE BOTTLE COFFEE', priority: 0 }))

  await user.click(within((await screen.findAllByRole('listitem'))[1]).getByRole('button', { name: 'Delete' }))
  await user.click(await screen.findByRole('button', { name: 'Delete rule' }))
  await waitFor(() => expect(posted.delete![0]).toEqual({ id: 'rule2' }))
  await waitFor(() => expect(screen.getAllByRole('listitem')).toHaveLength(1))
})

it('with AI enabled, suggestions can be accepted or discarded', async () => {
  window.econumoConfig = { AI_ENABLED: true }
  renderPage()
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Suggest rules' }))
  await waitFor(() => expect(posted.suggest![0]).toEqual({ scope: 'all', runId: '', scopeSourceId: '' }))
  const suggestion = await screen.findByRole('region', { name: 'Suggested rules' })
  expect(within(suggestion).getByText('Payee contains UBER')).toBeInTheDocument()
  expect(within(suggestion).getByText('4 rides last month')).toBeInTheDocument()
  expect(await within(suggestion).findByText('Matches 7 imported transactions')).toBeInTheDocument()
  await user.click(within(suggestion).getByRole('button', { name: 'Accept' }))
  await waitFor(() => expect(posted.create![0]).toMatchObject({ matchValue: 'UBER', categoryId: 'cat-food', priority: 2 }))
  await waitFor(() => expect(screen.queryByRole('region', { name: 'Suggested rules' })).not.toBeInTheDocument())
})

it('reordering persists the new priorities', async () => {
  renderPage()
  const user = userEvent.setup()
  const rows = await screen.findAllByRole('listitem')
  // dnd-kit keyboard sorting: focus the grip, Space to lift, ArrowDown, Space to drop
  const grip = within(rows[0]).getByRole('button', { name: 'Reorder' })
  grip.focus()
  await user.keyboard(' ')
  await user.keyboard('{ArrowDown}')
  await user.keyboard(' ')
  await waitFor(() => expect(posted.update).toHaveLength(2))
  expect(posted.update).toEqual(expect.arrayContaining([
    expect.objectContaining({ id: 'rule2', priority: 0 }),
    expect.objectContaining({ id: 'rule1', priority: 1 }),
  ]))
})
```

If the keyboard-sorting case proves flaky under jsdom (dnd-kit measures layout), replace the interaction with a direct call of the page's reorder handler by rendering `<SortableList>` through a test-only prop — the existing `SortableList` consumers (`grep -rn "SortableList" web/src --include=*.test.tsx`) show which approach the repo already uses; follow that one.

- [ ] **Step 2: Run to verify it fails**

Run: `pnpm --dir web exec vitest run src/features/imports/ImportRulesPage.test.tsx`
Expected: FAIL — module not found.

- [ ] **Step 3: `ruleSummary.ts`**

```ts
import type { TFunction } from 'i18next'
import type { ImportRuleSpecDto } from '@/api/dto/imports'

// The wire spec only: a stored rule or a suggestion carries extra keys
// (id/createdAt/updatedAt, reason) that must not be posted back.
export function ruleSpec(r: ImportRuleSpecDto): ImportRuleSpecDto {
  const { sourceId, action, matchField, matchType, matchValue, isCaseSensitive, categoryId, payeeId, tagId, labelIds, priority } = r
  return { sourceId, action, matchField, matchType, matchValue, isCaseSensitive, categoryId, payeeId, tagId, labelIds, priority }
}

export function describeMatch(spec: Pick<ImportRuleSpecDto, 'matchField' | 'matchType' | 'matchValue'>, t: TFunction): string {
  return `${t(`imports.rules.field.${spec.matchField}`)} ${t(`imports.rules.type.${spec.matchType}`)} ${spec.matchValue}`
}

export interface TargetNames {
  category: (id: string) => string | undefined
  payee: (id: string) => string | undefined
  tag: (id: string) => string | undefined
  label: (id: string) => string | undefined
}

// "Food · Grocer · health" — every target the rule sets, in a fixed order.
export function describeTargets(spec: ImportRuleSpecDto, names: TargetNames): string {
  const parts = [
    spec.categoryId ? names.category(spec.categoryId) : undefined,
    spec.payeeId ? names.payee(spec.payeeId) : undefined,
    spec.tagId ? names.tag(spec.tagId) : undefined,
    ...spec.labelIds.map(names.label),
  ]
  return parts.filter((p): p is string => Boolean(p)).join(' · ')
}
```

Also add a hook in the same file so the three consumers resolve names one way:

```ts
import { useCategories, useLabels, usePayees, useTags } from '@/features/classifications/queries'

export function useTargetNames(): TargetNames {
  const { data: categories = [] } = useCategories()
  const { data: payees = [] } = usePayees()
  const { data: tags = [] } = useTags()
  const { data: labels = [] } = useLabels()
  return {
    category: (id) => categories.find((c) => c.id === id)?.name,
    payee: (id) => payees.find((p) => p.id === id)?.name,
    tag: (id) => tags.find((x) => x.id === id)?.name,
    label: (id) => labels.find((l) => l.id === id)?.name,
  }
}
```

In `RulePromptDialog.tsx` replace the inline `existing_rule` template argument with `describeMatch(existing, t)`.

- [ ] **Step 4: `ImportRuleDialog.tsx`**

```tsx
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import type { ImportRuleAction, ImportRuleDto, ImportRuleMatchField, ImportRuleMatchType, ImportRuleSpecDto } from '@/api/dto/imports'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { useCategories, useLabels, usePayees, useTags } from '@/features/classifications/queries'
import { apiErrorMessage } from '@/lib/apiError'
import { ruleSpec } from './ruleSummary'
import { useCreateImportRule, usePreviewImportRule, useUpdateImportRule } from './queries'

const PREVIEW_DEBOUNCE_MS = 300
const ALL_SCOPE = { scope: 'all', runId: '', scopeSourceId: '' } as const

export const emptyRuleSpec = (priority: number): ImportRuleSpecDto => ({
  sourceId: '', action: 'classify', matchField: 'external_payee', matchType: 'contains', matchValue: '', isCaseSensitive: false,
  categoryId: '', payeeId: '', tagId: '', labelIds: [], priority,
})

interface ImportRuleDialogProps {
  open: boolean
  // editing an existing rule (its id is posted to update-rule) ...
  rule?: ImportRuleDto
  // ... or creating one, optionally prefilled (an accepted-for-editing suggestion)
  initial?: ImportRuleSpecDto
  onClose: () => void
  onSaved?: (rule: ImportRuleDto) => void
}

export function ImportRuleDialog({ open, rule, initial, onClose, onSaved }: ImportRuleDialogProps) {
  const { t } = useTranslation()
  const [spec, setSpec] = useState<ImportRuleSpecDto>(() => rule ? ruleSpec(rule) : initial ?? emptyRuleSpec(0))
  const { data: categories = [] } = useCategories()
  const { data: payees = [] } = usePayees()
  const { data: tags = [] } = useTags()
  const { data: labels = [] } = useLabels()
  const preview = usePreviewImportRule()
  const create = useCreateImportRule()
  const update = useUpdateImportRule()

  useEffect(() => {
    if (!open || spec.matchValue.trim() === '') {
      return
    }
    const { mutate: runPreview } = preview
    const handle = setTimeout(() => runPreview({ spec, scope: ALL_SCOPE }), PREVIEW_DEBOUNCE_MS)
    return () => clearTimeout(handle)
  }, [open, spec, preview.mutate])

  const patch = (p: Partial<ImportRuleSpecDto>) => setSpec((s) => ({ ...s, ...p }))
  const setAction = (action: ImportRuleAction) =>
    // a skip rule carries no targets: clear them so the server never sees stale ones
    setSpec((s) => action === 'skip' ? { ...s, action, categoryId: '', payeeId: '', tagId: '', labelIds: [] } : { ...s, action })
  const toggleLabel = (id: string) =>
    setSpec((s) => ({ ...s, labelIds: s.labelIds.includes(id) ? s.labelIds.filter((l) => l !== id) : [...s.labelIds, id] }))

  const save = async () => {
    try {
      const saved = rule
        ? await update.mutateAsync({ id: rule.id, spec })
        : await create.mutateAsync({ spec })
      onSaved?.(saved)
      onClose()
    } catch (err) {
      toast.error(apiErrorMessage(err))
    }
  }

  const canSave = spec.matchValue.trim() !== '' && (spec.action === 'skip' || spec.categoryId || spec.payeeId || spec.tagId || spec.labelIds.length > 0)

  return (
    <ResponsiveDialog open={open} onOpenChange={(o) => !o && onClose()} title={rule ? t('imports.rules.editor.title_edit') : t('imports.rules.editor.title_create')}>
      <div className="flex flex-col gap-3">
        <div className="flex flex-col gap-1">
          <Label htmlFor="rule-action">{t('imports.rules.editor.action')}</Label>
          <NativeSelect id="rule-action" className="w-full" value={spec.action} onChange={(e) => setAction(e.target.value as ImportRuleAction)}>
            <NativeSelectOption value="classify">{t('imports.rules.action.classify')}</NativeSelectOption>
            <NativeSelectOption value="skip">{t('imports.rules.action.skip')}</NativeSelectOption>
          </NativeSelect>
        </div>
        <div className="grid grid-cols-2 gap-2">
          <div className="flex flex-col gap-1">
            <Label htmlFor="rule-field">{t('imports.rules.editor.field')}</Label>
            <NativeSelect id="rule-field" className="w-full" value={spec.matchField} onChange={(e) => patch({ matchField: e.target.value as ImportRuleMatchField })}>
              <NativeSelectOption value="external_payee">{t('imports.rules.field.external_payee')}</NativeSelectOption>
              <NativeSelectOption value="description">{t('imports.rules.field.description')}</NativeSelectOption>
            </NativeSelect>
          </div>
          <div className="flex flex-col gap-1">
            <Label htmlFor="rule-type">{t('imports.rules.editor.type')}</Label>
            <NativeSelect id="rule-type" className="w-full" value={spec.matchType} onChange={(e) => patch({ matchType: e.target.value as ImportRuleMatchType })}>
              <NativeSelectOption value="contains">{t('imports.rules.type.contains')}</NativeSelectOption>
              <NativeSelectOption value="prefix">{t('imports.rules.type.prefix')}</NativeSelectOption>
              <NativeSelectOption value="exact">{t('imports.rules.type.exact')}</NativeSelectOption>
            </NativeSelect>
          </div>
        </div>
        <div className="flex flex-col gap-1">
          <Label htmlFor="rule-value">{t('imports.rules.editor.value')}</Label>
          <Input id="rule-value" value={spec.matchValue} maxLength={255} autoComplete="off" onChange={(e) => patch({ matchValue: e.target.value })} />
        </div>
        <label className="flex items-center gap-2 text-sm">
          <Checkbox checked={spec.isCaseSensitive} onCheckedChange={(v) => patch({ isCaseSensitive: v === true })} aria-label={t('imports.rules.editor.case_sensitive')} />
          {t('imports.rules.editor.case_sensitive')}
        </label>
        {spec.action === 'classify' ? (
          <>
            <div className="flex flex-col gap-1">
              <Label htmlFor="rule-category">{t('imports.rules.editor.category')}</Label>
              <NativeSelect id="rule-category" className="w-full" value={spec.categoryId} onChange={(e) => patch({ categoryId: e.target.value })}>
                <NativeSelectOption value="">—</NativeSelectOption>
                {categories.filter((c) => !c.isArchived || c.id === spec.categoryId).map((c) => <NativeSelectOption key={c.id} value={c.id}>{c.name}</NativeSelectOption>)}
              </NativeSelect>
            </div>
            <div className="flex flex-col gap-1">
              <Label htmlFor="rule-payee">{t('imports.rules.editor.payee')}</Label>
              <NativeSelect id="rule-payee" className="w-full" value={spec.payeeId} onChange={(e) => patch({ payeeId: e.target.value })}>
                <NativeSelectOption value="">—</NativeSelectOption>
                {payees.filter((p) => !p.isArchived || p.id === spec.payeeId).map((p) => <NativeSelectOption key={p.id} value={p.id}>{p.name}</NativeSelectOption>)}
              </NativeSelect>
            </div>
            <div className="flex flex-col gap-1">
              <Label htmlFor="rule-tag">{t('imports.rules.editor.tag')}</Label>
              <NativeSelect id="rule-tag" className="w-full" value={spec.tagId} onChange={(e) => patch({ tagId: e.target.value })}>
                <NativeSelectOption value="">—</NativeSelectOption>
                {tags.filter((x) => !x.isArchived || x.id === spec.tagId).map((x) => <NativeSelectOption key={x.id} value={x.id}>{x.name}</NativeSelectOption>)}
              </NativeSelect>
            </div>
            {labels.length > 0 ? (
              <fieldset className="flex flex-col gap-1">
                <legend className="text-sm">{t('imports.rules.editor.labels')}</legend>
                <div className="flex flex-wrap gap-3">
                  {labels.filter((l) => !l.isArchived || spec.labelIds.includes(l.id)).map((l) => (
                    <label key={l.id} className="flex items-center gap-2 text-sm">
                      <Checkbox checked={spec.labelIds.includes(l.id)} onCheckedChange={() => toggleLabel(l.id)} aria-label={l.name} />
                      {l.name}
                    </label>
                  ))}
                </div>
              </fieldset>
            ) : null}
          </>
        ) : null}
        <p className="text-sm text-muted-foreground" aria-live="polite">
          {spec.matchValue.trim() === '' ? '' : preview.isPending ? t('common.app.modal.loading.data_loading') : t('imports.rules.editor.match_count', { count: preview.data?.matched ?? 0 })}
        </p>
        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={onClose}>{t('common.app.form.cancel.label')}</Button>
          <Button type="button" disabled={!canSave || create.isPending || update.isPending} onClick={() => void save()}>{t('common.app.form.save.label')}</Button>
        </div>
      </div>
    </ResponsiveDialog>
  )
}
```

The page mounts the dialog with a `key` so `useState`'s initializer runs afresh per open (see Step 5). `common.app.form.cancel.label` / `common.app.form.save.label` / `common.app.modal.loading.data_loading` are existing shared keys — do not add duplicates.

- [ ] **Step 5: `ImportRulesPage.tsx`**

```tsx
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { GripVertical } from 'lucide-react'
import { toast } from 'sonner'
import type { ImportRuleDto, ImportRuleSpecDto, ImportRuleSuggestionDto } from '@/api/dto/imports'
import { RouterPage } from '@/app/router-pages'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { InfoBox } from '@/components/InfoBox'
import { SortableList } from '@/components/SortableList'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { SettingsShell } from '@/features/settings/SettingsShell'
import { apiErrorMessage } from '@/lib/apiError'
import { isAiEnabled } from '@/lib/config'
import { ImportRuleDialog, emptyRuleSpec } from './ImportRuleDialog'
import { describeMatch, describeTargets, ruleSpec, useTargetNames } from './ruleSummary'
import { useCreateImportRule, useDeleteImportRule, useImportRules, usePreviewImportRule, useSuggestImportRules, useUpdateImportRule } from './queries'

const ALL_SCOPE = { scope: 'all', runId: '', scopeSourceId: '' } as const

type Editor = { kind: 'closed' } | { kind: 'create'; initial: ImportRuleSpecDto } | { kind: 'edit'; rule: ImportRuleDto }

export function ImportRulesPage() {
  const { t } = useTranslation()
  const { data: rules = [], isPending } = useImportRules()
  const names = useTargetNames()
  const update = useUpdateImportRule()
  const remove = useDeleteImportRule()
  const create = useCreateImportRule()
  const suggest = useSuggestImportRules()
  const [editor, setEditor] = useState<Editor>({ kind: 'closed' })
  const [deleting, setDeleting] = useState<ImportRuleDto | null>(null)
  const [suggestions, setSuggestions] = useState<ImportRuleSuggestionDto[] | null>(null)

  // Priority is the list position; a drop rewrites only the rows that moved.
  const reorder = async (orderedIds: string[]) => {
    const byId = new Map(rules.map((r) => [r.id, r]))
    try {
      for (const [index, id] of orderedIds.entries()) {
        const r = byId.get(id)
        if (r && r.priority !== index) {
          await update.mutateAsync({ id, spec: { ...ruleSpec(r), priority: index } })
        }
      }
    } catch (err) {
      toast.error(apiErrorMessage(err))
    }
  }

  const confirmDelete = async () => {
    if (!deleting) {
      return
    }
    try {
      await remove.mutateAsync(deleting.id)
    } catch (err) {
      toast.error(apiErrorMessage(err))
    } finally {
      setDeleting(null)
    }
  }

  const runSuggest = () => suggest.mutate(ALL_SCOPE, {
    onSuccess: (items) => setSuggestions(items),
    onError: (err) => toast.error(apiErrorMessage(err)),
  })

  const accept = async (s: ImportRuleSuggestionDto) => {
    try {
      await create.mutateAsync({ spec: { ...ruleSpec(s), priority: rules.length } })
      discard(s)
    } catch (err) {
      toast.error(apiErrorMessage(err))
    }
  }
  const discard = (s: ImportRuleSuggestionDto) =>
    setSuggestions((list) => {
      const next = (list ?? []).filter((x) => x !== s)
      return next.length > 0 ? next : null
    })

  return (
    <SettingsShell title={t('imports.rules.page.title')} backTo={RouterPage.SETTINGS_DATA}>
      <div className="mx-auto flex w-full max-w-xl flex-col gap-3">
        <InfoBox>{t('imports.rules.page.intro')}</InfoBox>
        <div className="flex flex-wrap gap-2">
          <Button type="button" onClick={() => setEditor({ kind: 'create', initial: emptyRuleSpec(rules.length) })}>{t('imports.rules.page.add')}</Button>
          {isAiEnabled() ? (
            <Button type="button" variant="secondary" disabled={suggest.isPending} onClick={runSuggest}>
              {suggest.isPending ? t('imports.rules.suggest.running') : t('imports.rules.suggest.button')}
            </Button>
          ) : null}
        </div>
        {suggestions ? (
          <section aria-label={t('imports.rules.suggest.heading')} className="flex flex-col gap-2 rounded-lg border p-3">
            <h2 className="text-sm font-medium">{t('imports.rules.suggest.heading')}</h2>
            {suggestions.map((s, i) => (
              <SuggestionRow key={i} suggestion={s} targets={describeTargets(s, names)}
                onAccept={() => void accept(s)} onEdit={() => { setEditor({ kind: 'create', initial: { ...ruleSpec(s), priority: rules.length } }); discard(s) }} onDiscard={() => discard(s)} />
            ))}
          </section>
        ) : null}
        {isPending ? (
          <p className="text-sm text-muted-foreground">{t('common.app.modal.loading.data_loading')}</p>
        ) : rules.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('imports.rules.page.empty')}</p>
        ) : (
          <SortableList items={rules} onReorder={(ids) => void reorder(ids)} disabled={update.isPending} renderItem={(r, handle) => (
            <div className="flex items-center gap-2 rounded-lg bg-econumo-card px-3 py-2.5 text-sm">
              <button type="button" aria-label={t('imports.rules.page.reorder')} className="cursor-grab touch-none text-muted-foreground" {...handle.attributes} {...handle.listeners}>
                <GripVertical className="size-4" />
              </button>
              <div className="flex min-w-0 flex-1 flex-col">
                <span className="truncate">{describeMatch(r, t)}</span>
                {r.action === 'skip'
                  ? <span className="text-xs text-muted-foreground">{t('imports.rules.page.skip_hint')}</span>
                  : <span className="truncate text-xs text-muted-foreground">{describeTargets(r, names)}</span>}
              </div>
              <Badge variant={r.action === 'skip' ? 'destructive' : 'secondary'}>{t(`imports.rules.action.${r.action}`)}</Badge>
              <Button type="button" variant="ghost" size="sm" onClick={() => setEditor({ kind: 'edit', rule: r })}>{t('imports.rules.page.edit')}</Button>
              <Button type="button" variant="ghost" size="sm" onClick={() => setDeleting(r)}>{t('imports.rules.page.delete')}</Button>
            </div>
          )} />
        )}
      </div>
      {editor.kind !== 'closed' ? (
        <ImportRuleDialog key={editor.kind === 'edit' ? editor.rule.id : 'create'} open
          rule={editor.kind === 'edit' ? editor.rule : undefined}
          initial={editor.kind === 'create' ? editor.initial : undefined}
          onClose={() => setEditor({ kind: 'closed' })} />
      ) : null}
      <ConfirmDialog open={deleting !== null} onClose={() => setDeleting(null)} onConfirm={() => void confirmDelete()}
        title={t('imports.rules.page.delete_title')} question={deleting ? describeMatch(deleting, t) : ''}
        confirmLabel={t('imports.rules.page.delete_confirm')} cancelLabel={t('common.app.form.cancel.label')} destructive />
    </SettingsShell>
  )
}

function SuggestionRow({ suggestion, targets, onAccept, onEdit, onDiscard }: { suggestion: ImportRuleSuggestionDto; targets: string; onAccept: () => void; onEdit: () => void; onDiscard: () => void }) {
  const { t } = useTranslation()
  const preview = usePreviewImportRule()
  const { mutate: runPreview } = preview
  useEffect(() => {
    runPreview({ spec: suggestion, scope: ALL_SCOPE })
  }, [suggestion, runPreview])
  return (
    <div className="flex flex-col gap-1 rounded-lg bg-econumo-card px-3 py-2.5 text-sm">
      <span>{describeMatch(suggestion, t)}</span>
      <span className="text-xs text-muted-foreground">{targets}</span>
      <span className="text-xs text-muted-foreground">{suggestion.reason}</span>
      <span className="text-xs text-muted-foreground" aria-live="polite">
        {preview.isPending || !preview.data ? t('common.app.modal.loading.data_loading') : t('imports.rules.editor.match_count', { count: preview.data.matched })}
      </span>
      <div className="flex gap-2 pt-1">
        <Button type="button" size="sm" onClick={onAccept}>{t('imports.rules.suggest.accept')}</Button>
        <Button type="button" size="sm" variant="secondary" onClick={onEdit}>{t('imports.rules.page.edit')}</Button>
        <Button type="button" size="sm" variant="ghost" onClick={onDiscard}>{t('imports.rules.suggest.discard')}</Button>
      </div>
    </div>
  )
}
```

Every `list`/`listitem` role in the test comes from `SortableList`'s `<ul>/<li>` — keep the suggestions section a `<section>` with `<div>` children so the `listitem` count is the rule list only.

- [ ] **Step 6: Routing, menu entries, catalogue keys**

`web/src/app/router-pages.ts`: add `SETTINGS_IMPORT_RULES: '/settings/import-rules',` after `SETTINGS_SIMPLEFIN`.
`web/src/app/routes.tsx`: import `ImportRulesPage` and add `{ path: '/settings/import-rules', element: <ImportRulesPage /> },` after the `/settings/simplefin` route (line 75).
`web/src/features/imports/ImportsDataPage.tsx`: before the history `LinkRow`, add `<LinkRow label={t('imports.rules.page.menu_item')} to={RouterPage.SETTINGS_IMPORT_RULES} />`.
`web/src/features/settings/SettingsPage.tsx`: after the SimpleFIN `MenuRow` (line 152), add `<MenuRow label={t('imports.rules.page.menu_item')} to={RouterPage.SETTINGS_IMPORT_RULES} />`.

`locales/en.json`, inside the `imports.rules` object created in Task 13 — add `action`, extend `editor`, add `page` and `suggest`:

```json
"action": { "classify": "Classify", "skip": "Skip" },
"editor": {
  "title_create": "New import rule",
  "title_edit": "Edit import rule",
  "action": "Action",
  "field": "Match field",
  "type": "Match type",
  "value": "Match value",
  "case_sensitive": "Case sensitive",
  "category": "Category",
  "payee": "Payee",
  "tag": "Tag",
  "labels": "Labels",
  "match_count": "Matches {count} imported transactions"
},
"page": {
  "menu_item": "Import rules",
  "title": "Import rules",
  "intro": "Rules classify or skip imported transactions automatically. They run top to bottom; drag to change the order.",
  "add": "Add rule",
  "empty": "No rules yet. Correct an imported transaction and Econumo will offer to create one.",
  "reorder": "Reorder",
  "edit": "Edit",
  "delete": "Delete",
  "delete_title": "Delete this rule?",
  "delete_confirm": "Delete rule",
  "skip_hint": "Matching transactions are not imported"
},
"suggest": {
  "button": "Suggest rules",
  "running": "Thinking…",
  "heading": "Suggested rules",
  "accept": "Accept",
  "discard": "Discard"
}
```

- [ ] **Step 7: Run the tests, type-check, lint**

Run: `pnpm --dir web exec vitest run src/features/imports && pnpm --dir web exec tsc -b && pnpm --dir web lint`
Expected: PASS; tsc clean; no new lint warnings. If `metrics-coverage.test.ts` is unaffected (all three rule metrics are fired from the Task 12 hooks), it stays green — confirm with `pnpm --dir web exec vitest run src/lib/metrics-coverage.test.ts`.

- [ ] **Step 8: Commit**

```bash
git add web/src/features/imports/ImportRulesPage.tsx web/src/features/imports/ImportRulesPage.test.tsx web/src/features/imports/ImportRuleDialog.tsx web/src/features/imports/ruleSummary.ts web/src/features/imports/RulePromptDialog.tsx web/src/app/router-pages.ts web/src/app/routes.tsx web/src/features/imports/ImportsDataPage.tsx web/src/features/settings/SettingsPage.tsx locales/en.json
git commit -m "feat(web): import rules page — list, edit, reorder, delete, AI suggestions

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 15: Catalogues for the other ten languages, docs, regression plan, and the full gate

**Files:**
- Modify: `locales/{de,es,fr,it,nl,pl,pt,ru,uk,zh}.json` (the whole `imports.rules` subtree from Tasks 13–14)
- Modify: `docs/regression-test-plan.md` (new `## 5c. Imports — Rules` inserted before `## 6. Recurring transactions`, line 337)
- Modify: `CLAUDE.md` (route count + list, new env vars, `AI_ENABLED`, `internal/infra/ai`)
- Modify: `README.md` only if it lists `ECONUMO_*` variables (`grep -n ECONUMO_IMPORT_ README.md`; if present, add the two new ones in the same style)

**Interfaces:** none — documentation and translation only.

- [ ] **Step 1: Translate the `imports.rules` subtree**

For each of the ten non-English catalogues, add an `imports.rules` object with exactly the key set of `locales/en.json` (`field`, `type`, `action`, `prompt.*`, `editor.*`, `page.*`, `suggest.*`) and the same `{var}` placeholders per key. Write real translations in each language (not English copies); keep the pipe-free single-form convention used in `en` for these strings. The `errors.import.*` codes from Task 8 are already present in every catalogue.

- [ ] **Step 2: Run the catalogue guards**

Run: `go test ./internal/test/i18ntest/`
Expected: PASS — key parity, placeholder parity, and frontend `t()`-call coverage all green. A failure names the key and language; fix and re-run.

- [ ] **Step 3: Regression-plan section**

Insert before `## 6. Recurring transactions` in `docs/regression-test-plan.md`:

```markdown
## 5c. Imports — Rules

Preconditions: at least one import source with a completed run (§5a or §5b) whose transactions are still unedited.

- [ ] Edit an imported transaction and change only its category: after "Update"
      a "Create an import rule" prompt opens with the payee prefilled as a
      trimmed match value (store number, city, state and processor prefix
      dropped) and a live "Matches N transactions in this import" count that
      updates as the value is edited. 📱
- [ ] Edit the same transaction again changing only notes/amount/date: no prompt.
      Re-open it and pick the category the import already applied (or that a
      previous rule set): no prompt.
- [ ] Prompt → "Create rule" → "Apply": the matching unedited transactions in
      that import take the category; "N skipped (you've edited these)" names
      the edited ones and the "Also update the N transactions you edited"
      checkbox is off by default. Ticking it rewrites them too.
- [ ] After applying to the run, the "Also apply to all imports from <source>?"
      step shows its own count; "Apply to all imports" updates the older runs,
      "Done" leaves them alone. "Not now" on the first step creates nothing.
- [ ] A transaction that a rule classified (Import rules page shows the rule):
      changing its category offers "Update rule" (match shown read-only, no
      editor) rather than a second rule; the rule's targets change.
- [ ] Settings → Import & export → Import rules (also under Settings → Data):
      rules list in priority order, skip rules carry a red "Skip" badge and no
      targets; "Add rule" opens the editor with a live "Matches N imported
      transactions" count; Save/Edit/Delete round-trip; drag (or focus the
      grip, Space, arrow, Space) reorders and the order survives reload. 📱
- [ ] A skip rule with prefix "PAYMENT THANK YOU" on description: the next
      sync/ingest of a matching row lands as `skipped` in the run summary and
      creates no transaction; a classify rule on payee sets category/payee/
      tag/labels on newly imported rows only where the row had none.
- [ ] `ECONUMO_AI_DSN` unset: no "Suggest rules" button; `suggest-rules` returns
      400 `import.ai_disabled`. Set to a working OpenAI-compatible endpoint:
      the button proposes rules with a reason and a live count each; Accept
      creates the rule at the bottom of the list, Edit opens the editor
      prefilled, Discard removes the row; a 4th click inside the window gets
      429 with the standard envelope.
- [ ] Ingest-scoped PATs get 401 on every rule endpoint; a read-only
      (trial-ended) user gets 402 on `create-rule`/`update-rule`/`delete-rule`/
      `apply-rule`/`suggest-rules` and 200 on `get-rule-list` and `preview-rule`
      (preview is a POST read — confirm it is on the 402 allowlist ONLY if the
      implementer added it; otherwise expect 402 there too and correct this item).
```

Verify the last item against `internal/web/middleware` (grep the read-only allowlist for `preview-rule`) and make the sentence state the actual behavior — the plan's Task 10 registered `preview-rule` as a plain POST, so it returns 402 for read-only users; if that is what the code does, delete the parenthetical and write "and 402 on `preview-rule` as well".

- [ ] **Step 4: CLAUDE.md**

- In the `imports` paragraph (the "`api/` with 21 routes under `/api/v1/import/`" sentence, line ~198): change 21 → 28 and append `get-rule-list`, `create-rule`, `update-rule`, `delete-rule`, `preview-rule`, `apply-rule`, `suggest-rules` to the route list.
- In the same paragraph, after the provider-subpackage sentence, add: "Rules (stage 4) live in the root package (`rules.go` matcher, `rule_*.go` use cases) and consult an optional `Completer` (`internal/infra/ai`, an OpenAI-compatible chat client) for `suggest-rules` — `internal/imports` never imports `internal/infra/ai`; the concrete client is injected in `internal/server`."
- Configuration section, after the `ECONUMO_IMPORT_ALLOW_PRIVATE_HOSTS` bullet, add:
  - `ECONUMO_AI_DSN` — `openai://<api-key>@<host>[:port][/prefix]?model=<model>` enables AI rule suggestions (`suggest-rules`) against any OpenAI-compatible chat-completions endpoint; the key is optional for local servers (`openai://localhost:11434?model=llama3.1`). Unset (default) = disabled: the endpoint returns 400 `import.ai_disabled` and the SPA hides the action. A bad scheme/missing host/missing `model` fails at boot.
  - `ECONUMO_RATE_LIMIT_SUGGEST_RULES` — `import/suggest-rules` calls per user per window (default `3`; every call counts — each is a paid completion) — add to the rate-limit bullet list.
- "Web UI config" bullet: add `AI_ENABLED` (bool, always present, `true` iff `ECONUMO_AI_DSN` is set) next to `IMPORT_MATCHER`.
- Architecture tree: under `infra/`, add `│   ├── ai/ .................. OpenAI-compatible chat-completions client (rule suggestions); imports nothing internal`.
- "Notable behaviours": add a **Transaction import (rules)** bullet: one `import_rules` row per rule, per user, optionally scoped to a source; `classify` rules only fill empty fields on newly imported rows (first match per field in priority order, labels unioned up to 10), `skip` rules drop the row after currency conversion and before matching and beat every classify rule; the ledger row snapshots what was applied (`applied*` on `get-transaction-import-list`) and the SPA diffs a later edit against it to offer a rule; `apply-rule` backfills existing imports by run/source/all, skipping edited rows unless `includeEdited`.

- [ ] **Step 5: `.env.example` and README cross-check**

`.env.example` already gained `ECONUMO_AI_DSN` and `ECONUMO_RATE_LIMIT_SUGGEST_RULES` in Task 11 — confirm with `grep -n "ECONUMO_AI_DSN\|SUGGEST_RULES" .env.example`. If `README.md` documents the `ECONUMO_IMPORT_*` variables, add the two new ones beside them in the same format; otherwise leave README untouched.

- [ ] **Step 6: The full gate**

Run, from the repo root with the Go toolchain on PATH:

```bash
make go-test
pnpm --dir web exec tsc -b
pnpm --dir web lint
pnpm --dir web test
```

Expected: `make go-test` green with coverage ≥ the gate (the OpenAPI-freshness check passes because Task 10 committed the regenerated docs — if it fails, run `make swagger` and commit the result); `tsc` clean; lint shows only the pre-existing warnings; vitest green except the pre-existing `web/src/api/transaction.test.ts` Blob failure (known, not ours). If the `enginecompare` PostgreSQL is available (`DATABASE_TEST_PGSQL_URL`), also run `make test-repo-pgsql` — the new sqlc queries have pgsql variants that only that tier exercises.

- [ ] **Step 7: Commit**

```bash
git add locales/ docs/regression-test-plan.md CLAUDE.md README.md .env.example
git commit -m "docs: import rules — translations, regression plan, config reference

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Deferred (out of scope for this plan — follow-ups)

- **Server-side `suggestMatchValue`.** The trimmed match-value prefill is client-only; the server never needs it (it matches whatever value it is given), so no Go mirror is written.
- **Sync-on-open / run-row actions / Part 3 deletion endpoints** (`delete-run`, `delete-link`), **passphrase-change UI**, CSV run id, `import:purge-events` — all carried over from the stage-3 deferrals, untouched here.
- **MCP surface for rules.** `internal/imports` still has no `mcp/` package; adding rules there means adding the whole import feature to MCP, a separate decision.
- **apiparity success goldens for `suggest-rules`.** The catalogue only records the `ai_disabled` 400 (the real handler needs a live completer); a `seams.Completer` stub for the parity suite would let it record a 200 — worth doing when the seams package grows another fake.
- **Rule scoping UI.** `ImportRuleSpecDto.sourceId` is always `''` from the SPA (global rules); the server accepts a source-scoped rule, and the editor can grow a source picker once a user has more than one source with conflicting merchants.
