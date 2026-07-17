# User Language Column (write-only) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist each user's last selected UI language in `users.language` (default `'en'`, write-only) via a new `update-language` endpoint plus login capture, and clamp the SPA's initial language detection to supported languages.

**Architecture:** A dedicated `UPDATE` query (NOT part of `UpsertUser`, so no other write path can clobber the column) behind a new `Repository.UpdateLanguage` port; endpoint follows the `update-report-period` family exactly; login writes `reqctx.Language(ctx)` best-effort. Frontend: `locale()` resolution clamps to `getLocaleOptions()`; `applyLocale` fires a fire-and-forget API call when authenticated.

**Tech Stack:** Go stdlib + sqlc (engine-adapter pattern), existing i18n runtime, React SPA (axios client).

**Spec:** `docs/superpowers/specs/2026-07-16-user-language-column-design.md`

## Global Constraints

- Work in this worktree (`.claude/worktrees/sunny-squishing-dawn`), branch `feat/i18n`. Every subagent's FIRST command must `cd` here and verify `git branch --show-current` prints `feat/i18n`.
- Migration SQL (both engines, verbatim): `ALTER TABLE users ADD COLUMN language TEXT NOT NULL DEFAULT 'en';`
- The `language` column appears in NO SELECT, NO DTO, NO API response, and must NOT be added to `UpsertUser` (the whole-row upsert would clobber it with an empty value on every profile mutation).
- Both write paths store only members of `i18n.Supported` (endpoint: validated; login: middleware-resolved).
- Frozen contract: existing goldens stay byte-identical; ONLY the new `update-language` scenario adds golden content.
- New error code `user.language_invalid` registered in `errs.AllCodes` with `errors.user.language_invalid` entries in BOTH `locales/en.json` and `locales/ru.json` (i18ntest guard enforces).
- sqlc query files: ASCII-ONLY comments (an em dash in a `.sql` comment mangles sqlc v1.30 sqlite codegen). Regenerate with `sqlc generate` (config `internal/infra/storage/sqlc/sqlc.yaml`).
- Raw SQL in tests must use `db.Rebind(query)` for `?`→`$N` portability.
- Backend done-gate per task: `make go-test`. Frontend done-gate: `cd web && pnpm test && pnpm lint && pnpm exec tsc -b` (known allowed failure: pre-existing `ImportCsvDialog.test.tsx` happy-path timeout, identical on main).
- Comments sparse, why-not-what. Commit after every task.

---

### Task 1: Schema migration + repository method

**Files:**
- Create: `internal/infra/storage/migrations/sqlite/20260716000000.sql`
- Create: `internal/infra/storage/migrations/pgsql/20260716000000.sql`
- Modify: `internal/infra/storage/sqlc/query/sqlite/users.sql`, `internal/infra/storage/sqlc/query/pgsql/users.sql` (append one query)
- Generated: `internal/infra/storage/sqlc/gen/{sqlite,pgsql}/…` (via `sqlc generate` — never hand-edit)
- Modify: `internal/user/repository.go` (interface), `internal/user/repo/` (implementation — find the file holding the user queries' repo methods and its `querier` interface; add the method there and the passthrough/conversion in `sqlite.go` / `pgsql.go` if the adapters enumerate methods)
- Test: the user repo's existing `_test.go` file in `internal/user/repo/`

**Interfaces:**
- Consumes: nothing new.
- Produces (Task 2 depends on): `Repository.UpdateLanguage(ctx context.Context, id vo.Id, language string) error` on the `user.Repository` interface, implemented by the user repo. Missing user id: the UPDATE simply affects 0 rows and returns nil (no NotFound mapping needed — both callers pass an id they just loaded).

- [ ] **Step 1: Write both migration files**

`internal/infra/storage/migrations/sqlite/20260716000000.sql` and `internal/infra/storage/migrations/pgsql/20260716000000.sql`, identical content:

```sql
ALTER TABLE users ADD COLUMN language TEXT NOT NULL DEFAULT 'en';
```

(Check how neighbouring migration files in each dir are formatted — e.g. a trailing newline or a statement-marker comment — and match exactly.)

- [ ] **Step 2: Write the failing repo test**

In the user repo test file (find it: `ls internal/user/repo/*_test.go`; add to the file that already tests user-row methods, using its existing fixture/dbtest setup pattern):

```go
func TestUpdateLanguage(t *testing.T) {
	db := dbtest.New(t) // or the file's existing setup helper — mirror it
	repo := /* construct the repo exactly as sibling tests do */
	u := /* insert a user via the file's existing fixture/Save pattern */

	if err := repo.UpdateLanguage(context.Background(), u.ID, "ru"); err != nil {
		t.Fatalf("UpdateLanguage: %v", err)
	}
	var got string
	if err := db.Get(&got, db.Rebind("SELECT language FROM users WHERE id = ?"), u.ID.String()); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got != "ru" {
		t.Fatalf("language = %q, want ru", got)
	}
}

func TestUpdateLanguageDefaultsToEnglish(t *testing.T) {
	// a freshly inserted user gets the column default
	db := dbtest.New(t)
	u := /* insert via existing pattern */
	var got string
	if err := db.Get(&got, db.Rebind("SELECT language FROM users WHERE id = ?"), u.ID.String()); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got != "en" {
		t.Fatalf("default language = %q, want en", got)
	}
}
```

Adapt the construction lines to the file's real helpers (the assertions are the contract). Run: `go test ./internal/user/repo/ -run TestUpdateLanguage -v`
Expected: FAIL — `UpdateLanguage` undefined (and the default test fails to compile with it).

- [ ] **Step 3: Add the sqlc query to BOTH dialects**

Append to `internal/infra/storage/sqlc/query/sqlite/users.sql`:

```sql
-- name: UpdateUserLanguage :exec
UPDATE users SET language = ? WHERE id = ?;
```

Append to `internal/infra/storage/sqlc/query/pgsql/users.sql`:

```sql
-- name: UpdateUserLanguage :exec
UPDATE users SET language = $1 WHERE id = $2;
```

(ASCII-only; no prose comments needed.) Then run `sqlc generate` from the repo root (or wherever `sqlc.yaml` expects — check how the Makefile invokes it if unsure) and confirm `gen/sqlite` and `gen/pgsql` gained `UpdateUserLanguage`.

- [ ] **Step 4: Implement the repo method + interface**

`internal/user/repository.go` — add to the `Repository` interface after `Save`:

```go
	// UpdateLanguage persists the user's last selected UI language. Write-only:
	// nothing reads the column yet (future background email rendering). Kept out
	// of Save/UpsertUser so profile mutations cannot clobber it.
	UpdateLanguage(ctx context.Context, id vo.Id, language string) error
```

In `internal/user/repo/`, add the method to the repo struct following the engine-adapter pattern: extend the package's `querier` interface with `UpdateUserLanguage` in the canonical (sqlite-generated) types, add the pgsql adapter conversion in `pgsql.go` (whole-struct shim, mirroring the sibling methods), and write the method once:

```go
func (r *Repo) UpdateLanguage(ctx context.Context, id vo.Id, language string) error {
	return r.q.UpdateUserLanguage(ctx, gensqlite.UpdateUserLanguageParams{
		Language: language,
		ID:       id.String(),
	})
}
```

(Adjust receiver/field names to the file's actual ones; sqlc's generated params struct field order/names come from the query — check `gen/sqlite`.)

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/user/repo/ -run 'TestUpdateLanguage' -v`
Expected: both PASS.

- [ ] **Step 6: Backend done-gate + commit**

Run: `make go-test` — PASS (migrations apply in dbtest, archtest clean, goldens untouched).

```bash
git add -A && git commit -m "feat(user): users.language column with write-only repository method"
```

---

### Task 2: update-language endpoint + login capture

**Files:**
- Modify: `internal/model/user_dto.go` (request/result DTOs), `internal/shared/errs/codes.go` (new code), `internal/user/usecase.go` (`newLanguage` validator), `internal/user/profile.go` (use case), `internal/user/api/routes.go` (route)
- Create: `internal/user/api/language.go` (handler)
- Modify: `internal/user/login.go` (capture), `locales/en.json`, `locales/ru.json` (`errors.user.language_invalid`)
- Modify: `internal/test/apiparity/` scenario catalogue (+ regenerated golden)
- Modify: `CLAUDE.md` (one sentence, see Step 8)
- Test: `internal/user/api/` endpoint test file (mirror siblings), login test in `internal/user/`

**Interfaces:**
- Consumes: `Repository.UpdateLanguage(ctx, id vo.Id, language string) error` (Task 1); `i18n.Supported` (`internal/infra/i18n`); `reqctx.Language(ctx)`.
- Produces: `POST /api/v1/user/update-language`, body `{"language":"ru"}`, response `{"user": <CurrentUser>}` in the OK envelope; `errs.CodeUserLanguageInvalid = "user.language_invalid"`.

- [ ] **Step 1: DTOs in `internal/model/user_dto.go`**

Mirror `UpdateReportPeriodRequest`'s NotBlank-only Validate style exactly (read it first — reuse its exact blank-check helper/message/code):

```go
// UpdateLanguageRequest is the update-language request body. Membership in the
// supported-language set is checked in the feature (tier-2), like ReportPeriod.
type UpdateLanguageRequest struct {
	Language string `json:"language"`
}

func (r UpdateLanguageRequest) Validate() error {
	// same NotBlank construction as UpdateReportPeriodRequest.Validate, field key "language"
}

// UpdateLanguageResult is the update-language response.
type UpdateLanguageResult struct {
	User *CurrentUser `json:"user"`
}
```

(If `CurrentUser` is named differently in the result family — check `UpdateReportPeriodResult` — use that exact type.)

- [ ] **Step 2: Error code + catalogue**

`internal/shared/errs/codes.go`: add `CodeUserLanguageInvalid = "user.language_invalid"` to the const block and `AllCodes`.

`locales/en.json` under `errors.user`: `"language_invalid": "Language is incorrect"`.
`locales/ru.json` under `errors.user`: `"language_invalid": "Некорректный язык"`.

Run: `go test ./internal/test/i18ntest/ -v` — PASS (two-way guard sees code + entries).

- [ ] **Step 3: Failing use-case test**

In the user feature's existing use-case/profile test file (mirror the UpdateReportPeriod tests' setup):

```go
func TestUpdateLanguage(t *testing.T) {
	// setup service + user via the file's existing pattern
	res, err := svc.UpdateLanguage(ctx, userID, model.UpdateLanguageRequest{Language: "ru"})
	if err != nil || res.User == nil {
		t.Fatalf("UpdateLanguage: res=%v err=%v", res, err)
	}
	// read back users.language via db.Rebind("SELECT language FROM users WHERE id = ?") -> "ru"
}

func TestUpdateLanguageRejectsUnsupported(t *testing.T) {
	_, err := svc.UpdateLanguage(ctx, userID, model.UpdateLanguageRequest{Language: "xx"})
	v, ok := errs.AsValidation(err)
	if !ok {
		t.Fatalf("want ValidationError, got %v", err)
	}
	if v.Fields[0].Code != errs.CodeUserLanguageInvalid {
		t.Fatalf("code = %q", v.Fields[0].Code)
	}
}
```

Run: FAIL (method undefined).

- [ ] **Step 4: Use case + validator**

`internal/user/usecase.go`, next to `newReportPeriod` (same shape):

```go
func newLanguage(v string) (string, error) {
	for _, lang := range i18n.Supported {
		if v == lang {
			return v, nil
		}
	}
	return "", errs.NewValidation("Language is incorrect",
		errs.FieldError{Key: "language", Message: "Language is incorrect", Code: errs.CodeUserLanguageInvalid})
}
```

(import `github.com/econumo/econumo/internal/infra/i18n` — the user feature already imports infra packages, e.g. the mailer.)

`internal/user/profile.go`, after `UpdateReportPeriod`:

```go
// UpdateLanguage persists the caller's UI language. Write-only: nothing reads
// it yet; it exists so future background emails can render in the user's
// language. Deliberately not via mutate/Save (a dedicated UPDATE keeps the
// column out of the whole-row upsert).
func (s *Service) UpdateLanguage(ctx context.Context, userID vo.Id, req model.UpdateLanguageRequest) (*model.UpdateLanguageResult, error) {
	lang, err := newLanguage(req.Language)
	if err != nil {
		return nil, err
	}
	if err := s.repo.UpdateLanguage(ctx, userID, lang); err != nil {
		return nil, err
	}
	u, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	cur, err := s.toCurrentUser(ctx, u)
	if err != nil {
		return nil, err
	}
	return &model.UpdateLanguageResult{User: cur}, nil
}
```

Run the Step 3 tests: PASS.

- [ ] **Step 5: Handler + route**

Create `internal/user/api/language.go` mirroring `reportperiod.go` verbatim in structure (same imports, same `var _ =` trick if present, swag `@` block copied and adapted: route `/api/v1/user/update-language`, request `model.UpdateLanguageRequest`, success `apidoc.JsonResponseOk{data=model.UpdateLanguageResult}`), body:

```go
func (h *Handlers) UpdateLanguage(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.UpdateLanguage)
}
```

`internal/user/api/routes.go`: add after update-report-period:

```go
mux.Handle("POST /api/v1/user/update-language", auth(h.UpdateLanguage))
```

Add an endpoint test mirroring the file's update-report-period (or nearest sibling) endpoint test: happy path 200 with `"user"` in data, and invalid body `{"language":"xx"}` → 400 whose body contains `"user.language_invalid"`.

Run: `go test ./internal/user/... -v 2>&1 | tail -20` — PASS. Note `make go-lint` regenerates OpenAPI docs; run `make swagger` (or `make go-test`, which checks docs freshness) and commit the regenerated docs.

- [ ] **Step 6: Login capture (failing test first)**

In the login test file (`internal/user/login_test.go` or wherever Login's tests live — find with `grep -rln "func TestLogin" internal/user/`), add:

```go
func TestLoginPersistsLanguage(t *testing.T) {
	// setup + successful login via the file's existing pattern, with:
	ctx := reqctx.WithLanguage(context.Background(), "ru")
	// ...svc.Login(ctx, ...) succeeds...
	// read back: SELECT language FROM users WHERE id = ? -> "ru"
}
```

Run: FAIL (column still 'en'). Then in `internal/user/login.go`, immediately before the final `return &model.LoginResult{...}`:

```go
	// Best-effort: the language preference must never block a login.
	_ = s.repo.UpdateLanguage(ctx, u.ID, reqctx.Language(ctx))
```

(import `github.com/econumo/econumo/internal/shared/reqctx`). Run: PASS. Also confirm the no-header case in the same test file (`context.Background()` → column `en` — `reqctx.Language` defaults to "en").

- [ ] **Step 7: apiparity scenario + golden**

Read `internal/test/apiparity/` to find how update-* scenarios are declared (the guard test fails the suite if the new route has no scenario — run `go test ./internal/test/apiparity/` first to see the guard's failure message naming the uncovered route). Add a scenario for `POST /api/v1/user/update-language` with body `{"language":"ru"}` following the closest update-* sibling (auth token, fixture user). Then:

```bash
UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ && git diff --stat internal/test/apiparity/testdata/golden/
```

Inspect: ONLY new content for the update-language scenario (new golden entries); every pre-existing golden line unchanged. Scenario-count guard passes (count grew).

- [ ] **Step 8: CLAUDE.md + done-gate + commit**

In CLAUDE.md's i18n section (frontend-runtime bullet), append one sentence: the selected language is also persisted server-side (`users.language`, default `en` — written by `update-language` and on login from `Accept-Language`; write-only, for future background email rendering).

Run: `make go-test` — PASS.

```bash
git add -A && git commit -m "feat(user): update-language endpoint and login capture for users.language"
```

---

### Task 3: SPA — clamped detection + persistence call

**Files:**
- Modify: `web/src/lib/config.ts` (`locale()`), `web/src/lib/config.test.ts`
- Modify: `web/src/api/user.ts` (client fn), `web/src/components/LanguageSelector.tsx` (`applyLocale`), `web/src/components/LanguageSelector.test.tsx`

**Interfaces:**
- Consumes: `POST /api/v1/user/update-language` (Task 2); `getToken()` from `@/lib/storage`.
- Produces: `locale()` now always returns a member of `getLocaleOptions()`; `updateLanguage(language: string): Promise<void>` in `@/api/user`.

- [ ] **Step 1: Failing detection tests**

Add to `web/src/lib/config.test.ts` (mirror the file's existing localStorage/navigator stubbing; vitest `vi.stubGlobal` or the file's own pattern):

```ts
it('ignores an unsupported stored locale', () => {
  localStorage.setItem('econumo.locale', 'de') // use the real storage key — check lib/storage.ts setItem prefixing
  expect(locale()).toBe('en')
})

it('detects the first supported language from navigator.languages', () => {
  vi.stubGlobal('navigator', { ...navigator, languages: ['de-DE', 'ru-RU'], language: 'de-DE' })
  expect(locale()).toBe('ru')
})

it('falls back to english when nothing is supported', () => {
  vi.stubGlobal('navigator', { ...navigator, languages: ['de-DE', 'fr-FR'], language: 'de-DE' })
  expect(locale()).toBe('en')
})
```

(Adapt the storage-key line to how `getItem('locale')` actually namespaces keys — read `web/src/lib/storage.ts` first. Clear storage in the file's existing beforeEach/afterEach pattern.) Run: `cd web && pnpm test config` — the new tests FAIL (current `locale()` returns `de`/raw tags).

- [ ] **Step 2: Implement clamped `locale()`**

In `web/src/lib/config.ts`:

```ts
export function locale(value?: string): string {
  const supported = new Set(getLocaleOptions().map((o) => o.value))
  if (value === undefined) {
    const stored = getItem('locale')
    if (typeof stored === 'string' && supported.has(stored)) {
      return stored
    }
    const candidates = navigator.languages?.length ? navigator.languages : [navigator.language]
    for (const tag of candidates) {
      const primary = (tag || '').toLowerCase().split('-')[0]
      if (supported.has(primary)) {
        return primary
      }
    }
    return 'en'
  }
  setItem('locale', value)
  return value
}
```

Run: `pnpm test config` — PASS (including the file's pre-existing locale tests; if one asserted the old unclamped behavior, update it to the clamped expectation — that's the spec change).

- [ ] **Step 3: Failing persistence test**

Add to `web/src/components/LanguageSelector.test.tsx` (mirror its existing render/userEvent pattern and however sibling tests mock the api module):

```tsx
it('persists the choice to the API when authenticated', async () => {
  vi.spyOn(storage, 'getToken').mockReturnValue('eco_ses_test')
  const spy = vi.spyOn(userApi, 'updateLanguage').mockResolvedValue()
  // render, open dropdown, click Русский (existing test's interaction pattern)
  expect(spy).toHaveBeenCalledWith('ru')
})

it('does not call the API when logged out', async () => {
  vi.spyOn(storage, 'getToken').mockReturnValue(null)
  const spy = vi.spyOn(userApi, 'updateLanguage').mockResolvedValue()
  // render + switch language
  expect(spy).not.toHaveBeenCalled()
})
```

Run: FAIL (`updateLanguage` doesn't exist).

- [ ] **Step 4: Implement client fn + applyLocale hook**

`web/src/api/user.ts` (match the file's style):

```ts
export async function updateLanguage(language: string): Promise<void> {
  await api.post(apiUrl('/api/v1/user/update-language'), { language })
}
```

`web/src/components/LanguageSelector.tsx` — extend `applyLocale`:

```ts
export function applyLocale(value: string): void {
  locale(value)
  void i18n.changeLanguage(value)
  document.documentElement.lang = value
  // Best-effort server-side persist for future background emails; login
  // capture self-corrects if this is offline/fails.
  if (getToken() !== null) {
    updateLanguage(value).catch(() => {})
  }
}
```

(imports: `getToken` from `@/lib/storage`, `updateLanguage` from `@/api/user`; keep the function's existing lines exactly — only add the persist block.)

Run: `pnpm test LanguageSelector config` — PASS.

- [ ] **Step 5: Done-gate + commit**

Run: `cd web && pnpm test && pnpm lint && pnpm exec tsc -b` (known ImportCsvDialog failure allowed) AND `go test ./internal/test/i18ntest/` (no new frontend keys, but cheap insurance).

```bash
git add -A && git commit -m "feat(web): clamp initial language detection and persist selection server-side"
```

---

## Task order

1 → 2 → 3 (strictly sequential: repo → endpoint → SPA client).
