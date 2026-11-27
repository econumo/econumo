# CSV import wizard

## Why

A user tried to import a bank statement and gave up. In their words, the import "was
created for developers, not normal people". CSV import is step 0 of Getting started
("Bring in your history"), so this is often the first thing a new user does.

Today's dialog (`web/src/features/transactions/ImportCsvDialog.tsx`) shows all 10 fields on
one screen, each a dropdown of raw header names with a swap icon that toggles between
"from a column" and "a fixed value". Account is required: either a column of account names
or an existing account. A bank statement covers one account and has no account column, and
during onboarding the user has no accounts yet, so a typical statement cannot be imported
without editing the file. The user sees nothing of what will be written until the import
is done, and an ambiguous date like `03/04/2026` is silently read day-first.

## Goals

- Someone who has never heard "column mapping" imports a bank statement on the first try.
- Works from zero accounts (Getting started) as well as later.
- Every screen shows real values from the file, so the user recognises their data instead
  of reasoning about header names.
- A preview of the transactions, exactly as they will be written, before anything is
  written.
- Better guesses: every step comes pre-filled; AI improves the guesses where it is on.
- People moving from another app can still bring category, payee, tag and labels.

## Non-goals

- Formats other than CSV (OFX, QIF, XLSX, PDF).
- Duplicate detection against existing transactions (the provider importers have a matcher;
  CSV keeps its current behaviour).
- Fixing the balance of an account the import touched on the Settings path ("Keep current
  balance" was dropped from onboarding on 2026-11-08; in Getting started the Accounts step
  that follows lets the user type the real balance).
- Saving a mapping per bank for next time.
- Changing the server's CSV parsing for other clients.

## Anti-goals

- Any screen that reads like a developer tool: words like "mapping", "field", "column
  mode", "CSV column", raw header names shown without example values.
- Sending amounts, dates, descriptions or any cell value to the AI.
- A wrong preview: what the preview shows must be what gets written.

## Approaches considered

- **A. Result first:** guess everything, show a preview, hide column matching behind
  "Something looks wrong". Fewest clicks when the guess is right, but the user never sees
  why a value landed where it did.
- **B. Step-by-step wizard (chosen):** one question per screen, each pre-filled and showing
  sample values, ending with a preview. Chosen by Dmitry: guided, hard to get lost in, and
  the preview still catches mistakes.
- **C. Reword the current form:** cheapest, but still a form of dropdowns, which is the
  complaint.

## The wizard

> **Layout revised 2026-11-18: design 17, "sidebar stepper".** After trying a step bar with
> column cards and then a folding table, Dmitry compared 20 prototypes (local branch
> `feature/csv-import-lab`, `web/lab/variants/v17-sidebar-stepper.tsx`) and picked 17. It
> overrides the layout described below wherever they differ; the steps, their questions and
> all behaviour (guessing, AI, normalisation, fixes, retry, lock) stay.
>
> - **Desktop:** a wide dialog split in two. Left: title, the file (name, row count), then a
>   vertical list of steps (Account, Date, Amount, Category, Description, Payee, Extra
>   details, Check and import; Category and Payee were split out of Extra details on the
>   product owner's request, 2026-11-26), each with a check mark when answered and its answer in plain words underneath
>   ("New: Girokonto · EUR", "Buchungstag · 2 Oct 2026", "Betrag · 1.234,56",
>   "12 to import · 1 left out"); a footer line "12 transactions ready · 1 left out". Any step
>   can be opened from the list. Right: "Step 2 of 8 · Date", the step's question as a heading,
>   the body, and Back / Continue (Import on the last step).
> - **Phone:** the step list is the first screen (with "We read your file and filled in what
>   we could. Tap a step to change it." and a primary "Check N transactions"); tapping a step
>   opens it full screen with a back arrow to the list.
> - **Choosing a column:** a list of radio options, one per column, each showing the header
>   and its first values in one line ("02.10.2026 · 03.10.2026 · 05.10.2026"); a column used by
>   another step carries a small "now payee" style tag. This replaces the table and the
>   folding columns.
> - **Formats** sit under the choice as segmented controls with a live example ("“02.10.2026”
>   will be read as 2 Oct 2026").
> - **Check and import:** three stat tiles (Transactions, Money in, Money out), then the list of
>   transactions as they will be added (date, description, payee · category, signed amount).
>   Rows that will be left out appear in the list as problem rows with their reason and the
>   inline fix (type a date/amount, or skip), instead of the one-at-a-time card.

One dialog (`ResponsiveDialog`, a sheet on a phone) with a step bar, Back and Next.

**Moving between steps.** Once a file is loaded, the step bar is clickable: the user can
jump to any step, forward or back, in any order, because every step is already pre-filled.
Steps are never locked behind earlier ones. A step whose answer is missing or causes
skipped rows carries a warning mark in the bar, and Import is disabled until the required
ones (account, date, amount) are answered. On a wide dialog the bar shows every step's
name; on a phone it shows numbered dots with the current step's name, and each dot is
tappable. Back and Next stay for people who just go through in order.

**An Econumo export goes straight to the end.** When the file's header is an Econumo export
(see Guessing), the wizard skips from File directly to the Preview step: everything is
already mapped, so the user only confirms. In that case the Preview also shows the account
list (described under step 2, each new account with its currency pre-filled from
`account_currency`), so currencies can be checked without leaving the screen. The step bar
is still there, so any step can be opened to change something. The same shortcut is not
used for other files: a guessed mapping of a bank statement still walks through the steps.
It replaces `ImportCsvDialog` everywhere it is used today: Getting started's Import step
(`web/src/features/onboarding/ImportStep.tsx`) and Settings → Data
(`web/src/features/imports/ImportsDataPage.tsx`). The `initialFile` (drag-and-drop) entry
and the `onComplete(result)` contract stay, so `ImportStep` and `finishImportStep` keep
working.

Each "which column" step shows the file the way a spreadsheet does: a table of the header
and the first rows (about 6), scrolling sideways on a phone. The step asks one question
("Tap the column with the date") and the user taps anywhere in a column to choose it. The
chosen column is highlighted; columns already given to another step carry a small label
naming it ("Amount"). There are no dropdowns of header names and no per-column cards.
*(Revised 2026-11-17 after trying the first build: separate cards per column broke the file
into pieces and were hard to read.)*

Warning marks in the step bar are for a missing required answer only. Rows that will be
skipped are an informational note on the step and on Check, not a warning. Dates on Check
are shown in the UI language's date format, not ISO.

### 1. File

Drop or pick a `.csv` (10 MB limit as today). The browser parses it (`lib/csv.ts`), and
the guesser runs (see Guessing). An unreadable or empty file says so in plain words and
stays on this step.

### 2. Account: "Which account is this statement from?"

- **A new account** (default when the user has no writable accounts): name (starts empty:
  the file name is not a default; an empty name is a missing answer until typed) and currency
  (defaults to the user's currency, the same picker the Accounts step uses).
- **An account I already have** (default when they have one or more): the account picker,
  writable accounts only, as today.
- **Each row says which account** (a small secondary option): pick the column, shown with
  sample values, as on the other steps. Pre-selected when an Econumo export is recognised.
  The step then lists the distinct account names found: a name matching one of the user's
  writable accounts (case-insensitive, as the server matches) reads "Your existing
  account · EUR"; every other reads "New account · CAD" with a quiet "Change
  currency" that opens a per-account picker. Where that currency comes from is a second
  answer row, a role like the others (one part of the file, one role; never uploaded, it
  only decides new accounts' currency): a part of the file holding three-letter codes
  (an Econumo export's `account_currency`) is guessed, and its Change offers the parts
  holding codes plus "My file has no currency"; without one, "New accounts will be in
  [USD ▾]" picks one currency for every new account (the user's currency by default).
  Per account: its own pick, else the one known code its rows name, else that currency.
  An unknown code reads "your file says AUD, which isn't set up here". *(Builder-check
  call: the server alone would create them in the instance base currency and ignore
  `account_currency`.)*

New accounts are created with `create-account` when the user presses Import, not earlier,
and the import's rows then resolve to them (by id for the single-account case; by the now
existing name for the per-row case). Created accounts are remembered for the life of the
dialog, so a retry never creates them twice. If the user closes the dialog after an account
was created but before any rows landed, the empty account stays; it shows up in Getting
started's Accounts step or the account list, where it can be removed. *(My call.)*

### 3. Date: "Which column has the date?"

Column cards. Under the cards, the selected column's values are checked:

- If every value reads one way only, nothing more is asked.
- If they are ambiguous (`03/04/2026` could be either), ask with a real example from the
  file: "Is 03/04/2026 the 3rd of April or March 4th?" Pre-select from the values (any day
  above 12 settles it), else from the browser's locale (more specific than the UI language:
  en-US vs en-GB).
- Rows whose date cannot be read are counted ("3 rows have no readable date and will be
  skipped").

### 4. Amount: "How does the file show amounts?"

- One column with + and − (default), or separate "money in" and "money out" columns. Cards
  as above; the two-column choice asks for each.
- "Spending shows as: −4.50" with a switch to flip signs, using a real value from the file
  (some banks export spending as positive). One-column mode only: with separate columns,
  money in is always `+|value|` and money out `−|value|`, whatever sign the file uses, so
  no switch is shown. *(My call.)*
- Decimal comma vs point is detected from the values; if ambiguous, asked with an example.

### 5. Category (optional): "How are the payments sorted?"

Built like Description: the guessed answer first ("Strata · Interest · Car Insurance", from
“category”), Change opens "Which of these is the category?" with short names that repeat
(not ids or currency codes), "Show all", the "Already used" group and "The file has no
category". Optional: with nothing that fits, or None picked, the step answers calmly ("Your
file has no categories. You can sort transactions into categories later.") with Looks
right; it never warns and never blocks Import. The "N new categories will be created" note
and the shared-account "left out" note live here. One column has one role on every step: a
pick takes its column from whichever step read it (the last pick wins), and a required step
that loses it shows its warning mark again; the guess and the AI never share a column.

### 6. Description: "Which column describes each transaction?"

Column cards, plus "None". This is the text a bank calls details, reference, merchant.

### 7. Payee (optional): "Who was paid, or who paid you?"

As Category, offering any text; "Your file doesn’t say who was paid. You can add payees
later." when nothing fits. The shared-account "left out" note for payees lives here.

### 8. Extra details (optional): "Anything else to bring over?"

For people moving from another app. Lists the columns not used yet, each with sample
values, and lets the user mark each as Budget tag, Reporting tags, or leave it out
(default: leave out, unless the guesser recognised it). Labels get the separator choice
that exists today, in plain words ("Several labels in one cell are separated by…"). Shown
only when the file has unused columns; otherwise skipped. A "new tags will be created: 12"
note as today.

The old "fixed value for every row" options (one category/payee/tag/labels/date/description
for the whole file) are dropped from the UI; nobody bringing a bank statement needs them.
The server still accepts those fields.

### 9. Preview: "Here's what will be imported"

- **One transaction at a time**, not a list: a card laid out like the app's transaction
  view (account, date, signed amount in the account's currency, red/green; description;
  payee, category, tag and labels when mapped), headed "Transaction 3 of 214".
- **Switching between records:** Previous / Next arrows (and left/right keys, a swipe on a
  phone) move through every row of the file. When some rows will be skipped, a "Next
  problem" button jumps to the next one, and a skipped row's card says why ("No readable
  date") instead of showing a transaction.
- **Fixing a problem row on its card:** a row skipped for an unreadable date or amount
  shows an input for that value right on the card (a date picker, an amount field). Once
  filled, the row counts as a transaction and is uploaded with the typed value; "Skip this
  row" leaves it out explicitly. These per-row fixes live in the dialog's state, and they
  are dropped for a row if a step change makes it readable on its own. There is no
  editable table of all rows: whole-file problems are fixed at their step, and any other
  field can be edited in the app after the import. *(Agreed: no table; per-row fix is my
  call, recommended and not objected to.)*
- Under the card, always visible: the original line from the file, so a wrong date or
  amount can be traced to its source. Each value on the card links back to the step that
  shapes it (date, amount, description, extras); a "Something looks wrong?" link goes back
  to step 3 (step 2 for an Econumo export).
- Totals above the card: "214 transactions, 2026-01-03 to 2026-03-31. Money in 6,000.00,
  money out 4,812.40." Rows that will be skipped are counted with the reason.
- Primary button "Import 214 transactions". The progress bar for chunked uploads stays.

The result screen (`ImportResultDialog`) stays, with plain-language error wording.

## Making the preview true

The preview is built by the same code that produces what is uploaded. The browser parses
dates and amounts with the user's choices (date order, sign flip, decimal separator) and
rewrites the date and amount cells as `YYYY-MM-DD` (or `YYYY-MM-DD HH:MM:SS` when the file
carries a time of day, which the server stores today and an Econumo export writes; the
preview shows the time then) and plain decimals in the chunk files it already builds
(`buildCsvText`, `runImport` in `importCsv.ts`). The server's own heuristics then only ever
see unambiguous values. Rows the browser cannot read are not sent; they are counted as
skipped in the preview and appear in the result screen's failed count and error list with
their original row numbers. *(Builder-check calls.)*

### Failures and retry

`runImport` keeps going past a failed chunk, so a run can end part-written. The wizard
keeps per-chunk state; the result screen offers "Try the rest again", which re-sends only
the chunks that failed, never the ones that landed. There is no whole-import retry after
any rows were written, which keeps Getting started's existing guard against duplicates.
*(Builder-check call.)*

No server change is needed for this; the import endpoint, mapping and response are
unchanged.

## Guessing

Every step is pre-filled by a guesser that runs once after the file is parsed:

1. **Econumo export** recognised by its exact header (as today's `autoDetect`): everything
   mapped, including category/payee/tag/labels and "each row says which account".
2. **Built-in rules** (always, no AI needed): header synonyms for date, amount, money
   in/out, description, payee, category in all 11 UI languages (e.g. Buchungstag, Betrag,
   Verwendungszweck, Fecha, Importe, Дата, Сумма, Описание), checked against the column's
   value type so "Date" holding text is not picked.
3. **AI** (only when AI is configured and the user's `ai` option is on): one request after
   parsing, which may override the built-in guess for columns it is confident about. The
   wizard opens immediately with built-in guesses and updates still-untouched steps when
   the AI reply arrives; a step the user already answered is never changed. Failure,
   timeout or AI off = built-in guesses only, no message.

### What the AI receives

Only:

- the column header names, in file order;
- per column, a type computed in the browser: `date`, `number`, `text`, `empty` (and
  for numbers, whether negatives occur);
- the user's language.

Never a cell value: no amounts, dates, descriptions or payee text. This adds "CSV column
header names and computed column types" to the AI data allowlist in CLAUDE.md, with a guard
test on the prompt's exact key set like every other prompt builder.

The reply is a JSON object assigning header names to roles (`date`, `amount`,
`amountIn`, `amountOut`, `description`, `payee`, `category`, `tag`, `labels`,
`account`), each optional; names not in the header are ignored.

Server side: a new endpoint `GET /api/v1/transaction/suggest-import-columns` (auth, a read like `onboarding/suggest-icon`; reads stay GET per the API conventions
and `GET` is never restricted for readonly callers) in the `transaction` feature, with its prompt in
`prompts/import_columns.md`, its own per-user AI rate-limit scope, and the established AI
rules (re-read the `ai` option before the call, never fail the request, WARN with ids and
`err_type` only). It answers an empty suggestion rather than an error when AI is off or
fails. AI eval cases (tag `aieval`) cover English, German and Russian bank headers.

## Analytics

Keep the four existing events. Add `step` to `TRANSACTION_IMPORT_CLOSE` (which step was
abandoned), and new events for the AI guess used and the preview's "Something looks
wrong?" link. Keys go into `METRICS` per the rules.

## Other rules this touches

- i18n: all new strings in all 11 catalogues; drop keys that are no longer used.
- Offline: the wizard's Import button reads `useOnline` as today.
- Regression plan: rewrite the CSV import items in `docs/regression-test-plan.md` (the
  Import step items around line 405, the Settings → Data import items).
- API parity goldens and the OpenAPI docs for the new endpoint.
- Phone: every step must work in the bottom sheet; column cards stack.

## Left to the builder

- Component structure, state shape, and how steps share state.
- Exact copy (in plain words, no developer terms), layout of the cards, the step indicator.
- The header synonym lists and the value-type detection rules.
- The default name for a new account.
- How much of `importCsv.ts` / `autoDetect` is reused versus rewritten.
- Test split between unit tests for the guesser/normaliser and component tests per step.
