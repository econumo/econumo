# Profile Currency by ID Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Store the profile default currency as a currency ID in the `users_options` row (wire stays code-shaped), make the delete census and hide guard exact by id, and refuse archiving a profile-default currency — per `docs/superpowers/specs/2026-08-01-profile-currency-id-design.md`.

**Architecture:** Data migration `20260801100000` rewrites option values code→id (own-first, skip already-ids, delete unresolvable). The user module maps id→code at every options-rendering edge (`GetOptionList`, `currentUser`, `toCurrentUserWithEmail`) via a `CodeByID` lookup; profile writes persist the resolved id; registration seeds the resolved default id. `CountCurrencyUsage` compares the id; a new `CountDefaultCurrencyUsage` powers the archive guard; `ProfileCurrency` becomes `CurrencyID`.

**Tech Stack:** Go + sqlc v1.30.0 (regen command as in prior plans), dbtest/fixture, apiparity.

## Global Constraints

- Wire FROZEN: options list keeps `{"name":"currency","value":"<CODE>"}`; `update-currency` (profile) keeps accepting a code; `currency_id` keeps its meaning. **All existing goldens byte-identical**; the only golden change is the new archive-guard scenario calls (additive).
- New frozen string (fieldless): `"Currency is in use and cannot be archived"`, code `currency.cannot_archive` (errs registry + BOTH locale catalogues).
- Read fallbacks preserved exactly: absent/dangling option → code `model.DefaultCurrency` ("USD") + its global id.
- gofmt + conventional commits per task; sqlc regen with the pinned v1.30.0.

---

### Task 1: Migration `20260801100000` (data-only)

**Files:**
- Create: `internal/infra/storage/migrations/{sqlite,pgsql}/20260801100000.sql`
- Create: `internal/infra/storage/migrations/migration_20260801100000_test.go`

- [ ] **Step 1: Failing test** (package `migrations_test`, reuse `openFK`/`toRunList`). Seed users u1..u4 with options: u1 `currency='PTS'` + own custom PTS (id `p1`) AND a global PTS must NOT exist (own-first covered by giving u2 a global-code case); u2 `currency='EUR'` + global EUR (id `e1`); u3 `currency='ZZZ'` (resolves to nothing); u4 `currency='<e1>'` (already an id — untouched). After running all migrations assert: u1 value = `p1`; u2 value = `e1`; u3 row DELETED; u4 value still `e1`. (Insert option rows with raw SQL: `INSERT INTO users_options (user_id, name, value, created_at, updated_at) VALUES (...)` — check the actual column list in the schema first and adapt.)
- [ ] **Step 2: RED**, then write the migration (IDENTICAL statements both engines):

```sql
-- Profile default currency becomes a currency ID (was a code). Resolve each
-- stored code the way reads did: the owner's own custom first, then global.
-- Values already matching a currencies.id are left as-is (double-applied
-- pre-release DBs). Unresolvable codes lose the row: the absent-option USD
-- fallback is the same behavior the dangling code already produced.
UPDATE users_options SET value = COALESCE(
    (SELECT c.id FROM currencies c WHERE c.code = users_options.value AND c.user_id = users_options.user_id),
    (SELECT c.id FROM currencies c WHERE c.code = users_options.value AND c.user_id IS NULL)
)
WHERE name = 'currency'
  AND NOT EXISTS (SELECT 1 FROM currencies c2 WHERE c2.id = users_options.value);
DELETE FROM users_options WHERE name = 'currency' AND value IS NULL;
```

- [ ] **Step 3: GREEN; `go test ./internal/... -count=1` all green.** No other test changes: fixtures insert option rows AFTER migrations run, so existing test data stays code-shaped and the (still code-based, until Task 2) reads keep working — only the migration test exercises the rewrite.
- [ ] **Step 4: Commit** `feat(user): migrate the profile currency option from code to id`.

---

### Task 2: User module — store/read the id (wire unchanged)

**Files:**
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/user_read.sql` or `currencies.sql` (wherever `CurrencyIDByCode` lives — add `GetCurrencyCodeByID :one SELECT code FROM currencies WHERE id = ?`), regen
- Modify: `internal/user/ports.go` (`CurrencyLookup` gains `CodeByID(ctx, id string) (string, error)`), `internal/server/glue_user.go` (adapter over `currencyrepo.Lookup.GetByID`), the user READ repo (same method on its own querier)
- Modify: `internal/user/read.go` (`GetOptionList`, `currentUser`), `internal/user/usecase.go` (`toCurrentUserWithEmail`), `internal/user/profile.go` (`UpdateCurrency` persists the id), `internal/model/user.go` (`UpdateCurrency(value)` — semantic now id; adjust doc + `CurrencyCode()` helper users), registration/default seeding (grep the writer of the seeded `OptionCurrency` default and store the resolved default id via the service's `CurrencyLookup`)
- Tests: user service/read tests + harnesses; **apiparity run with NO golden changes** is the acceptance gate

**Key logic (shared helper in the user package):**

```go
// resolveCurrencyOption maps the STORED option value (a currency id; legacy
// absence handled) to the wire code + id, with the frozen USD fallback.
// lookupCode: CodeByID; lookupID: global-id-by-code (for the fallback).
```

Each rendering site: when building `OptionResult` for `OptionCurrency`, emit the mapped CODE (not the stored id); `currency_id` comes straight from the stored value (fallback path unchanged). `profile.go` keeps resolving the request code user-aware and now persists that id via `u.UpdateCurrency(id, now)`.

- [ ] Steps: failing round-trip test first (update profile to a custom code → stored option value IS the id, wire shows the code, `currency_id` matches) → implement → `go test ./internal/... -count=1` → `go test ./internal/test/apiparity/` with `git status internal/test/apiparity/testdata/` CLEAN → commit `feat(user): profile currency persists the resolved currency id`.

---

### Task 3: Census by id

**Files:**
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/currency_write.sql` (`CountCurrencyUsage` options term becomes `users_options.value = ?` with the id; pgsql uses `$1` everywhere — the id/code asymmetry disappears), regen
- Modify: `internal/currency/repo/manage.go` + `internal/currency/manage.go` (`CountCurrencyUsage(ctx, id)` — drop the code param), delete use case call site, fakes
- Tests: `manage_integration_test.go` usage-count test updates + the **Bob/Alice regression test**: Bob's profile option holds BOB's own `PTS` id; deleting ALICE's `PTS` custom must succeed.

- [ ] TDD as usual; commit `fix(currency): delete-protection census matches profile defaults by id`.

---

### Task 4: Archive guard + `ProfileCurrency` by id

**Files:**
- Modify: `internal/shared/errs/codes.go` (+`CodeCurrencyCannotArchive = "currency.cannot_archive"` + AllCodes), `locales/{en,ru}.json` (`errors.currency.cannot_archive`)
- Modify: sqlc (`CountDefaultCurrencyUsage :one SELECT COUNT(*) FROM users_options WHERE name = 'currency' AND value = ?`), `ManageModel` + repo
- Modify: `internal/currency/manage_lifecycle.go` (`setArchived(archived == true)` refuses when the count > 0 with the frozen message), `internal/currency/visibility.go` guard compares `rec.ID == profileID`
- Modify: `internal/currency/manage.go` (`ProfileCurrency` → `CurrencyID(ctx, userID) (string, error)`), `internal/server/glue_currency.go` (returns the stored option value; absent → the global id of `model.DefaultCurrency`, needing the currency lookup injected: `NewCurrencyProfileCurrency(users, currencyLookup)`), server.go wiring, fakes/harnesses
- Modify: `internal/test/apiparity/catalogue.go` — inside `currency_write_read`, before the existing `archive-currency` call insert: set profile default to PTS (`POST /api/v1/user/update-currency` `{"currency":"PTS"}`), `err:archive-default` (→ 400 with the frozen message), revert profile (`{"currency":"USD"}`); regenerate + inspect goldens (additive calls only)
- Tests: service-level guard tests (own default blocks; after switching away succeeds), glue test update, i18n guards

- [ ] TDD as usual; `make swagger` unaffected (no annotation changes; grep to confirm). Commit `feat(currency): refuse archiving a profile-default currency; profile guard by id`.

---

### Task 5: Full verification

- [ ] `make test` (smoke + engines + web — web must be untouched: zero SPA changes in this plan) + `git status` clean except intended files.
- [ ] Boot the demo DB build; confirm John's option row now stores the USD id while `get-user-data` still shows `"currency":"USD"`, and archiving PTS while it is set as his default is refused (then revert any profile change, revoke the session).
- [ ] Commit anything outstanding.
