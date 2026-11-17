# CSV Import Wizard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the one-screen CSV import dialog with a step-by-step wizard (File, Account, Date, Amount, Description, Extras, Preview) that a non-technical user can finish with a bank statement and zero accounts, with AI-assisted column guessing that never sees a cell value.

**Architecture:** All parsing, guessing and normalising moves into pure TypeScript modules under `web/src/features/csvImport/`; the wizard UI is built on them, and the upload sends normalised `YYYY-MM-DD[ HH:MM:SS]` dates and plain decimals through the unchanged `import-transaction-list` endpoint. One new backend read, `GET /api/v1/transaction/suggest-import-columns`, asks the AI to assign header names to roles from header names and browser-computed column types only.

**Tech Stack:** Go (stdlib HTTP, `internal/web/endpoint`), React 19 + TypeScript, TanStack Query, Tailwind/shadcn, vitest, i18next catalogues in `locales/`.

**Spec:** `docs/superpowers/specs/2026-11-16-csv-import-wizard-design.md`

## Global Constraints

- No developer words in any UI copy: "mapping", "field", "column mode", "CSV column", and no raw header name shown without example values.
- The AI receives only: header names in file order, per-column type (`date` | `number` | `text` | `empty`), whether a number column has negatives, the user's language. Never a cell value.
- What the preview shows is exactly what is uploaded: one code path (`prepareRows`) feeds both.
- Every new string exists in all 11 catalogues (`de en es fr it nl pl pt ru uk zh`); registers: de Sie, nl u, fr vous, pt European. Plurals are one pipe-joined value read by `pluralPick`.
- The `import-transaction-list` endpoint, its mapping and response stay unchanged.
- 10 MB file limit (`MAX_FILE_SIZE = 10485760`), upload chunk size 500 rows (`CHUNK_SIZE`).
- Preview "Something looks wrong?" goes to step 3 (Date), or step 2 (Account) for an Econumo export.
- Required answers before Import is enabled: account, date, amount.
- Comments only for the non-obvious why (CLAUDE.md rule); Swagger `// @…` blocks on the new handler.
- New metric keys are `app`-prefixed camelCase in `METRICS`.

## Review Focus

1. A file with a UTF-8 BOM, CRLF line endings, or `;` as the delimiter (European banks) parses with the right header; pinned in Task 5.
2. A dual-amount file where some rows have both in and out empty: the row is skipped with reason "No amount", not imported as 0; pinned in Task 6.
3. Trailing blank lines or a footer line with fewer cells ("Closing balance;1.234,56") are skipped with a reason, not crash or import garbage; pinned in Task 6.
4. Amounts like `1.234,56`, `-1,234.56`, `(45.00)`, `45.00-`, `€ 12,00`, `1 234,56` parse to the same plain decimal the server would store; pinned in Task 4.
5. A header with duplicate column names (two "Amount" columns) keeps them distinct in cards and upload; pinned in Task 5.

---

## File Structure

Backend:
- `internal/model/transaction_view.go`: add `SuggestImportColumnsRequest`, `ImportColumnFact`, `SuggestImportColumnsResult`.
- `internal/transaction/ports.go`: add `Completer`, `AttemptLimiter`, `AIFacts`, `AIFactsInfo`.
- `internal/transaction/suggest_columns.go` (new): `SuggestImportColumns`, prompt payload builder, reply validation, `RateScopeImportColumns`.
- `internal/transaction/suggest_columns_test.go` (new), `internal/transaction/aieval_test.go` (new, tag `aieval`).
- `internal/transaction/api/suggest_columns.go` (new) + `routes.go`.
- `prompts/import_columns.md` (new) + `prompts/embed.go` var `ImportColumns`.
- `internal/server/glue_transaction_ai.go` (new), `internal/server/server.go` wiring + rate scope.
- `internal/test/apiparity/catalogue_transaction.go` scenario + goldens; OpenAPI docs via `make swagger`.

Frontend (`web/src/features/csvImport/`, new):
- `parse.ts`: `readCsvFile`, delimiter-aware parsing, duplicate header disambiguation.
- `columns.ts`: `profileColumns`.
- `dates.ts`, `amounts.ts`: detection and cell parsing.
- `guess.ts`: synonyms, `guessAnswers`, `applyAiGuess`.
- `prepare.ts`: `WizardAnswers`, `prepareRows`, `summarize`, `buildUpload`.
- `upload.ts`: `runChunks` (per-chunk state, retry failed only), account creation.
- `ImportWizard.tsx`, `StepBar.tsx`, `ColumnCards.tsx`, `steps/{FileStep,AccountStep,DateStep,AmountStep,DescriptionStep,ExtrasStep,PreviewStep}.tsx`, `useColumnGuess.ts`.
- `web/src/api/transaction.ts`: `suggestImportColumns`.
- Delete `web/src/features/transactions/ImportCsvDialog.tsx` (+ test) and `importCsv.ts` (+ test) once unused; keep `ImportResultDialog.tsx`, retyped to the new result.

---

### Task 1: Backend `suggest-import-columns` endpoint

**Files:**
- Modify: `internal/model/transaction_view.go`, `internal/transaction/ports.go`, `internal/transaction/usecase.go` (setter), `internal/transaction/api/routes.go`, `internal/server/server.go`, `prompts/embed.go`, `CLAUDE.md` (AI allowlist line)
- Create: `internal/transaction/suggest_columns.go`, `internal/transaction/suggest_columns_test.go`, `internal/transaction/api/suggest_columns.go`, `prompts/import_columns.md`, `internal/server/glue_transaction_ai.go`
- Test: also `internal/test/apiparity/` scenario + regenerated goldens

**Interfaces:**
- Produces (wire): `GET /api/v1/transaction/suggest-import-columns?columns=<JSON>` where `columns` is a JSON array of `{"name": string, "type": "date"|"number"|"text"|"empty", "negatives": bool}` (max 60 entries, names trimmed, max 100 chars each, else validation 400). Response `data`: `{"roles": {"date"?: string, "amount"?: string, "amountIn"?: string, "amountOut"?: string, "description"?: string, "payee"?: string, "category"?: string, "tag"?: string, "labels"?: string, "account"?: string}}`; empty object when no AI.
- Produces (Go):
  - `model.ImportColumnFact{Name, Type string; Negatives bool}`, `model.SuggestImportColumnsRequest{Columns []ImportColumnFact}` with `Validate() error`, `model.SuggestImportColumnsResult{Roles map[string]string \`json:"roles"\`}`.
  - `transaction.Completer` (same method set as `imports.Completer`), `transaction.AttemptLimiter` (`Allow`, `Reserve`), `transaction.AIFacts{ AIFacts(ctx, userID vo.Id) (AIFactsInfo, error) }`, `AIFactsInfo{Language string; Readonly, AIOff bool}`.
  - `(*Service).SetColumnAdvisor(c Completer, facts AIFacts, limiter AttemptLimiter)`.
  - `(*Service).SuggestImportColumns(ctx, userID vo.Id, req model.SuggestImportColumnsRequest) (*model.SuggestImportColumnsResult, error)`.
  - `const RateScopeImportColumns = "import-columns"`, cap 20 in `server.go`'s limiter map, listed in `limiterIPExempt`.

- [ ] **Step 1: Write the failing tests** in `internal/transaction/suggest_columns_test.go` with a fake completer recording `(system, user, schema)`:
  - `TestSuggestImportColumnsSendsOnlyTheAllowlist`: unmarshal the recorded user payload; top-level keys exactly `{"language","columns"}`; each column's keys exactly `{"name","type","negatives"}`.
  - `TestSuggestImportColumnsKeepsOnlyKnownHeaders`: reply `{"date":"Buchungstag","amount":"Betrag","description":"Nope"}` for columns `Buchungstag, Betrag, Verwendungszweck` → roles `{"date":"Buchungstag","amount":"Betrag"}`.
  - `TestSuggestImportColumnsDropsUnknownRolesAndDuplicates`: reply assigning one header to two roles keeps the first role in the fixed order `date, amount, amountIn, amountOut, description, payee, category, tag, labels, account`; unknown role keys dropped.
  - `TestSuggestImportColumnsEmptyWhen`: subtests `no_completer`, `ai_off`, `readonly`, `completer_error`, `bad_json`, `rate_limited` → empty roles, nil error, completer not called for the first three and the limiter case.
  - `TestSuggestImportColumnsValidate`: 61 columns, empty name, 101-char name, type `"money"` → validation error.
- [ ] **Step 2: Run** `GOTOOLCHAIN=go1.27.1 /usr/local/go/bin/go test ./internal/transaction/ -run SuggestImportColumns` — expect FAIL (undefined).
- [ ] **Step 3: Implement** the model types, ports, setter, `suggest_columns.go` (timeout 8 s like `suggestIconTimeout`; log WARN `"import columns suggestion failed"` with `user_id` and `ai.LogAttrs(err)` only; schema name `import_columns`; payload `{"language": lang or "en", "columns": [...]}`), `prompts/import_columns.md` (explain roles, that `amountIn`/`amountOut` replace `amount`, answer only with header names given, omit a role when unsure, headers may be in any language), `prompts.ImportColumns`.
- [ ] **Step 4: Run the Step 2 command** — expect PASS.
- [ ] **Step 5: Wire the edge:** handler `SuggestImportColumns` in `internal/transaction/api/suggest_columns.go` (query-param read: hand-written like the other query-param reads; parse `columns` JSON, `Validate()`, envelope), route `GET /api/v1/transaction/suggest-import-columns` with `auth`; `glue_transaction_ai.go` adapting `userRepo` facts the same way `NewImportsOwnerFacts` does into `transaction.AIFactsInfo`; in `server.go` call `transactionSvc.SetColumnAdvisor(aiClient, NewTransactionAIFacts(userRepo, clk), authLimiter)` when `aiClient != nil` (add a `seams.TransactionCompleter` override mirroring `ImportsCompleter` for tests); rate scope 20 + IP exempt.
- [ ] **Step 6: API test** in `internal/transaction/api/` (existing harness): authenticated GET with two columns returns 200 and `{"roles":{}}` with no AI; malformed `columns` returns 400 envelope; missing token returns the frozen 401.
- [ ] **Step 7: Parity + docs:** add an apiparity scenario for the route, `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/`, inspect the diff; `make swagger`; add "CSV column header names and browser-computed column types" to the AI data allowlist bullet in `CLAUDE.md`, and the endpoint to the AI consumers list.
- [ ] **Step 8: Run** `GOTOOLCHAIN=go1.27.1 make go-lint` and `go test ./internal/transaction/... ./internal/test/apiparity/ ./internal/test/archtest/` — expect PASS.
- [ ] **Step 9: Commit** `feat: suggest-import-columns AI endpoint`.

### Task 2: AI evals for column suggestions

**Files:**
- Create: `internal/transaction/aieval_test.go` (`//go:build aieval`)

**Interfaces:**
- Consumes: `(*Service).SuggestImportColumns`, `internal/test/aieval` helpers and `aieval.PassThreshold`.

- [ ] **Step 1: Write cases** per language `en`, `de`, `ru`, at least 8 headers sets each, real bank styles, each with expected roles for date, amount (or amountIn+amountOut) and description. Examples to include verbatim:
  - de: `Buchungstag;Valutadatum;Buchungstext;Auftraggeber / Begünstigter;Verwendungszweck;Kontonummer;BLZ;Betrag (EUR)` → date `Buchungstag`, amount `Betrag (EUR)`, description `Verwendungszweck`, payee `Auftraggeber / Begünstigter`.
  - en: `Transaction Date,Post Date,Description,Category,Type,Amount,Memo` → date `Transaction Date`, amount `Amount`, description `Description`, category `Category`.
  - en dual: `Date,Description,Debit,Credit,Balance` → amountOut `Debit`, amountIn `Credit`.
  - ru: `Дата операции;Дата платежа;Номер карты;Статус;Сумма операции;Валюта операции;Категория;Описание` → date `Дата операции`, amount `Сумма операции`, category `Категория`, description `Описание`.
  Score a case as passed when every expected role matches and no required role (date, amount or the in/out pair) is assigned to a wrong header.
- [ ] **Step 2: Run** `ECONUMO_AI_DSN=… make ai-eval` if a DSN is available locally; otherwise rely on the CI job. Expect each language ≥ 0.90. Fix the prompt, never the threshold.
- [ ] **Step 3: Commit** `test: AI evals for import column suggestions`.

### Task 3: Dates

**Files:**
- Create: `web/src/features/csvImport/dates.ts`, `dates.test.ts`

**Interfaces:**
- Produces:
  - `type DateOrder = 'ymd' | 'dmy' | 'mdy'`
  - `interface ParsedDate { date: string /* YYYY-MM-DD */; time: string | null /* HH:MM:SS */ }`
  - `parseDateCell(value: string, order: DateOrder): ParsedDate | null` — separators `- / .`, optional `HH:MM[:SS]` after a space or `T`, ISO with offset (keep the local wall time as written), 8-digit `YYYYMMDD`, 10/13-digit unix seconds/millis (UTC), surrounding quotes/space trimmed; invalid calendar dates (`2026-02-30`) → null.
  - `detectDateOrder(values: string[], locale: string): { order: DateOrder; ambiguous: boolean; example: string | null }` — `ymd` when values start with a 4-digit year; else a first part > 12 in any value → `dmy`, a second part > 12 → `mdy`; when neither, `ambiguous: true`, `example` = first value, order from `localeDateOrder(locale)`.
  - `localeDateOrder(locale: string): 'dmy' | 'mdy'` — `mdy` for `en-US`, `en-PH` and `es-US`; else `dmy`. Pass `navigator.language`.

- [ ] **Step 1: Write failing tests:** `parseDateCell('03/04/2026','dmy')` → `{date:'2026-04-03', time:null}`; same with `'mdy'` → `2026-03-04`; `'2026-03-04 14:05:09'` → time `'14:05:09'`; `'2026-03-04T14:05'` → time `'14:05:00'`; `'20260304'` → `2026-03-04`; `'1772582400'` → a date; `'31/02/2026'` → null; `detectDateOrder(['03/04/2026','15/04/2026'],'en-US')` → `{order:'dmy', ambiguous:false}`; `detectDateOrder(['03/04/2026'],'en-US')` → `{order:'mdy', ambiguous:true, example:'03/04/2026'}`.
- [ ] **Step 2: Run** `pnpm vitest run src/features/csvImport/dates.test.ts` — FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** `feat(web): date detection for the import wizard`.

### Task 4: Amounts

**Files:**
- Create: `web/src/features/csvImport/amounts.ts`, `amounts.test.ts`

**Interfaces:**
- Produces:
  - `type DecimalSeparator = '.' | ','`
  - `parseAmountCell(value: string, sep: DecimalSeparator): string | null` — returns a plain decimal string (`-1234.56`, no exponent, no trailing zeros beyond what was written); strips currency symbols/codes and spaces (incl. NBSP and thin space), handles thousands separators, `(…)` and trailing `-` as negative, leading `+`; empty → null; non-numeric → null.
  - `detectDecimalSeparator(values: string[]): { sep: DecimalSeparator; ambiguous: boolean; example: string | null }` — a value with both `.` and `,` settles it by the last one; a single separator followed by exactly 3 digits is ambiguous alone; default `.`.
  - `signedAmount(input: { single?: string | null; inflow?: string | null; outflow?: string | null }, mode: 'single' | 'dual', flip: boolean, sep: DecimalSeparator): string | null` — single: parsed, negated when `flip`; dual: `+|in|` if in non-empty and non-zero, else `−|out|` if out non-empty, else null.

- [ ] **Step 1: Write failing tests:** `'1.234,56'`,`','` → `'1234.56'`; `'-1,234.56'`,`'.'` → `'-1234.56'`; `'(45.00)'` → `'-45.00'`; `'45.00-'` → `'-45.00'`; `'€ 12,00'`,`','` → `'12.00'`; `'1 234,56'`,`','` → `'1234.56'`; `'abc'` → null; `detectDecimalSeparator(['1.234,56'])` → `{sep:',', ambiguous:false}`; `detectDecimalSeparator(['1,234'])` → `ambiguous:true`; `signedAmount({inflow:'', outflow:'-12.00'},'dual',false,'.')` → `'-12.00'`; `signedAmount({inflow:'', outflow:''},'dual',false,'.')` → null; `signedAmount({single:'4.50'},'single',true,'.')` → `'-4.50'`.
- [ ] **Step 2–4: Run FAIL, implement, run PASS** (`pnpm vitest run src/features/csvImport/amounts.test.ts`).
- [ ] **Step 5: Commit** `feat(web): amount parsing for the import wizard`.

### Task 5: Parsing and column profiles

**Files:**
- Create: `web/src/features/csvImport/parse.ts`, `columns.ts`, `parse.test.ts`, `columns.test.ts`
- Modify: `web/src/lib/csv.ts` only if delimiter support belongs there (add an optional `delimiter` parameter to `parseCsv`/`parseCsvLine`, default `,`, keeping current behaviour for other callers)

**Interfaces:**
- Produces:
  - `interface ParsedFile { header: string[]; rows: string[][]; delimiter: ',' | ';' | '\t' | '|'; rawLines: string[] }` — `header` has duplicates disambiguated as `Amount`, `Amount (2)`; `rawLines[i]` is the original text of `rows[i]`.
  - `readCsvFile(file: File): Promise<ParsedFile | { error: 'too_big' | 'empty' | 'unreadable' }>`
  - `parseCsvText(text: string): ParsedFile` (strips BOM, CRLF, detects delimiter by the most consistent count over the first 20 lines).
  - `type ColumnType = 'date' | 'number' | 'text' | 'empty'`
  - `interface ColumnProfile { index: number; name: string; type: ColumnType; negatives: boolean; samples: string[] }` (`samples`: first 3 distinct non-empty values, each truncated at 25 chars with `…`).
  - `profileColumns(file: ParsedFile): ColumnProfile[]` — type is the majority (≥ 80 %) of the first 500 non-empty values classified by `parseDateCell` (any order) and `parseAmountCell`; otherwise `text`; no values → `empty`.

- [ ] **Step 1: Write failing tests:** `parseCsvText` on `"﻿Datum;Betrag\r\n01.02.2026;-4,50\r\n"` gives header `['Datum','Betrag']`, delimiter `;`, rows `[['01.02.2026','-4,50']]`; quoted delimiters inside cells stay one cell; duplicate headers `Amount,Amount` → `['Amount','Amount (2)']`; `profileColumns` marks `Datum` `date`, `Betrag` `number` with `negatives: true`, an all-blank column `empty`, samples capped at 3.
- [ ] **Step 2: Run** `cd web && pnpm vitest run src/features/csvImport/parse.test.ts src/features/csvImport/columns.test.ts` — expect FAIL.
- [ ] **Step 3: Implement** (uses `parseDateCell` and `parseAmountCell` from Tasks 3–4 for typing).
- [ ] **Step 4: Run Step 2 command** — expect PASS.
- [ ] **Step 5: Commit** `feat(web): CSV parsing and column profiles for the import wizard`.

### Task 6: Answers, guessing and row preparation

**Files:**
- Create: `web/src/features/csvImport/guess.ts`, `prepare.ts`, `guess.test.ts`, `prepare.test.ts`

**Interfaces:**
- Consumes: Tasks 3–5 (`parseDateCell`, `detectDateOrder`, `parseAmountCell`, `detectDecimalSeparator`, `signedAmount`, `ParsedFile`, `ColumnProfile`).
- Produces:
  - ```ts
    type Role = 'date' | 'amount' | 'amountIn' | 'amountOut' | 'description' | 'payee' | 'category' | 'tag' | 'labels' | 'account'
    type AccountChoice =
      | { kind: 'new'; name: string; currencyId: string }
      | { kind: 'existing'; accountId: string }
      | { kind: 'column'; column: number; currencies: Record<string, string> /* lowercased new name -> currencyId */ }
    interface WizardAnswers {
      account: AccountChoice | null
      date: { column: number | null; order: DateOrder }
      amount: { mode: 'single' | 'dual'; column: number | null; inColumn: number | null; outColumn: number | null; flip: boolean; sep: DecimalSeparator }
      description: number | null
      extras: { payee: number | null; category: number | null; tag: number | null; labels: number | null; labelsSeparator: string }
    }
    type RowFix = { date?: string; amount?: string } | 'skip'
    ```
  - `isEconumoExport(file: ParsedFile): boolean` (same exact-header rule as today's `EXPORT_HEADER` in `web/src/features/transactions/importCsv.ts`).
  - `guessAnswers(file: ParsedFile, profiles: ColumnProfile[], ctx: { hasWritableAccounts: boolean; userCurrencyId: string; locale: string; defaultAccountName: string }): WizardAnswers` — Econumo export → `account: {kind:'column', column: account_name, currencies: from account_currency}` and every role mapped; else synonyms (`SYNONYMS: Record<Role, string[]>`, lowercase, all 11 languages, matched exact first then substring, a column claimed once, and only if its profile type fits the role: date→`date`, amounts→`number`, others→`text`); account defaults to `{kind:'new', name: defaultAccountName, currencyId: userCurrencyId}` without writable accounts, else `null` (user picks).
  - `applyAiGuess(answers: WizardAnswers, roles: Partial<Record<Role, string>>, header: string[], touched: ReadonlySet<StepId>): WizardAnswers` — only changes steps not in `touched`; ignores names not in `header`; `amountIn`+`amountOut` together switch to dual.
  - `type StepId = 'file' | 'account' | 'date' | 'amount' | 'description' | 'extras' | 'preview'`
  - ```ts
    interface PreparedRow {
      fileRow: number            // 1-based data row number in the file (header excluded), as the result screen shows
      raw: string
      status: 'ok' | 'skipped'
      reason: 'no_date' | 'no_amount' | 'no_account' | 'user_skipped' | null
      date: string | null; time: string | null; amount: string | null
      account: string | null; description: string; payee: string; category: string; tag: string; labels: string
    }
    ```
  - `prepareRows(file: ParsedFile, answers: WizardAnswers, fixes: ReadonlyMap<number, RowFix>): PreparedRow[]` — a fix applies only to a row whose own parse fails for that value (fixes for rows that became readable are ignored); a fully blank row is dropped entirely (not counted).
  - `summarize(rows: PreparedRow[]): { total: number; skipped: number; byReason: Record<string, number>; firstDate: string | null; lastDate: string | null; moneyIn: string; moneyOut: string }` (decimal-safe sums: use the app's existing decimal helper if one exists in `web/src/lib`, else integer minor-unit arithmetic on the parsed strings).
  - `buildUpload(rows: PreparedRow[], answers: WizardAnswers): { header: string[]; rows: string[][]; fileRows: number[]; mapping: Record<string, string | null>; fields: Record<string, string> }` — only `ok` rows; header `['date','amount','account','description','payee','category','tag','labels']` limited to what is mapped; `date` cell is `date` or `date + ' ' + time`; `account` column present only for `kind:'column'`; `fields.accountId` for `existing` (and `new`, after creation, filled in by Task 11); `fields.labelsSeparator` always sent.

- [ ] **Step 1: Write failing tests:**
  - `guess.test.ts`: German header `Buchungstag;Betrag;Verwendungszweck` with typed profiles → date 0, amount single 1, description 2; a `Date` column of text is not picked for date; `Date,Description,Debit,Credit` → dual with out=Debit, in=Credit; Econumo export header → account kind `column` with `currencies` from `account_currency`; `applyAiGuess` leaves a touched `date` step alone and fills an untouched `description`.
  - `prepare.test.ts`: a row with unreadable date → `status:'skipped', reason:'no_date'`; same row with fix `{date:'2026-03-04'}` → ok; a fix on a readable row is ignored; `'skip'` → `reason:'user_skipped'`; a footer `Closing balance;1.234,56` (no date) → skipped `no_date`; trailing empty line → not in output; dual row with both empty → `no_amount`; `buildUpload` keeps `fileRows` aligned with uploaded rows and writes `'2026-03-04 14:05:09'` when a time exists; `summarize` totals `moneyIn`/`moneyOut` exactly for `['0.10','0.20']` → `'0.30'`.
- [ ] **Step 2–4: Run FAIL, implement, run PASS** (`pnpm vitest run src/features/csvImport/guess.test.ts src/features/csvImport/prepare.test.ts`).
- [ ] **Step 5: Commit** `feat(web): guessing and row preparation for the import wizard`.

### Task 7: Upload with retry of failed chunks

**Files:**
- Create: `web/src/features/csvImport/upload.ts`, `upload.test.ts`

**Interfaces:**
- Consumes: `buildUpload` output; `importTransactionList` (`web/src/api/transaction.ts`, unchanged); `createAccount(form: CreateAccountForm)` (`web/src/api/account.ts`).
- Produces:
  - `interface ImportResult { imported: number; failed: number; errors: { message: string; rows: number[] }[]; failedChunks: number[] }` — replaces `AggregatedImportResult`; `rows` are file row numbers via `fileRows`; browser-skipped rows are added as errors with plain messages (i18n keys resolved by the caller) and counted in `failed`.
  - `runChunks(upload, post: ImportPoster, opts: { chunks?: number[]; onProgress?: (done: number, total: number) => void }): Promise<ImportResult>` — chunks of `CHUNK_SIZE` (500); with `opts.chunks` sends only those indices.
  - `ensureAccounts(answers: WizardAnswers, created: Map<string, string>, existingNames: Set<string>): Promise<{ accountId?: string }>` — `new`: creates once (UUIDv7 id, balance `'0'`, the default account icon used by `AccountsStep`, `folderId: null`) and stores in `created` under key `'new'`; `column`: creates each new name in `currencies` not in `existingNames` (lowercased) and not already in `created`.

- [ ] **Step 1: Write failing tests:** 1,200 rows → 3 posts; post 2 throws → `failedChunks: [1]`, `imported` counts chunks 0 and 2, error rows are the file rows of chunk 1; `runChunks(..., {chunks:[1]})` posts once; error rows from the server (1-based within the chunk) map through `fileRows`; `ensureAccounts` called twice with the same `created` map calls `createAccount` once.
- [ ] **Step 2–4: Run FAIL, implement, run PASS** (`pnpm vitest run src/features/csvImport/upload.test.ts`).
- [ ] **Step 5: Commit** `feat(web): chunked import upload with retry of failed chunks`.

### Task 8: Wizard shell, step bar and File step

**Files:**
- Create: `web/src/features/csvImport/ImportWizard.tsx`, `StepBar.tsx`, `steps/FileStep.tsx`, `ImportWizard.test.tsx`
- Modify: `web/src/features/onboarding/ImportStep.tsx`, `web/src/features/imports/ImportsDataPage.tsx`, `web/src/features/onboarding/OnboardingPage.test.tsx` (mock name), `web/src/lib/metrics.ts`, all 11 `locales/*.json`

**Interfaces:**
- Consumes: Tasks 5, 6.
- Produces:
  - `ImportWizard({ open, onClose, onComplete, initialFile }: { open: boolean; onClose(): void; onComplete(result: ImportResult): void; initialFile?: File })` — same contract as `ImportCsvDialog`.
  - `StepBar({ steps, current, onSelect }: { steps: { id: StepId; label: string; warning: boolean; hidden?: boolean }[]; current: StepId; onSelect(id: StepId): void })` — full labels when the dialog is wide, numbered tappable dots + current label on a phone (use the app's existing breakpoint hook used by `ResponsiveDialog`).
  - Wizard state: `file`, `profiles`, `answers`, `touched: Set<StepId>`, `fixes`, `step`; `setAnswers(step, patch)` adds `step` to `touched`.
  - Step bar disabled until a file is loaded; then any step clickable; `extras` hidden when no unused columns; warnings: account/date/amount unanswered, or any `prepareRows` skips attributable to that step.
  - Econumo export: after File, go straight to `preview`.
  - Metrics: keep `TRANSACTION_IMPORT_OPEN/CLOSE/IMPORT/FAIL`; `TRANSACTION_IMPORT_CLOSE` gets `{ step: StepId }`.
- i18n namespace: `transactions.import_wizard.*` (step names "Your file", "Account", "Date", "Amount", "Description", "Extra details", "Check"; Back, Next; file errors "This file is larger than 10 MB", "This file is empty", "This file couldn't be read as a spreadsheet (CSV)").

- [ ] **Step 1: Write failing tests** (`ImportWizard.test.tsx`, msw/fake api as the existing `ImportCsvDialog.test.tsx` does): opening with `initialFile` of a bank CSV lands on step `account`; clicking the `Amount` step from `Account` works; an Econumo export file lands on `Check`; closing on step `date` fires `appTransactionImportClose` with `{step:'date'}`; a 10 MB+ file shows the size message and stays on File.
- [ ] **Step 2: Run** `pnpm vitest run src/features/csvImport/ImportWizard.test.tsx` — FAIL.
- [ ] **Step 3: Implement** the shell, StepBar and FileStep (reuse the drop-zone look from `ImportStep.tsx`). Steps 2–7 get minimal bodies here (a plain column picker each, enough for the tests above); Tasks 9–11 replace them.
- [ ] **Step 4: Swap** `ImportCsvDialog` for `ImportWizard` in `ImportStep.tsx` and `ImportsDataPage.tsx`; `ImportResultDialog` takes `ImportResult`.
- [ ] **Step 5: Run** `pnpm vitest run src/features/csvImport src/features/onboarding src/features/imports src/lib` — PASS.
- [ ] **Step 6: Commit** `feat(web): import wizard shell and step bar`.

### Task 9: Column cards and the Account, Date, Amount, Description steps

**Files:**
- Create: `web/src/features/csvImport/ColumnCards.tsx`, `steps/AccountStep.tsx`, `steps/DateStep.tsx`, `steps/AmountStep.tsx`, `steps/DescriptionStep.tsx`, `steps/steps.test.tsx`
- Modify: all 11 `locales/*.json`

**Interfaces:**
- Consumes: wizard state from Task 8, `profiles`, `detectDateOrder`, `detectDecimalSeparator`, `useAccounts`, `useCurrencies`, `CurrencyField`, `EntitySelect` (account picker).
- Produces: `ColumnCards({ profiles, value, onChange, allowNone, label }: { profiles: ColumnProfile[]; value: number | null; onChange(index: number | null): void; allowNone?: boolean; label: string })` — radio-group of cards, each the header name plus its `samples`; stacks on a phone.
- Copy (en, verbatim; translate the rest): Account step title "Which account is this statement from?", options "A new account", "An account I already have", "Each row says which account"; Date step "Which column has the date?", ambiguity question "Is {example} the {dmy} or {mdy}?" where `{dmy}`/`{mdy}` are the example formatted as long dates in the UI language; skipped note "{count} rows have no readable date and will be skipped" (plural); Amount step "How does the file show amounts?", "One column with + and −", "Separate columns for money in and money out", "Spending shows as: {example}" with a switch "Flip the signs"; Description step "Which column describes each transaction?", option "None".
- Writable accounts filter: same rule as today's dialog (owner, or shared with a role other than `guest`).
- "Each row says which account": list distinct names from the chosen column; an existing writable name shows "Existing account"; others get a `CurrencyField`.

- [ ] **Step 1: Write failing tests:** with no accounts the Account step pre-selects "A new account" with the user's currency; choosing a card on the Date step updates the answer and marks the step touched; ambiguous `03/04/2026` shows the question and switching it changes how the Preview's first card reads; dual amount hides "Flip the signs"; the per-row account list shows a currency picker only for new names.
- [ ] **Step 2–4: Run FAIL, implement, run PASS** (`pnpm vitest run src/features/csvImport/steps/steps.test.tsx`).
- [ ] **Step 5: Commit** `feat(web): import wizard account, date, amount and description steps`.

### Task 10: Extras step

**Files:**
- Create: `web/src/features/csvImport/steps/ExtrasStep.tsx`, `steps/ExtrasStep.test.tsx`
- Modify: all 11 `locales/*.json`

**Interfaces:**
- Consumes: wizard state; `useCategories`, `useLabels`, `useTags` for the "will be created" counts (reuse the counting idea of today's `countNewLabels`, matching names case-insensitively against the account owner's classifications, archived included).
- Produces: per unused column (sample values shown), a choice Category / Payee / Tag / Labels / Leave out (default Leave out unless guessed); a role can be given to one column only (choosing it elsewhere clears the previous). Labels show "Several labels in one cell are separated by…" with `;` `,` `|` Tab and a custom input. Notes "{count} new categories will be created" and the same for labels (plurals).

- [ ] **Step 1: Write failing tests:** columns already used by earlier steps are not listed; assigning Category to two columns keeps only the last; the new-category count excludes names that exist with different letter case.
- [ ] **Step 2–4: Run FAIL, implement, run PASS.**
- [ ] **Step 5: Commit** `feat(web): import wizard extras step`.

### Task 11: Preview, import and AI guess

**Files:**
- Create: `web/src/features/csvImport/steps/PreviewStep.tsx`, `useColumnGuess.ts`, `steps/PreviewStep.test.tsx`
- Modify: `web/src/api/transaction.ts` (`suggestImportColumns`), `ImportWizard.tsx`, `web/src/features/transactions/ImportResultDialog.tsx`, `web/src/lib/metrics.ts`, all 11 `locales/*.json`

**Interfaces:**
- Consumes: `prepareRows`, `summarize`, `buildUpload`, `runChunks`, `ensureAccounts`, `finishImportStep`, `scheduleAiRefresh`, `useOnline`.
- Produces:
  - `suggestImportColumns(columns: { name: string; type: ColumnType; negatives: boolean }[], signal?: AbortSignal): Promise<Partial<Record<Role, string>>>` — GET with `params: { columns: JSON.stringify(columns) }`; any error resolves `{}`.
  - `useColumnGuess(profiles: ColumnProfile[] | null, onGuess: (roles: Partial<Record<Role, string>>) => void)` — one call per loaded file, aborted on file change/close; skipped for an Econumo export.
  - PreviewStep: totals line ("{count} transactions, {first} to {last}. Money in {in}, money out {out}."; "{count} will be skipped" with reasons); a card for row `i` laid out like the app's transaction view (account, date [+ time], signed amount colored with the app's income/expense classes, description, payee/category/tag/labels when mapped); header "Transaction {n} of {total}"; Previous/Next buttons, ArrowLeft/ArrowRight keys while the dialog has focus, horizontal swipe on touch; "Next problem" shown when any row is skipped; the original line under the card ("From the file:"); each value on the card is a link to its step; "Something looks wrong?" → `date` (or `account` for an Econumo export); a skipped row's card states the reason and, for `no_date`/`no_amount`, shows a date input or amount input and "Skip this row"; primary "Import {count} transactions" (plural), disabled offline or while required answers are missing. For an Econumo export (and any `kind:'column'` account answer) the step also shows the account list from the Account step, with currency pickers for new names, above the card.
  - Import: `ensureAccounts` → `buildUpload` (+ `accountId`) → `runChunks` with progress → `scheduleAiRefresh` → when `imported > 0`, `finishImportStep(queryClient).catch(() => undefined)` → `invalidateQueries` → `onClose(); onComplete(result)`. The wizard keeps `created` accounts and `failedChunks` for its lifetime.
  - `ImportResultDialog`: when `failedChunks.length > 0`, a "Try the rest again" button that calls back into the wizard (`onRetry`) to `runChunks` with `chunks: failedChunks`.
  - Metrics: `TRANSACTION_IMPORT_AI_GUESS: 'appTransactionImportAiGuess'` (fired when a non-empty AI reply changes at least one untouched answer), `TRANSACTION_IMPORT_LOOKS_WRONG: 'appTransactionImportLooksWrong'`.

- [ ] **Step 1: Write failing tests:** the card for row 1 shows `−4.50` for `-4,50` with `,` separator; Next/ArrowRight moves to "Transaction 2 of N"; "Next problem" jumps to the first skipped row; typing a date on a `no_date` card raises the Import count by one; "Skip this row" keeps it out; Import with a `new` account calls create-account once then import with that `accountId`; a failed chunk shows "Try the rest again" and retrying posts only that chunk; a non-empty AI guess fills an untouched description step but not a touched date step; offline disables Import.
- [ ] **Step 2–4: Run FAIL, implement, run PASS** (`pnpm vitest run src/features/csvImport src/features/transactions/ImportResultDialog.test.tsx`).
- [ ] **Step 5: Commit** `feat(web): import wizard preview, import and AI column guess`.

### Task 12: Cleanup, docs and full frontend check

**Files:**
- Delete: `web/src/features/transactions/ImportCsvDialog.tsx`, `ImportCsvDialog.test.tsx`, `importCsv.ts`, `importCsv.test.ts` (move anything still used, e.g. the export header list, into `csvImport/guess.ts`)
- Modify: `locales/*.json` (remove `transactions.import_csv.*` keys no longer read), `web/src/lib/offline-coverage.test.ts` / `offline-notice-coverage.test.ts` if they name the old dialog, `docs/regression-test-plan.md`, `CLAUDE.md` (Frontend: one line pointing at `web/src/features/csvImport` and the spec), `web/src/features/onboarding/OnboardingImport.test.tsx`

- [ ] **Step 1: Delete and fix imports;** `grep -rn "import_csv\.\|ImportCsvDialog\|importCsv'" web/src locales` returns nothing that is unused.
- [ ] **Step 2: Regression plan:** rewrite the CSV import items (Import step items near line 405, Settings → Data import items, the "CSV import names a new category … in different letter cases" item) with verifiable outcomes for: bank statement with no accounts, Econumo export straight to Check, ambiguous date question, flip signs, fixing a problem row, Try the rest again, AI on vs off. Mark phone-relevant items 📱.
- [ ] **Step 3: Run** `cd web && pnpm vitest run && pnpm lint && pnpm tsc --noEmit` (or the repo's typecheck script) and `GOTOOLCHAIN=go1.27.1 go test ./internal/test/i18ntest/` — PASS.
- [ ] **Step 4: Run the real app** (`make go-run` + `make web-run`), import a German-style statement with zero accounts and an Econumo export; screenshot desktop and phone width.
- [ ] **Step 5: Commit** `chore: remove the old CSV import dialog; docs and regression plan`.
