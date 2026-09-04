# Econumo — Manual Regression Test Plan

This is the canonical manual regression checklist for the Econumo app. Run it
before every release (or any risky change) against a build of the release
candidate. Automated suites (`make test`, apiparity/enginecompare goldens)
already pin the API contract — this plan focuses on what they cannot see:
real-browser UI behavior, responsive layouts, multi-user sharing flows, and
end-to-end journeys.

> **Maintenance rule:** whenever user-observable behavior changes (a new
> feature, a changed flow, a removed control), update the affected checklist
> items in the same PR. See "Maintaining this plan" at the bottom.

---

## 1. Environment setup

1. Build the release candidate: `cd web && pnpm build`, then
   `go build -o ./econumo ./cmd/econumo` (the SPA embeds into the binary).
2. Run against a **fresh, empty** SQLite database so onboarding and
   registration are exercised:

   ```bash
   rm -f /tmp/econumo-regression.sqlite
   DATABASE_URL="sqlite:///tmp/econumo-regression.sqlite" \
   PORT=8181 \
   ECONUMO_ALLOW_REGISTRATION=true \
   ECONUMO_EMAIL_VERIFICATION=false \
   ECONUMO_ADMIN_PORT= ECONUMO_ADMIN_TOKEN= ECONUMO_BILLING_URL= ECONUMO_DATA_SALT= \
   ./econumo serve
   ```

   The binary loads the repo `.env` for any variable not set in the shell, so
   the explicit empty overrides above are required — otherwise a developer
   `.env` can silently enable the admin listener (port collision at boot),
   email verification, or a data salt. `rm -f` guarantees the DB is fresh on
   repeat runs.

3. Open `http://localhost:8181` — the embedded SPA and API share one origin.
4. Test data conventions used below — create these three users (via the
   registration UI, which doubles as the registration test):
   - **User A** ("Alice") — primary user, owns most data.
   - **User B** ("Bob") — sharing partner: connected to A, gets shared
     accounts/budgets in various roles.
   - **User C** ("Carol") — isolation control: never connected; used to verify
     that A/B's data never leaks to an unrelated user.
5. Optionally verify CLI user creation too:
   `./econumo user:create "Carol" carol@example.test secret123`.

### Viewport matrix

Every suite below gets a full pass on **Desktop**; the items marked 📱 get an
extra pass on Mobile and Tablet. When time is short, prioritize: dialogs
(drawer vs centered dialog), row action menus (bottom sheet vs dropdown),
navigation (single-pane vs sidebar).

| Viewport | Size | What changes |
|---|---|---|
| Desktop | 1280×800+ | Sidebar + workspace side by side; dropdown menus; centered dialogs; collapse rail |
| Tablet | 768×1024 | Compact shell (single pane, `<1024px`): sidebar only on `/`, page headers with back buttons, bottom-sheet row actions; dialogs still centered (`≥640px`) |
| Mobile | 375×812 | Same compact shell **plus** bottom-sheet drawers instead of dialogs (`<640px`), FAB on account page, safe-area insets |

---

## 2. Authentication & registration

- [ ] 📱 Register a new user: name/email/password/confirm; inline validation
      (short password, mismatched confirm, bad email); redirected to login.
      Must work in a fresh browser profile with the custom-server section
      collapsed (regression guard: the hidden `host` field once blocked every
      submit on self-hosted instances).
- [ ] Register with an email that already exists → the failure dialog states
      the server's reason ("User already exists", localized), no account
      created.
- [ ] Login with correct credentials → lands on `/` (onboarding for a fresh
      user); wrong password → error dialog, no session created.
- [ ] "Remember email" checkbox pre-fills the email on next visit.
- [ ] Forgot password: send code → email arrives (console transport prints to
      server stdout in dev) → enter code + new password → old password rejected,
      new one works; all other sessions of the user are revoked.
- [ ] Recovery with a wrong/expired code → clear error, password unchanged.
- [ ] Logout (Settings → Profile → Log out, with confirm) → back at login; the
      session disappears from the Sessions list (check from another session).
- [ ] Expired/invalid token: clear the token (or revoke the session elsewhere)
      → next API call redirects to `/login?reason=expired` with the
      session-expired notice.
- [ ] With `ECONUMO_ALLOW_REGISTRATION=false`: Sign-up tab is disabled on the
      login screen, opening `/register` directly redirects to the login page,
      and registering via API returns an error.
- [ ] With `ECONUMO_EMAIL_VERIFICATION=true` (separate boot): fresh
      registration → first login is blocked, code email is sent, verification
      dialog accepts the code, resend has a cooldown, then login proceeds.
- [ ] Language badge/selector on the login page switches the auth UI language
      and persists.

## 3. Onboarding (fresh user)

- [ ] 📱 A fresh user lands on onboarding; steps show completion checkmarks as
      they are done (account created, transactions, classifications, avatar,
      connections, budget).
- [ ] Inline actions work from the onboarding page: add account, import CSV,
      pick avatar.
- [ ] "Complete onboarding" navigates to `/budget`; the sidebar onboarding link
      disappears; `/` now renders the budget.

## 4. Accounts & folders

- [ ] 📱 Create account from the sidebar and from Settings → Accounts: name,
      starting balance (calculator input accepts a formula, e.g. `100+23.5`),
      currency, icon. Balance shows correctly in the sidebar and account page.
- [ ] Starting balance creates a "correction"-style initial transaction dated
      now; account page shows it.
- [ ] Edit account: rename, change icon; balance edit creates a correction to
      match the new balance.
- [ ] Create a second currency account (e.g. EUR). On a fresh self-hosted DB
      (no OER token) there are no global currencies besides USD — create a
      custom currency in Settings → Currencies first (name, code, symbol,
      rate); that is the normal manual path.
- [ ] Folders: create, rename, collapse/expand, hide/show (hidden folder
      disappears from sidebar but its accounts stay reachable via settings),
      delete — deleting a folder MOVES its accounts into the general folder
      (nothing is lost); the general folder itself offers no Delete; reorder
      folders.
- [ ] Drag-and-drop an account within a folder and across folders (desktop);
      order persists after reload.
- [ ] Delete an account with transactions → confirm dialog; account disappears;
      its balance is zeroed by an automatic "Balance adjustment (account
      deleted)" correction; a budget that included it keeps the history
      (spent/budgeted-before figures unchanged).
- [ ] Account page 📱: transaction list groups by day, "today" anchor,
      future-dated (not-posted recurring) entries render above with markers;
      search filters the list; virtualized scroll stays smooth with 100+ rows
      (seed them via CSV import — a generated 100+-row file doubles as the
      import-at-scale test).
- [ ] Mobile: FAB adds a transaction; row tap opens the preview bottom sheet.

## 5. Transactions

- [ ] 📱 Create expense / income / transfer from the account page and via the
      global add button: date picker, amount (formula), category & payee
      selects with **create-on-type** inline creation, tags & labels chips,
      description.
- [ ] Cross-currency transfer (USD account → EUR account) asks for both
      amounts; both accounts' balances update by their respective amounts.
- [ ] Same-currency transfer: single amount; swap from/to button works.
- [ ] Edit a transaction (amount, category, date, account) → balances and
      budget figures update everywhere (sidebar, account header, budget table).
- [ ] Delete from row menu and from preview dialog → confirm → balance updates.
- [ ] Preview dialog: read-only cards; edit/delete hidden when the caller lacks
      write access (see sharing suite); "make recurring" pre-fills the
      recurring dialog.
- [ ] Future-dated transaction shows above the "today" separator and does not
      count toward "balance as of end of today".
- [ ] **CSV import** 📱: pick a file, map columns (single amount and
      inflow/outflow dual mode, date, category, payee, description, tags,
      labels with separator), constant-value fields; result dialog shows
      imported/failed counts and per-row errors; imported rows appear with
      correct signs and classifications.
- [ ] Import with a broken row (bad date/amount) → partial result, failed rows
      detailed, good rows imported.
- [ ] **CSV export**: multi-select accounts, select-all/deselect-all; exported
      file contains the expected rows/columns and respects the account choice.

## 6. Recurring transactions

- [ ] 📱 Create a recurring rule (from a transaction's "make recurring" and
      from Settings → Recurring): type, amount, schedule, accounts, category.
- [ ] Due occurrence appears on the account page as "not posted". Post from the
      account-page row preview posts immediately (dated today); Post from
      Settings → Recurring opens a pre-filled review dialog (scheduled date)
      that you confirm. Both advance the schedule. Skip (advances without
      posting) is offered ONLY on the account-page row preview.
- [ ] Month-end clamping (31st → Feb 28 → Mar 31) is long-horizon — covered by
      unit tests; in a manual run just note the next-date math looks right.
- [ ] Edit and delete a rule; delete asks for confirmation; posted transactions
      survive rule deletion.
- [ ] Recurring settings page groups rules by account; actions are gated by
      write permission on shared accounts.

## 7. Classifications (categories, tags, labels, payees)

For **each** of categories / tags / payees (and labels inside the tags page):

- [ ] 📱 Create, rename, change icon (and type/kind where applicable:
      expense/income category tabs, kind toggle — "Budget tag" is the tag entity,
      "Reporting tag" is the label entity; kind locked after creation).
- [ ] Archive → item leaves active lists and pickers but history keeps it;
      unarchive restores; "active only" switch reveals archived entries.
- [ ] Delete an unused item; deleting one in use warns / behaves per rules.
- [ ] Merge two items (`MergeDialog`) → transactions of the source move to the
      target, source disappears; category merge warns when the source is bound
      to a budget envelope.
- [ ] Manual sort dialog + drag reorder persists order across reload and is
      reflected in transaction-dialog pickers.
- [ ] Search (inline on desktop, search dialog on compact) and empty states.
- [ ] Creating from within the transaction dialog (create-on-type) lands the
      item in the settings list too.

## 8. Currencies

- [ ] "My currencies" vs "Global currencies" tabs; enable/disable one currency;
      bulk enable-all/disable-all. Only executable when server-side global
      currencies exist (rates fetched); a fresh DB has just the locked USD.
- [ ] Base currency and profile currency rows are locked with a reason.
- [ ] Create a custom currency (name, code, symbol, fraction digits, rate);
      it becomes usable for accounts; edit it; delete it (soft delete —
      accounts/transactions in it keep resolving symbol and rate).
- [ ] Rates caption shows the rate and the SPA converts non-base balances in
      totals (sidebar total, budget expense widget note).
- [ ] Change profile default currency (Settings → Profile) → totals and budget
      default currency chips update.

## 9. Budgets — table & plan

- [ ] 📱 Create a budget: name, currency, ≥1 owned account required; appears in
      the sidebar; becomes default when first/chosen.
- [ ] Budget table: budgeted / spent / available columns; expanding an element
      shows details; totals row; uncategorized and labels sections appear with
      info notes when relevant.
- [ ] Element visibility rule: a category/tag/envelope with **either** spending
      or a limit (incl. carried over) is visible; with neither it is not.
- [ ] Set a limit via the available cell / set-limit dialog; formula input;
      limit shows immediately and carries into the next period per rules.
- [ ] Spent cell drilldown opens the transactions dialog (filtered list,
      preview, delete works and refreshes figures).
- [ ] Period strip: navigate previous/next months; figures change. Months
      BEFORE the budget start remain browsable as read-only history (dimmed,
      scrolling keeps extending into the past) and show that month's real
      spending; limit cells are not editable there and set-limit is refused.
      Months past the end month are not offered (the active month stays
      visible even if the stored selection is outside); scrolling never
      extends past the end month.
- [ ] Period strip desktop arrows (‹ ›, left of the strip, hidden on mobile
      where the strip scrolls by touch): they PAN the strip only — the
      selected month and the table below never change; panning to either
      edge keeps extending the window (past months included).
- [ ] Currency filter chips (multi-currency data) filter rows/totals.
- [ ] **Edit structure** mode 📱: create folder, drag elements between folders,
      per-element menu (change currency, move to folder, edit envelope, delete
      envelope), delete folder; leaving the mode persists the layout.
- [ ] Envelopes: create via the "+" button on a folder header in Edit
      structure mode (name, currency, categories multi-select);
      transactions of member categories aggregate under the envelope; edit
      membership; delete envelope returns categories to top level.
- [ ] Tag on a transaction: spending counts toward the **tag** element, not the
      category (tagged-spend accounting rule).
- [ ] **Plan sheet** 📱: spreadsheet grid renders months; inline edit of a
      planned amount; keyboard cell navigation (arrows), Excel-style
      fill-right by drag handle (desktop) and Shift+Arrow; month window
      scrolling; hide-empty-rows toggle; transfers/balance totals rows show
      tooltips.
- [ ] Budget with accounts in two currencies: per-currency balances section is
      correct; expense widget shows the conversion note.

## 10. Budget lifecycle & list

- [ ] Budgets list (Settings → Budgets): set default, open, edit (name +
      accounts replace-set), delete with confirm.
- [ ] **Accounts membership**: adding/removing accounts in edit; an account
      with transactions inside the budget window since the start of the
      current month cannot be removed (clear error); hidden-accounts note and
      included counter are correct (never "N of M" with N>M).
- [ ] **Duplicate** (clone): name pre-fills with a localized "(copy)" suffix;
      deep copy with/without plans from a chosen start month; copy starts
      unarchived/open-ended; structure and sharing carried.
- [ ] **Duplicate/Complete as shared admin** ("Full control", not owner): both
      actions are offered and succeed; the cloner owns the copy, the former
      owner appears in its sharing set as an accepted "Full control"
      participant, and all member accounts (the former owner's included) carry
      over. A "Can edit"/read-only participant is offered neither action.
- [ ] **Complete**: sets end month; optionally continues with a copy (+ plans,
      new name). Ended budget: period strip clamps at end month, set-limit
      refuses later periods.
- [ ] **Archive**: archived budget is hidden from the main list (archived
      section toggle reveals it) and is read-only — every write is refused
      with the "budget archived" error except unarchive/delete/sharing-exit
      actions; unarchive restores writability.
- [ ] Reset budget (if surfaced in UI) clears plans after confirm.

## 11. Sharing — connections, accounts, budgets (multi-user)

Run with Users A and B side by side (two browser profiles/windows), verifying
User C sees none of it.

**Connections**
- [ ] 📱 A generates an invite code; B accepts it via the dialog → both see the
      connection with avatars; wrong/expired code → clear error; rate limiting
      after repeated bad codes (429 toast).
- [ ] Delete the connection (confirm) → shared grants are revoked on both
      sides.

**Account sharing**
- [ ] A shares an account with B (`guest`, then upgrade to `user`, `admin`):
      B gets a sharing-request badge; the requests dialog lists the invite
      with folder selection; Accept places the account in the chosen folder
      and A's categories/payees/tags resolve on B's side immediately (no
      "Uncategorized" rows, no stale caches).
- [ ] Decline works (with confirm) and removes the pending invite.
- [ ] Role behavior: `guest` = read-only (no add/edit/delete transaction
      controls anywhere — row menu, preview, FAB); `user` can write
      transactions; `admin` can also edit the account. Verify on desktop
      dropdowns AND mobile bottom sheets.
- [ ] Shared account shows the owner's avatar/mark in B's sidebar; A sees B's
      avatar on the account's shared stack.
- [ ] B sees A's categories/payees/tags via the shared account and can use
      them on transactions in that account; B cannot edit A's classifications.
- [ ] A revokes access → the account disappears from B's sidebar immediately
      (or on next sync); B's own data untouched.
- [ ] Owner deletes a shared account → it disappears for B too.

**Budget sharing**
- [ ] A shares a budget with B (reader and admin roles): B accepts via the
      requests dialog; accepted budget becomes B's default; B sees elements,
      figures, and A's shared accounts inside the budget.
- [ ] Budget roles: reader cannot change limits/structure (entry points
      disabled, not just failing); admin can set limits and edit structure.
- [ ] A participant leaving (decline after accept / revoke) removes their
      accounts from the budget with them.
- [ ] Archived shared budget: B can still leave/decline but not write.

## 12. Profile & security settings

- [ ] 📱 Avatar picker: icon + color; persists; shows in sidebar and shared
      stacks on the partner's side.
- [ ] Name inline edit with validation (length limits).
- [ ] Default currency picker and language dialog persist (language also
      server-side — a relogin/other device keeps it).
- [ ] Analytics toggle (Settings → Profile → privacy): switching it off
      persists across a reload; log out and back in — the toggle still reads
      off; a read-only user (lapsed trial) can still flip it, unlike other
      writes on that account.
- [ ] **Change password**: wrong old password rejected; success revokes all
      *other* sessions (verify: second browser session is logged out, current
      one stays).
- [ ] **Change email**: request (new email + password) → code sent to the new
      address → confirm; resend with cooldown; wrong code rejected; login works
      with the new email only.
- [ ] **Sessions** 📱: list shows device descriptions, current badge, relative
      last-active; revoke one (other) session logs that device out; revoke-all-
      others keeps only the current; revoking the current session logs out.
- [ ] **Personal access tokens** 📱: create with expiry presets + custom date;
      token revealed exactly once with copy button; API call with the PAT
      works (e.g. `GET /api/v1/user/get-user-data`); revoked PAT stops working;
      list shows last-used/expiry.

## 13. Cross-cutting & platform

- [ ] i18n: switch to a non-English language — spot-check every page for
      untranslated keys/overflowing labels; server-rendered errors (e.g. bad
      login) arrive translated; switch back.
- [ ] Sync button: spins during refetch; failure (kill the server briefly)
      turns it amber with a tooltip; recovery clears it. App restores from the
      persisted cache on reload without a boot-loader flash.
- [ ] Update notices (environment-dependent — needs `ECONUMO_CHECK_UPDATES`
      and reachability of econumo.com): with a newer release available, the
      dismissible sidebar notice and the settings "update available" row
      appear (simulate by pointing `ECONUMO_VERSION` at an old value).
- [ ] Readonly/trial gating (cloud only, `ECONUMO_TRIAL` set): expired user
      gets 402 toasts on writes, subscription banner shows; security actions
      (logout, password, sessions) still work.
- [ ] 404 page renders for an unknown route; deep links (e.g. `/settings/
      profile/sessions`, `/account/<id>`) survive a hard reload.
- [ ] API docs reachable from settings footer (`/api/doc`).
- [ ] No console errors during a full pass (keep dev tools open); no PII in
      server logs (spot-check).

## 14. Responsive-specific sweep 📱

A dedicated pass on Mobile (375×812) and Tablet (768×1024):

- [ ] Navigation: `/` shows the sidebar-as-home; entering any page shows a
      back-button header; back always returns to the logical origin.
- [ ] Every dialog used in the suites above renders as a bottom-sheet drawer
      (short content: previews, action lists, confirms) or a full-screen sheet
      (long forms, e.g. Add transaction) on mobile (<640px), and a centered
      dialog on tablet — content scrolls, safe areas respected, keyboard does
      not cover inputs.
- [ ] Row actions open bottom sheets (accounts, transactions, classifications,
      recurring, sessions, tokens, budgets).
- [ ] Long lists scroll smoothly; sidebar scroll position is remembered when
      navigating back.
- [ ] Plan sheet is usable on tablet (fill handle desktop-only is expected).
- [ ] Toolbar buttons collapse labels to icons without overflow; no horizontal
      page scrolling anywhere.

---

## Maintaining this plan

- This document lives at `docs/regression-test-plan.md` and is part of the
  definition of done for behavior changes: **any PR that changes
  user-observable behavior must update the affected checklist items** (add
  cases for new features, edit changed flows, delete removed ones).
- Keep items phrased as verifiable outcomes ("X happens"), not instructions
  ("click X").
- Keep the 📱 markers accurate — they drive the mobile/tablet passes.
- When a regression escapes to production, add a checklist item that would
  have caught it.
