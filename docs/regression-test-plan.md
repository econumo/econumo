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
- [ ] Password login lands in the app and STAYS there: sign in, confirm the
      dashboard loads and a reload keeps you signed in (the token survives).
      A 401 from the background `get-identity-list` probe must never bounce a
      just-signed-in user back to `/login`. Same check after an OAuth
      "Continue with..." sign-in.
- [ ] With `ECONUMO_ALLOW_REGISTRATION=false`: Sign-up tab is disabled on the
      login screen, opening `/register` directly redirects to the login page,
      and registering via API returns an error.
- [ ] With `ECONUMO_EMAIL_VERIFICATION=true` (separate boot): fresh
      registration → first login is blocked, code email is sent, verification
      dialog accepts the code, resend has a cooldown, then login proceeds.
- [ ] A verification code is single-use and only the newest one works: wait out
      the cooldown and resend, then the FIRST emailed code is refused with "The
      confirmation code is not valid." while the second one confirms; after the
      confirmation, submitting that same code again is refused the same way and
      login still proceeds.
- [ ] Language badge/selector on the login page switches the auth UI language
      and persists.
- [ ] 📱 With Google/Apple/SSO configured: "Continue with…" buttons appear on
      both `/login` and `/register`, on desktop and mobile; sign-in through
      each provider completes and lands the user in the app.
- [ ] 📱 iOS home-screen PWA, Sign in with Apple: the app shell fills the whole
      screen afterwards — no white band below the sidebar footer. Apple is the
      only provider that returns by cross-site POST, which leaves iOS holding a
      stale (too short) `svh` for that window; the full-screen shells are sized
      in `dvh` so the band cannot appear. It used to survive a reload and a
      rotation, so relaunch the PWA between attempts when retesting.
- [ ] Apple key is loaded from a FILE: with
      `ECONUMO_OAUTH_APPLE_PRIVATE_KEY_FILE` pointing at the unmodified `.p8`,
      the server boots and Apple sign-in completes — under systemd as well as
      Docker. A path that does not exist, or an empty file, fails at boot with
      a message naming the variable; the removed inline
      `ECONUMO_OAUTH_APPLE_PRIVATE_KEY` also fails at boot and points to the
      file variable.
- [ ] First sign-in through a provider (no matching Econumo account, email
      verified) creates a new account and lands on onboarding.
- [ ] Sign-in through a provider whose verified email matches an existing
      account that HAS a password is refused with "An account with this email
      address already exists. Sign in with your password (or reset it), then
      link this provider from Settings." — no session, no linked identity, and
      the account is otherwise untouched (its password, sessions and personal
      access tokens all keep working). Signing in with the password and
      linking from Settings then works.
- [ ] Sign-in through a provider whose verified email matches an existing
      PASSWORDLESS account (one created through another provider) auto-links
      with no confirmation dialog, signs into that account, shows the new
      provider under Settings → Profile → Sign-in methods, and delivers the
      owner's notice email (console transport prints it to server stdout in
      dev) naming the provider, addressed to the account email and CC'd to
      every OTHER address the account's providers reported (the account email
      itself is never duplicated into the Cc).
- [ ] Starting a provider sign-in in one browser and opening the returned
      Econumo callback URL in a DIFFERENT browser (or a private window) fails
      with "The sign-in attempt expired or was already used." — only the
      browser that started the flow can complete it.
- [ ] Sign-in with a provider that reports an unverified email is rejected
      with "The sign-in provider has not verified this email address."; no
      account created or linked (this includes a Google account whose
      sign-in address Google reports as unverified).
- [ ] With `ECONUMO_ALLOW_REGISTRATION=false` (separate boot) and no matching
      account: sign-in via a provider shows "Registration is disabled on this
      server. Sign in with an existing account first."
- [ ] Cancelling at the provider (deny consent / close the flow) returns to
      Econumo showing "Sign-in was cancelled."
- [ ] Password reset is a full reclaim: on an account with an open session, a
      personal access token and two linked providers (one whose provider email
      matches the account's, one whose does not), completing "Forgot password"
      signs the session out, makes the token stop authenticating, removes the
      provider with the different email from Settings → Profile → Linked
      accounts, and keeps the matching one. A provider sign-in started just
      before the reset can no longer be completed afterwards (the returning
      browser lands on the login page with an error instead of a session), and
      a pending email change is cancelled. A provider sign-in that was already
      mid-flight when the reset landed cannot be completed either — finishing
      it returns the sign-in error rather than a session. An email change that
      was pending (code sent but not yet confirmed) can no longer be confirmed
      after the reset, even if the confirmation was already in flight — it
      reports "The confirmation code is not valid." and the account keeps its
      own address. The recovery dialog says so before the reset is submitted.
      CLI `user:change-password <email> <new>` performs the same reclaim
      (sessions, tokens, foreign identity, pending grants).
- [ ] A reset code is single-use and only the newest one works: request
      "Forgot password" twice and the FIRST emailed code is refused with
      "Reset password error" (the second one still works); after a reset
      completes, submitting that same code again is refused the same way and
      the password stays the one the completed reset set.
- [ ] A provider-created (passwordless) account using Settings → Profile →
      "Set a password" keeps its linked provider through that flow — it is the
      same reset endpoint, and the provider vouches for the account's address.
- [ ] Linking from Settings completes only in the browser that started it:
      start the link in one browser, then open the returned Econumo callback
      URL in a DIFFERENT browser signed in as another user — that user's
      Sign-in methods page shows an error and gains NO identity, and the
      provider account stays unlinked everywhere.
- [ ] A FAILED link from Settings (e.g. linking a provider account already
      linked to another Econumo user) returns to Settings → Profile → Linked
      accounts with the error banner there ("This external account is already
      linked to another Econumo account."), not to the login page; the banner
      does not survive a reload. 📱 The app does the same via the deep link.
- [ ] RP-initiated logout (custom OIDC slot with an end-session endpoint):
      logging out redirects through the provider and back to `/login`.
- [ ] Logging out of a Google/Apple session (or any provider in the app) ends
      the Econumo session and lands on `/login` with no interstitial notice —
      those providers publish no end-session endpoint, so there is no IdP
      redirect and nothing to confirm.
- [ ] 📱 App: starting provider sign-in opens the in-app browser sheet, not an
      embedded web view; completing sign-in returns to the app (the verified
      `https://<backend>/oauth/app-return` link, or the
      `com.econumo.app://oauth` scheme on a backend without app links) and the
      sheet closes automatically. An expired or already-used attempt returns to
      the app's login page with "The sign-in attempt expired or was already
      used." (not a web page inside the sheet).
- [ ] 📱 With app links configured, the return from the provider opens the app
      directly; opening the return URL in a browser with the app not installed
      shows the "return to the app" page with a single "Open the app" link (and
      no handoff code visible in the page text).
- [ ] 📱 App: tapping a provider button twice while the browser sheet is
      opening starts ONE flow; the buttons stay disabled until the sheet
      closes or the sign-in returns.
- [ ] Web: with a custom backend selected (different origin than the page),
      no provider buttons are shown.
- [ ] Web: with a custom backend selected (different origin than the page),
      Settings → Profile → Sign-in methods shows the linked list but offers no
      Link buttons.
- [ ] `ECONUMO_PASSWORD_LOGIN=false` with NO provider configured: the server
      refuses to start, naming `ECONUMO_PASSWORD_LOGIN`. A malformed value
      (e.g. `nope`) also fails at boot.
- [ ] 📱 `ECONUMO_PASSWORD_LOGIN=false` with a provider configured: `/login`
      shows only the provider buttons — no email/password fields, no "Sign
      in" or "Forgot password" buttons, no "or continue with" divider. With
      `ECONUMO_ALLOW_REGISTRATION=true` the Sign-up tab also shows only the
      provider buttons (plus the privacy note), and a first sign-in through the
      provider creates the account; with it `false` the Sign-up tab stays
      disabled.
- [ ] `ECONUMO_PASSWORD_LOGIN=false`: calling `login-user`, `register-user`,
      `remind-password`, `reset-password` or `update-password` directly returns
      400 "Password sign-in is disabled" (localized); existing sessions and
      personal access tokens keep working.
- [ ] `ECONUMO_PASSWORD_LOGIN=false`: Settings → Profile shows neither "Change
      password" nor "Set a password"; on Sign-in methods an account with one
      linked provider cannot unlink it ("This is your only sign-in method, so
      it cannot be unlinked."), even if it has a password.
- [ ] `ECONUMO_PASSWORD_LOGIN=false`: a provider sign-in whose email matches an
      account WITH a password is still refused, now with "An account with this
      email address already exists and can't be linked automatically. Contact
      your administrator." An account that linked the provider while passwords
      were on signs in through it normally.
- [ ] 📱 App pointed at a backend with `ECONUMO_PASSWORD_LOGIN=false`: on a
      cold start the login screen drops the password form as soon as that
      server's config arrives (no restart needed). Typing the address of a
      backend that allows passwords into the custom-server field brings the
      password form back within a second; switching back hides it again. An
      unreachable address shows the password form (the server then decides).
      Switching from a backend with no providers to a provider-only one shows
      the new backend's provider buttons (never an empty screen).
- [ ] Web, `ECONUMO_PASSWORD_LOGIN=false` with a custom backend selected on a
      different origin: the password form is shown again (the provider buttons
      are not), since the serving instance's setting says nothing about that
      backend.

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
- [ ] Not-posted recurring entries due today or overdue lead the "today" group,
      above today's real transactions, however far in the past they were due —
      so opening the account puts them first on screen. They stay dimmed with
      the red "not posted" note; templates due in the FUTURE keep their own day
      group above the fold. With no transactions today at all, the pinned rows
      still open a "today" group of their own.
- [ ] Mobile: FAB adds a transaction; row tap opens the preview bottom sheet.

## 5. Transactions

- [ ] 📱 Create expense / income / transfer from the account page and via the
      global add button: date picker, amount (formula), category & payee
      selects with **create-on-type** inline creation, tags & labels chips,
      description.
- [ ] Cross-currency transfer (USD account → EUR account) asks for both
      amounts; both accounts' balances update by their respective amounts.
- [ ] Same-currency transfer: single amount; swap from/to button works.
- [ ] 📱 Transfer with no "To" account: Add/Update shows "Required field" under
      the To select and sends nothing; the API rejects the same body with a
      400 on `accountRecipientId` (create AND update).
- [ ] A legacy transfer row whose recipient is NULL (renders "[Hidden
      account]") still offers Delete from the row menu and the preview
      dialog, and deleting it restores the source balance.
- [ ] Edit a transaction (amount, category, date, account) → balances and
      budget figures update everywhere (sidebar, account header, budget table).
- [ ] Delete from row menu and from preview dialog → confirm → balance updates.
- [ ] Preview dialog: read-only cards; edit/delete hidden when the caller lacks
      write access (see sharing suite); "make recurring" pre-fills the
      recurring dialog.
- [ ] Future-dated transaction shows above the "today" separator and does not
      count toward "balance as of end of today" — including one dated exactly
      00:00 tomorrow (SQLite AND PostgreSQL).
- [ ] Transaction list rows carry `isImported` (0/1) in the API response; an
      imported row shows the import glyph (tooltip "Imported"), hand-entered
      rows do not; the preview dialog of an imported row lists "Imported from"
      (source · card · merchant amount currency · posted time).
- [ ] **CSV import** 📱: pick a file, map columns (single amount and
      inflow/outflow dual mode, date, category, payee, description, tags,
      labels with separator), constant-value fields; result dialog shows
      imported/failed counts and per-row errors; imported rows appear with
      correct signs and classifications.
- [ ] Import with a broken row (bad date/amount) → partial result, failed rows
      detailed, good rows imported.
- [ ] **CSV export**: multi-select accounts, select-all/deselect-all; exported
      file contains the expected rows/columns and respects the account choice.

## 5a. Imports — Apple Wallet

- [ ] Settings → Data group has two rows 📱: "Import & export" (only the CSV
      import/export rows; dialogs open as before) and "Apple Wallet" (its own page:
      setup, cards, "Import queue" link; back returns to Settings).
- [ ] "Set up Apple Wallet" creates the source (idempotent: a second click or a
      second device does not create a second source); the section flips to
      "Connected" with a seven-step checklist (Install econumo-wallet-v1, Install
      econumo-setup-v1, Configure the Shortcuts, Run econumo-wallet-v1 once,
      Create the automation, Make the first payment with the iPhone unlocked,
      Switch the automation to Run Immediately), every box unticked; "Disconnect" (confirmation) removes the source, its
      cards, its queue and the hand ticks; already-imported transactions stay.
- [ ] Any step can be ticked/unticked by hand; ticks survive a reload and are
      per source (a reconnect starts with an empty list) 📱.
- [ ] A ticked step (by hand or automatically) folds to its title only: its
      text, download link, buttons and "Configure manually" link disappear;
      unticking it by hand brings them back 📱.
- [ ] Steps 1–2 links download `econumo-wallet-v1.shortcut` and
      `econumo-setup-v1.shortcut` in a new window (from the home-screen app the
      Safari sheet closes back to the page, no relaunch needed) 📱.
- [ ] iOS only 📱: "Configure on this iPhone" mints an ingest PAT (visible under
      Profile → Tokens with scope `ingest`), opens the Shortcuts app with the
      Setup shortcut prefilled and ticks step 3 (folded to its title); a user
      who already holds an `ingest` PAT sees step 3 ticked on load. "Configure
      manually" opens `https://econumo.com/docs/user-guide/apple-wallet` in a
      new tab (no in-app token/recipe panel).
- [ ] iOS only 📱: Configure with an `ingest` PAT minted meanwhile from another
      device (page rendered before it existed) revokes that token first and
      mints a fresh one — Profile → Tokens shows exactly one live `ingest`
      PAT afterwards; `full` PATs are untouched.
- [ ] Desktop: an "Open this page on your iPhone…" hint sits above the list and
      the iOS-only buttons ("Configure on this iPhone", "Run econumo-wallet-v1")
      are absent; downloads, "Configure manually", both "Check" buttons and the
      hand ticks still work.
- [ ] Step 4 📱 iOS only: "Run econumo-wallet-v1" opens
      `shortcuts://run-shortcut?name=econumo-wallet-v1`; after allowing the
      prompts on the phone, "Check" ticks step 4 (folded to its title) and the
      `account is required` row disappears from the queue's "Needs attention"
      list; "Check" with nothing received reports "Nothing received yet…" and
      leaves the box unticked.
- [ ] Step 5 text walks through Automation → + → Wallet → cards & categories →
      Run After Confirmation → econumo-wallet-v1; step 6 text says to tap Run
      when the automation asks and Always Allow for Wallet access.
- [ ] Step 6: with no cards on the source its "Check" reports "No payment
      received yet…" and leaves the box unticked.
- [ ] The first card arriving from Apple Pay ticks steps 1–6 by itself,
      regardless of which were ticked by hand; step 7 (Switch the automation
      to Run Immediately) stays a hand tick and is the only open step left 📱.
- [ ] With all seven steps done the list collapses to "Setup complete" + "Show
      steps"; "Show steps" expands the ticked list (titles only), "Hide steps"
      collapses it again 📱.
- [ ] Ingest with an `ingest`-scoped PAT: `POST /api/v1/import/ingest-apple-wallet-event`
      → `status: queued` for an unmapped card; the card appears in the list as
      "Unmapped · 1 queued", tap count and last-seen date update per event; a
      `full` PAT / session token is accepted too; an `ingest` PAT on any other route
      is 401.
- [ ] Same payload twice → `duplicate`, no second row; a body without `account`
      or with a bad currency → `status: failed`, row in "Needs attention" with the
      error text and the raw payload; Retry re-parses (toast with the outcome),
      Discard removes it.
- [ ] Map card → account (owned accounts only in the picker; shared accounts
      absent): the queue replays — toast "N imported, N matched, N skipped";
      imported transactions appear on the account with the glyph; a same-amount
      hand-entered transaction within ±3 days is adopted (no duplicate) and shows
      the provenance card.
- [ ] A new card whose first taps were all in a foreign currency (USD taps →
      CAD account) maps without error; its queued taps import converted at the
      day's rate (a tap with no stored rate stays queued), and the provenance
      line keeps the original "… USD" amount. An ignored card offers
      "Map instead"; "Unmap" (confirmation) returns the card to unmapped and
      new taps queue again.
- [ ] Review banner 📱: with queued rows, every page except the queue shows
      "N imported transactions are waiting for review" + "Review"; the banner
      disappears when the queue empties.
- [ ] Queue page 📱: rows grouped by card, unmapped cards carry "Map to account"
      (→ Apple Wallet page) and "Ignore"; tapping a row opens the add-transaction
      dialog prefilled (account, amount, merchant as description, posted date);
      saving posts `import-queued-event` — the row leaves the queue and the
      transaction is created with the glyph; Skip moves a row to "Skipped",
      Restore brings it back.
- [ ] A second tap with the same amount on the same card within ±3 days of a
      hand-entered transaction of that amount is adopted (no duplicate); a tap
      already linked from this source is never adopted twice.
- [ ] Rate limit: the 61st ingest within the window from one user is 429 with the
      frozen envelope.
- [ ] A tap whose merchant name exceeds 255 characters is imported with the
      name cut to 255 (no failed row).
- [ ] Manually importing a queued row whose card currency differs from the
      chosen account's opens the dialog with an EMPTY amount and a "Card
      amount: … EUR" line; a same-currency row is prefilled.

## 5b. Imports — SimpleFIN

Preconditions: a SimpleFIN Bridge account with at least one linked bank and a fresh setup token.

- [ ] Settings → SimpleFIN on a device with no key: the connect form asks for a
      setup token and a passphrase (+ repeat); a passphrase under 8 characters or
      a mismatch is rejected inline before any request. 📱
- [ ] Connect: after "Connect" the page shows the bridge's accounts as Unmapped
      rows, the source row reads "Never synced", and the network log shows
      `create-source` carrying `credentialCiphertext` starting `v1:` — the access
      URL appears in no request other than `list-external-accounts`/`sync-source`
      bodies.
- [ ] A used/invalid setup token shows the server's error inline; nothing is
      created (Settings → SimpleFIN still shows the connect form after reload).
- [ ] Second device (or same browser after "Forget this device"): the page shows
      the unlock prompt; a wrong passphrase reads "Wrong passphrase."; the right
      one lists the accounts. 📱
- [ ] "Forget this device" returns the page to the unlock prompt; "Reconnect"
      from the unlock prompt with "I forgot my passphrase" ticked accepts a new
      setup token + new passphrase and replaces the connection (account mappings
      kept).
- [ ] After that passphrase reset, a device still unlocked under the OLD
      passphrase opens Settings → SimpleFIN to the unlock prompt reading "Your
      passphrase was changed on another device…" (not the reconnect form); the
      new passphrase unlocks it and Sync now works. 📱
- [ ] Map a bridge account to an owned account: the queue replays immediately and
      toasts "{n} imported, {m} matched, {s} skipped"; the imported transactions
      carry the Imported badge and their provenance sheet names the SimpleFIN
      source.
- [ ] Mapping a bridge account onto an account in another currency (USD bank →
      EUR account) is refused with the "The card's currency does not match the
      account's currency." error.
- [ ] "Sync now" pulls new transactions for a mapped account; a fully completed
      run toasts "{n} imported, {m} matched" (a partial or failed run shows no
      toast — the run summary card is the only record of it).
- [ ] Sync with a "From" date after today, or a range longer than 400 days, is
      accepted by the form (there is no client-side check) but rejected by the
      server (`import.sync_range_invalid`) and surfaced as a toast; "From"
      defaults to 3 days before the last sync (30 days back before the first).
- [ ] Sync twice with the same window: the second run reports 0 imported, 0
      matched (exact duplicates are skipped) and the last-synced timestamp
      advances.
- [ ] An Apple Wallet tap transaction later confirmed by SimpleFIN's posted
      version of the same purchase (same merchant tokens, within the
      tip-tolerance window) is adopted and its amount corrected to the posted
      value ("amounts updated" count > 0) rather than creating a second
      transaction — needs Apple Wallet connected on the same account too.
- [ ] A hand-entered transaction with the exact same amount as an incoming
      bridge row, dated within a few days of it, is adopted (matched count)
      rather than duplicated.
- [ ] Unmapped bridge account with transactions: sync queues them (queue page
      shows them with "Card not mapped" reason); mapping the account replays
      the queue.
- [ ] Per-account failure (one linked account's transaction write errors
      mid-sync while another account succeeds — not triggered by a missing
      rate or a deleted account, both of which queue their events instead):
      the run shows "Completed with errors", the failing account's error is
      listed under the run summary, other accounts' rows still import.
- [ ] Bridge unreachable: "Sync now" toasts the server's "try again in a few
      minutes" message, the run list shows a Failed run, and the failed state
      stays on the page until the next successful sync.
- [ ] Access URL revoked in the bridge (or the connection deleted there): "Sync
      now" toasts the "access URL is no longer valid, reconnect" message rather
      than the unreachable one, so the user reconnects instead of retrying.
- [ ] A run where some bridge rows cannot be parsed reports "Completed with
      errors" with a non-zero failed count and no success toast (never a clean
      "Completed"); the rows are listed on the queue page's needs-attention
      list.
- [ ] Run detail names each row's bank account the way the run summary does
      (the bank's own account name, falling back to the bridge id) — never a
      bare `ACT-…` id when the source's accounts are known. 📱
- [ ] Settings → Data: "Sync bank connections" is absent without a SimpleFIN
      source; with one and a locked device it navigates to Settings →
      SimpleFIN; unlocked it syncs every pull source and toasts the totals; the
      row is disabled (not clickable, dimmed) while the syncs run. 📱
- [ ] Settings → Data → Import history lists runs newest first with status,
      counts and errors; a run opens its detail; a transaction deleted after
      import shows struck-through with "Deleted since"; queued rows read
      "Waiting for review". Rows have no actions in this version. 📱
- [ ] Rate limits: the 6th `claim-setup-token` within 15 minutes and the 11th
      `sync-source` return 429 with the standard envelope.
- [ ] Ingest-scoped PATs get 401 on every SimpleFIN endpoint; a read-only
      (trial-ended) user gets 402 on `claim-setup-token`, `set-credential-key`,
      `sync-source`.
- [ ] Apple Wallet regression: §5a still passes unchanged (the `cards` list,
      queue, and provenance UI share code with the SimpleFIN account list).

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
- [ ] After creating a rule from a transaction and applying it (that source row
      is counted as "skipped — you edited it"), re-open THAT transaction and
      save a notes-only change: still no prompt, and no second copy of the rule
      is ever offered. Changing its category again does prompt.
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
- [ ] Same flow on a rule that sets two or more labels: add a THIRD label to an
      imported transaction and choose "Update rule" — the rule keeps its
      original labels and gains the new one (the label set is unioned, never
      replaced by the single added label).
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
- [ ] Typing in either rule editor fires `preview-rule` on a 300 ms debounce and
      it is capped per user (`ECONUMO_RATE_LIMIT_PREVIEW_RULE`, default 120 per
      window): set it to 1 and the second preview returns 429 with the standard
      envelope while the rest of the editor still works.
- [ ] Ingest-scoped PATs get 401 on every rule endpoint; a read-only
      (trial-ended) user gets 402 on `create-rule`/`update-rule`/`delete-rule`/
      `apply-rule`/`suggest-rules` and 402 on `preview-rule` as well, and 200
      on `get-rule-list`.

## 6. Recurring transactions

- [ ] 📱 Create a recurring rule (from a transaction's "make recurring" and
      from Settings → Recurring): type, amount, schedule, accounts, category.
- [ ] Due occurrence appears on the account page as "not posted". Post from the
      account-page row preview posts immediately (dated today); Post from
      Settings → Recurring opens a pre-filled review dialog that you confirm.
      Both advance the schedule. Skip (advances without posting) is offered
      ONLY on the account-page row preview.
- [ ] Post an OVERDUE occurrence (schedule date in the past) both ways: the
      created transaction is dated TODAY, not at the missed date, and lands in
      today's group. The review dialog's date chip likewise pre-fills today —
      a template still ahead of schedule keeps pre-filling its scheduled date.
- [ ] 📱 On an account with several upcoming templates, scroll up into the
      future block, then Post (and separately Skip) a template row in view
      from its preview: the list stays where it was — it does NOT jump to
      the top of the future block where the next occurrence now sits.
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
      totals (sidebar total).
- [ ] Change profile default currency (Settings → Profile) → totals update.

## 9. Budgets — table & plan

- [ ] 📱 Create a budget: name, currency, ≥1 owned account required; appears in
      the sidebar; becomes default when first/chosen.
- [ ] Budget table: budgeted / spent / available columns; expanding an element
      shows details; totals row; uncategorized and labels sections appear with
      info notes when relevant.
- [ ] 📱 Budget view (desktop and tablet): Available is the one bold figure,
      plain when zero or above and a red pill when below zero; there are no
      green pills. Spent and the thin bar under each row are gray, and turn
      red only when the month spent more than its budget and earlier months
      do not cover it (Available below zero). A row whose earlier months
      overspent but which is within this month's budget shows a red
      Available and a gray Spent. The bar fills against budget plus what
      earlier months left. A future month shows "—" for Spent and no bars.
      Folder headers are a tinted band with bold totals; the budget name is
      in normal case.
- [ ] Element visibility rule: a category/tag/envelope with **either** spending
      or a limit (incl. carried over) is visible; with neither it is not.
- [ ] Set a limit via the inline editor on a budgeted amount (desktop) or the
      item sheet's "Set budget" (tablet and phone); formula input; limit shows
      immediately and carries into the next period per rules.
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
- [ ] 📱 The budget header shows no currency chips on /budget or /plan, and
      no "Spending progress" widget appears anywhere on the page.
- [ ] 📱 Budget view (desktop and tablet): no amount carries a currency
      symbol; the budget currency code (e.g. "USD") sits once at the left of
      the column headings. An envelope in another currency shows a small code
      tag next to its name ("Travel EUR") and its amounts in that currency;
      folder lines and the Total stay in the budget currency. Items outside
      any folder are listed under "No folder". In Edit structure mode the
      folder "+" buttons and the row menus keep every amount column aligned.
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
      scrolling; hide-empty-rows toggle; the Transfers line appears only when
      money crossed the budget boundary in a visible month and shows the in/out
      split as a tooltip; the Balance row shows tooltips. No amount carries a
      currency symbol: the budget currency code sits at the left of the month
      header, and a row in another currency has a code tag next to its name.
- [ ] 📱 Plan grid: past months and the current month show the small actual
      above the plan; future months show only the plan. A month with no plan
      is blank (not 0.00) and clicking it still opens the editor. An expense
      actual is red only when it is over the plan; nothing turns green. The
      current month's column has a light tint.
- [ ] Plan grid (desktop): clicking a month's actual opens that month's
      transactions for the row, the expense Uncategorized row included; a zero
      actual, a future month, and an income or savings row are not links. 📱
      On a tablet a tap on the expense Uncategorized cell with spending opens
      its transactions (other cells open the item sheet).
- [ ] 📱 **Budget settings — Accounts and Savings accounts pickers**: the
      create and edit budget dialogs show Name, Currency, then two picker rows
      (styled like Currency): "Accounts" (the included accounts' names, and
      "N of M included") and "Savings accounts" (the savings accounts' names,
      or "None"). The dialog stays that size with 30 accounts. Tapping
      Accounts opens a searchable checklist (search from six accounts up;
      accounts in hidden folders last, under "In hidden folders"): tapping an
      account checks/unchecks it, OK closes and the row updates. A member with
      past transactions shows checked with a lock icon, cannot be unchecked,
      and the list ends with "Accounts with transactions in past months can't
      be removed". Tapping Savings accounts opens the same kind of checklist
      over the INCLUDED accounts only ("Money moved into these accounts counts
      as saved, not spent." above, "Savings accounts are shown by name, with
      their saved amounts and balances, to everyone with access to this
      budget." below). With nothing included the Savings row is disabled and
      reads "Include an account above to mark it as savings."; excluding a
      savings account also drops it from Savings accounts. The savings role
      can be changed at any time — when creating the budget, or later on an
      existing (even locked) member. Escape (desktop) closes only the picker,
      not the budget dialog. On a 320px phone in German or Ukrainian the rows
      truncate long name lists with "…" and the pickers open as bottom sheets.
- [ ] 📱 Turn off (or remove) a savings member that still carries plans or
      comments in that budget: saving asks "Delete planned savings?" —
      "Planned amounts and comments of the savings accounts you turned off or
      removed will be deleted from this budget. Saved amounts and
      transactions are not affected." Cancel closes the confirmation; the
      settings dialog stays open with your edits and nothing is saved — the
      account keeps its savings flag, plans and comments,
      unchanged; "Delete plans" resends the same edit and it goes through,
      deleting that budget's plans and comments for the account (its
      transactions and balance are untouched). A savings member with no plans
      or comments in the budget toggles off or removes without asking.
- [ ] The same account can be a savings member of one budget and an everyday
      member of another: flip it to savings in Budget A only — Budget A shows
      its Savings row/section/block, Budget B keeps it as an ordinary budgeted
      account, and neither budget's plans/figures affect the other.
- [ ] In a budget shared with another participant (any role), that
      participant's own copy of the budget settings dialog lists only their
      own accounts — an account you own never appears there, so another
      participant has no switch to flag or unflag your account as savings,
      regardless of their role.
- [ ] 📱 Deposit into a savings account (e.g. in January), give it no plan,
      then delete it (e.g. in June). The deletion writes a correction that
      zeroes its balance from the deletion month on, so view a Plan window
      that ends BEFORE the deletion month and has no activity on the account
      (e.g. one over March): the account drops out of the Plan sheet's
      Savings section and the phone month view's Savings rows entirely (no
      row), yet
      its balance still counts as Total savings, not everyday Balance — the
      split still sums to the Balance.
- [ ] 📱 **Plan sheet — Savings section**: with a savings account in the
      budget, a "Savings" section appears below Expenses and above Archived,
      one row per savings account in their saved order; a budget without
      savings accounts shows no such section.
- [ ] 📱 Fold the Savings header: its rows hide, and stay hidden after a
      reload; unfold brings them back.
- [ ] Plan sheet keyboard: ArrowDown from the last expense row lands on the
      first savings row, and from the last savings row on the first archived
      row; with Savings folded it skips straight to Archived.
- [ ] 📱 Edit a savings row's planned amount (popover on desktop; on a tablet
      or phone the item sheet's "Set budget"): the new value shows at once and
      survives a reload.
- [ ] Fill-right a savings planned amount (drag handle and Shift+Arrow): every
      covered month gets the value.
- [ ] 📱 Edit structure mode: savings rows reorder by drag among themselves
      only (the order survives a reload); a savings row cannot be dropped
      into a folder or the Income/Expenses area, and its row menu has no
      "Move to folder…" (Change currency is still there).
- [ ] 📱 A deleted savings account stays in the Savings section, read-only (no
      amount editor, no drag grip), only while it still has a plan or actual
      activity in the visible period; once neither remains it drops out.
- [ ] An everyday→savings transfer counts toward "Saved" (the savings row's
      Actual, and the monthly block's Saved column); a savings↔savings
      transfer and a transfer with an account that is not a budget member do
      not move it either way.
- [ ] 📱 Totals: a "Savings" line appears below Transfers (actual for past
      months, the larger of actual and planned for the current and future
      months); without savings accounts the line is absent.
- [ ] 📱 Balance split: the sticky area shows "Balance" (everyday accounts) and
      "Total savings"; for every month the two add up to the single
      Balance the same budget showed before its savings account was marked
      savings. Without savings accounts only "Balance" shows, unchanged.
- [ ] 📱 Two savings accounts in the current month, one planned 500 with
      nothing saved yet, the other planned 0 with 300 saved: the Savings line
      shows 800 and the Total savings rises by exactly 800 over the previous
      month (Balance drops by the same). A transfer already booked into a future
      month above that month's plan counts at its booked amount in both.
- [ ] 📱 Book a future-dated transfer of 20 from a savings account to an
      account that is not in the budget: from that month on the Savings
      balance is 20 lower, and the everyday Balance is the same as before the
      transfer.
- [ ] 📱 A savings account that saved more or less than planned: its actual
      stays plain (never red or green).
- [ ] 📱 Edit a savings account's balance (the correction transaction), then
      switch to the Plan view in the same tab: the Total savings updates
      without a manual reload.
- [ ] 📱 "Total savings" is a plain label with no info icon. Record
      interest on a savings account: the Total savings rises by it while the
      Savings line does not — intended, not a bug.
- [ ] 📱 A savings cell carries comment threads like any other cell: the
      corner marker shows on a commented cell, and Shift+Enter (desktop) or a
      tap on the marker opens its thread.
- [ ] 📱 Each savings cell shows, under the planned amount, the account's
      balance at the END of that month (hover: "Balance at the end of the
      month…"), in the row's currency: a past month shows the actual booked
      balance; the current month shows the balance so far plus the plan not
      yet met (planned 200, saved 50: 150 higher than booked); a future month
      adds its own unmet plan on top (planned 200 more: 350 higher). A month
      saved over plan adds nothing. It matches the "Balance at month end" of
      that account's item sheet on a phone for the same month. Expense and
      income cells carry no such line.
- [ ] 📱 Save into a TFSA all year, then transfer the lot to an RRSP (both
      savings accounts in the budget): neither row's Saved / Actual moves, the
      Total savings is unchanged, and from that month on the TFSA row's
      balance is that much lower and the RRSP row's that much higher.
- [ ] **Budget view (desktop and tablet) shows no savings**: with a savings
      account in the budget, no Savings block appears and the Total row counts
      the expense categories only (its Budget, Spent and Available match the
      folders' sums). Savings are planned and tracked in the Plan view and, on
      a phone, in the month view.
- [ ] 📱 Phone month view, Savings unfolded: one row per savings account with
      Planned and Saved (the row's item sheet adds Balance at month end); at
      320px and 375px, also in German, Polish and Ukrainian, a long account
      name truncates rather than pushing the amounts, and a five-digit amount
      such as 12,345.67 fits without overlapping its neighbour.
- [ ] 📱 A savings account's Balance at month end (phone item sheet; the Plan
      view's line under each savings cell) is its balance at the end of the
      selected month: a past month
      shows the actual booked end-of-month balance; the current month shows
      the current balance plus the plan not yet met this month; a future month
      also adds every unmet plan from the current month through it. Saving
      more than planned in a month adds nothing (and does not cover another
      month's shortfall). A deleted savings account shows its booked balance.
- [ ] 📱 Change a savings account's planned amount in the current month (Plan
      view, or the phone item sheet): its Balance at month end moves by the
      change at once; then open a later month — its Balance reflects the new
      plan too (no manual reload).
- [ ] Plan view, Edit structure mode: savings rows show drag grips (a
      deleted account's row has none) and reorder among themselves only; the
      order survives a reload and is the order of the phone month view's
      Savings rows.
- [ ] 📱 A commented savings cell in the Plan view carries the corner marker;
      clicking (tapping) it opens that cell's thread. A comment posted there
      shows in the phone month view's item sheet for that account and month.
- [ ] **Budget cell comments** 📱: post a comment on a plan cell; it appears
      immediately and survives a reload.
- [ ] 📱 Open the same cell in the monthly view for that month: the comment is
      there (cross-view sync).
- [ ] 📱 A comment's time reads short, like "Sep 29, 6:42 PM" (no seconds),
      in the thread and in the hover preview; a comment from an earlier year
      also names the year.
- [ ] Edit your own comment: the text updates and "(edited)" appears.
- [ ] Desktop: a comment's pencil (edit) and bin (delete) icons appear only
      while the pointer is over that comment (or it has keyboard focus); 📱 on a
      phone or tablet they are always visible.
- [ ] Another participant cannot edit your comment; the budget owner can
      delete it.
- [ ] 📱 A guest (read-only role) can post, edit and delete their own comment.
- [ ] A cell with comments shows the purple corner marker; a cell without
      shows none (only the faint hover corner below); the uncategorized row
      never shows one.
- [ ] Desktop: click a cell's corner marker (Budget view, Plan view, a
      savings Planned cell): the thread opens in a popover beside that cell.
      Clicking another cell's marker switches to that cell's thread.
      Opening a thread from the keyboard (Tab to the marker, Enter) and
      pressing Esc closes it and puts focus back on the marker.
- [ ] Desktop: rest the pointer on a commented cell: after a moment a card
      previews its latest two comments (plus "+N more"); it disappears when
      the pointer leaves (it stays while the pointer is on the card itself),
      and never shows while the thread or the amount editor is open — not even
      when the pointer drifts off the open amount editor and back over the
      cell. Tabbing onto a cell's amount shows no card.
- [ ] Desktop: hover a cell with no comments (Budget view budgeted amount,
      Archive rows included; savings Planned cell; Plan view month cell): a
      faint grey triangle appears at its top-right, where the purple marker
      would be, and disappears when the pointer leaves. Clicking it opens the
      empty thread in a popover beside the cell (the amount editor does not
      open). A guest gets it too. It never appears on the uncategorized row,
      in edit-structure mode, on an archived budget or a month outside the
      budget's range, or on phones and tablets. Right-clicking a cell shows
      the browser's normal context menu.
- [ ] Desktop, monthly view: Tab to a budgeted amount (or a savings Planned
      amount) with no comments yet and press Shift+F2: that cell's thread
      opens in a popover beside it, ready for a first comment.
- [ ] 📱 Tablet (640–1023 px), Budget view: tap a budgeted amount (editable or
      not, Archive rows included) or a Savings planned amount: the item sheet
      opens ("Food · July": Budget, Spent, Available, the two latest comments,
      "Comments (N)" / "Add comment", "Set budget", "Transactions"). "Set
      budget" replaces the sheet with the amount dialog (no comments in it);
      "Comments" replaces it with the thread. A savings row's sheet has no
      "Transactions". A guest's sheet has no "Set budget". Available is not a button; tapping Spent still lists
      the transactions; the corner marker still opens the thread beside the cell.
- [ ] 📱 Tablet, Plan view: tap any month cell (not the Uncategorized row):
      the cell is selected and the item sheet opens for that element and
      month — Budget/Spent (Planned/Received for income, Planned/Saved/Balance
      for savings), the two latest comments, "Set budget" ("Set plan" for income),
      "Comments", and "Transactions" for expense rows. The sheet's month and
      any foreign-currency rate are the tapped column's. The corner marker still
      opens only the thread.
- [ ] 📱 Tablet: holding a finger on a cell opens nothing extra (no actions
      menu, no text-selection callout from the app) and the tap still works.
- [ ] 📱 Phone: a comment thread opens as a sheet whose composer stays pinned
      above the keyboard.
- [ ] On the plan grid, select a cell and press Shift+F2 (or Shift+Enter):
      its thread opens in a popover beside the cell; Esc closes it and the
      arrow keys keep moving the selection. Plain Enter on the same cell opens
      the amount editor, which has no comments section.
- [ ] On desktop, a non-editable cell (guest role, an archived element —
      even on a budget you can edit — or a month outside the budget's range):
      clicking its amount opens that cell's thread in a popover beside the
      cell, so a thread can be started there too.
- [ ] Double-click Post (or press Post then Cmd/Ctrl+Enter quickly): exactly
      one comment is created, and Post stays disabled until it lands.
- [ ] Post a comment, then start typing the next one before the first lands:
      the new text stays in the composer.
- [ ] Archive the budget: comment threads are readable, the composer is gone.
- [ ] Reset the budget (REST route only — there is no UI for reset): planned
      amounts AND comments are cleared.
- [ ] Clone a budget with plans: comments at or after the start month come
      across with their original authors; cloning without plans copies none.
- [ ] Merge two categories: the source cell's comment thread appears on the
      target cell.
- [ ] Revoke a participant: their comments on surviving cells still render
      their name.
- [ ] Budget with accounts in two currencies: per-currency balances section is
      correct.
- [ ] Rates loaded by `currency:update-rates` (or the in-process updater) are
      applied, on SQLite AND PostgreSQL: an expense from a foreign-currency
      account in a budget-currency category counts in the category's spent at
      the converted amount (not 1:1), in both the table and the plan sheet, and
      `get-budget` `currencyRates` lists the global rates, not only custom ones.
- [ ] Transaction dated exactly 00:00 on the 1st of the budget month (e.g. a
      date-only CSV import row), on SQLite AND PostgreSQL: it counts ONCE, in
      that month's income/expenses, not also in its starting balance; every
      month's starting balance equals the previous month's ending balance.
- [ ] SQLite instance upgraded from a release before this fix: after the first
      boot, account balances and transaction lists (dates included) match the
      pre-upgrade figures. Include transactions imported from a CSV whose date
      column carried an RFC3339 offset (e.g. `2024-04-10T10:00:00+03:00`):
      they list and export at the UTC time after the upgrade instead of
      failing the list.

### Phone month view 📱

- [ ] 📱 On a phone (< 640 px, the iOS home-screen PWA included) `/budget` and
      `/plan` show the same single month view: header with the budget name in
      normal case, the month strip, and no Budget/Plan switch in the settings
      menu. At the top, one card under a "USD · Planned · Actual" heading
      (the budget's currency code at the left) holds an
      "Income" line (planned, received) and a "Savings" line (planned,
      saved, in the budget currency); below it, after a wider gap,
      "Expenses · Budget · Spent" heads the expense folders.
- [ ] 📱 The Income and Savings lines start folded; tapping one unfolds its
      rows inside the same card, and the fold state survives a month switch
      and a reload.
- [ ] 📱 Each expense row shows Budget and Spent with no currency symbol and a
      thin bar under the row. Spent and the bar turn red only when the month
      spent more than its budget and money left from earlier months does not
      cover it; otherwise they stay gray. The bar measures spending against
      the budget plus what earlier months left. A future month shows `—` for
      Spent, with no bar and no colour.
- [ ] 📱 What earlier months left shows read-only right before the budget, so
      the two read as one figure, e.g. "530.00 + 700.00" — on phone rows and in the Budget table (desktop and
      tablet, with a "Left from earlier months" tooltip); only the budget is
      editable. Nothing shows when nothing is left; an earlier overspend shows
      as a red negative amount ("-30.00 +").
- [ ] 📱 The Total row (desktop and tablet) leads its budget with the sum of
      what earlier months left, in the budget currency ("2,530.00 +
      3,725.00"); on a phone the Totals card's Budget line reads "left +
      budget". Savings and Uncategorized add nothing to it.
- [ ] 📱 Open September, then August; change a category's August budget (e.g.
      to 0), then go back to September without reloading: September's
      leftover for that category follows the change at once (it equals
      August's Available).
- [ ] 📱 A category/envelope in another currency carries a small code tag
      (`Travel EUR`) and its amounts are in that currency.
- [ ] 📱 Income is one collapsed header like a folder's: "Income", the
      planned total under Budget and the received total under Spent (— in a
      future month); tapping it unfolds
      the income rows (Planned / Received), and the fold state survives a
      month switch. The income Uncategorized row, and an archived income
      category, appear only in a month that received something, so the rows
      always add up to the received total; an archived row's sheet has no
      "Set plan".
- [ ] 📱 Tapping a row opens its item sheet ("Food · July"): Budget, Spent,
      Available, each centred in its column; "Over by … — covered by … left
      from earlier months" when carry-over covers an overspend (no sentence
      when Available is negative: it shows in red); the two latest comments
      (author and text) and "Comments (N)", or "No comments yet." in gray with
      "Add comment";
      "Set budget" and "Transactions". A foreign-currency item adds its code to
      every amount, the amount in the budget currency, and the month's average
      rate.
- [ ] 📱 From the sheet, "Set budget", "Comments" and "Transactions" each
      replace the sheet (never stacked); closing them returns to the list.
      Saving a budget updates the row at once and survives a reload.
- [ ] 📱 An income row's sheet offers "Set plan" (Planned / Received); a
      savings row's sheet shows Planned, Saved and Balance at month end.
- [ ] 📱 A guest, an archived budget, a month outside the
      budget's range, an archived element and the Uncategorized row get no
      "Set budget" in the sheet; Uncategorized has no comments link, and an
      empty read-only thread shows none.
- [ ] 📱 The Totals card lists, in order: Budget ("left + budget", with
      "… available" under it, red when negative; expenses only), Income
      (received), Expenses (spent), Transfers (only when money
      crossed the budget boundary that month), Savings (saved, only with
      savings accounts), Total savings (with savings accounts), Balance at
      month end. A future month shows "—" for received, spent and saved. Balance and Total savings match
      the Plan view's figures for the same month on a desktop, including a
      month several months ahead of the current one whose earlier months
      carry unmet plans.
- [ ] 📱 Children of an envelope/tag unfold from the chevron; tapping a child
      or a reporting tag opens its transactions directly.
- [ ] 📱 "Edit structure" on a phone shows the route's structure editor — the
      budget table on /budget, the plan grid in edit mode on /plan (drag to
      reorder, folder menus); "Done" returns to the month view.

## 10. Budget lifecycle & list

- [ ] Budgets list (Settings → Budgets): set default, open, edit (name +
      accounts replace-set), delete with confirm.
- [ ] **Accounts membership**: adding/removing accounts in edit; an account
      with transactions inside the budget window since the start of the
      current month cannot be removed (clear error); hidden-accounts note and
      included counter are correct (never "N of M" with N>M).
- [ ] **Duplicate** (clone): name pre-fills with a localized "(copy)" suffix;
      deep copy with/without plans from a chosen start month (a savings
      account's Savings row carries over, and its planned limits carry over
      too when plans are copied); copy starts unarchived/open-ended;
      structure and sharing carried.
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
- [ ] **Change password**: wrong old password rejected. Changing the password
      from Settings signs out the other sessions, keeps this one and personal
      tokens, cancels a pending email change and any outstanding reset code,
      and a login with the old password that was already in flight gets
      "Invalid credentials."
- [ ] **Change email**: request (new email + password) → code sent to the new
      address → confirm; resend with cooldown; wrong code rejected; login works
      with the new email only. The heads-up notice to the OLD address is CC'd
      to the account's linked-provider addresses; the CODE to the new address
      goes to that address alone, with no Cc.
- [ ] **Sessions** 📱: list shows device descriptions, current badge, relative
      last-active; revoke one (other) session logs that device out; revoke-all-
      others keeps only the current; revoking the current session logs out.
- [ ] **Personal access tokens** 📱: create with expiry presets + custom date;
      token revealed exactly once with copy button; API call with the PAT
      works (e.g. `GET /api/v1/user/get-user-data`); revoked PAT stops working;
      list shows last-used/expiry.
- [ ] Analytics toggle (Settings → Profile → Privacy, the last group on the
      page): switching it off persists across a reload; log out and back in —
      the toggle still reads off; a read-only user (lapsed trial) can still
      flip it, unlike other writes on that account.
- [ ] Create a personal token with scope "full" — it works everywhere; an
      "ingest" token (created via "Configure on this iPhone" or the API) is
      rejected with 401 on every non-import route and accepted on
      `import/ingest-apple-wallet-event`.
      flip it, unlike other writes on that account. Its description renders on
      TWO lines (the reassurance about financial and personal data starts a new
      line), in every UI language.
- [ ] Analytics reach the collector for signed-in sessions only (DevTools →
      Network, filter `t.econumo.com`): the login/register pages load neither
      `twillingate.js` nor send any request; after login the SDK script loads
      once and every `ingest/events` body carries `$user_id`, `$consent: 0`,
      a detected `$os` and `$device` (`mobile` on a phone, `desktop` on a
      laptop, whatever the window width), and no `$install_id`; a reload of a signed-in page
      sends its page view once the user data has loaded; log out — the logout
      event goes out, nothing after it. With analytics switched off in
      Settings, a reload loads no `twillingate.js` at all. DevTools →
      Application: no `twillingate_*` or `econumo_*` key in localStorage and no
      cookie from it.
- [ ] Page views are real views (same Network filter): navigating between
      pages sends events named `$page_view` (not `page_view`) whose `$host` is
      `app.econumo.com` on the cloud and `selfhosted_<id>` on a self-hosted
      instance, whose `$path` has every UUID replaced by `:id`
      (`/account/:id`), and which carry no `$referrer`.
- [ ] System and session keys ride every event, view or product (same
      Network filter): each entry in `events` carries `$host` (as above),
      `$path` (the page that event happened on, UUIDs as `:id` — it differs
      between events in one batch when you navigate mid-flush), `$app_locale` (switch the UI
      language and the next event carries the new code), `access_state` and
      `deployment`; none carries `host`, `locale`, `mode` or `current_url`.
      A product event also carries its own data where it has any (merging a
      payee sends `type: payee`). `window.dataLayer` is undefined in the
      console.
- [ ] Profile facts are totals (same Network filter, in each event's
      `attributes`, once the relevant page has loaded its list): `accounts`,
      `categories`, `payees`, `tags` and `labels` count hidden and archived
      items too (archive one → the count does not drop), `budgets` counts
      archived budgets but not an unaccepted invite, `months_since_signup` is
      the whole months since registration (`0` in the first month), and no
      `accounts_hidden`, `*_archived` or `signup_month` key is sent.
- [ ] Auth-method flags say which sign-in methods the user HAS (same Network
      filter, in each event's `attributes`):
      `auth_password`, `auth_google`, `auth_apple`, `auth_sso` are each `on`
      or `off`. A password account with Google linked sends
      `auth_password: on`, `auth_google: on`, `auth_apple: off`,
      `auth_sso: off`; an OAuth-only account (never set a password) sends
      `auth_password: off`. They are present from the FIRST batch after
      sign-in without opening Settings, and survive a reload. Link a provider
      in Settings → its flag flips to `on` on the next event; unlink it →
      back to `off`. The custom OIDC provider reports as
      `auth_sso`. Log out and sign in as someone else → the flags describe the
      new user, never the previous one's.
- [ ] 📱 Sign-in methods (Settings → Profile → Sign-in methods): lists every
      linked provider with its email and linked date; linking an unlinked
      provider goes through the provider flow and returns with a "linked"
      toast AND the newly linked provider already in the list (no manual
      reload — the return trip is what writes the link) AND the owner's notice
      email naming the provider (console transport prints it to server stdout
      in dev); linking a SECOND
      provider straight afterwards, without leaving the page, works the same
      way (📱 especially in the app, where the deep link returns to the same
      screen); unlinking a provider (with confirm dialog) removes it from the
      list AND emails the owner a notice naming the unlinked provider.
- [ ] Linking a provider account that is ALREADY linked to this same Econumo
      account again (unlink then relink is a fresh link, so use a second pass
      through the flow while it is still linked) succeeds without sending a
      second notice email — only a newly gained sign-in method is announced.
- [ ] Unlink a provider while a sign-in through it is mid-flight (callback
      done, handoff not yet exchanged): the exchange fails with the
      sign-in-link-invalid error and no session is opened.
- [ ] Unlink is refused for a passwordless user's last remaining identity
      (button disabled, hint text shown: "Set a password before unlinking
      your only sign-in method."), including two unlink requests sent at the
      same time — one succeeds, the other is refused. A refused unlink sends
      no notice email; the concurrent pair sends exactly one.
- [ ] Both identity notices (linked and unlinked) arrive in the ACCOUNT's
      stored language, not the language of whoever triggered the flow: set
      the UI language to e.g. Russian, link and unlink a provider, and check
      both emails are Russian.
- [ ] Both identity notices are CC'd to the account's linked-provider
      addresses. With two providers linked under different addresses, unlink
      one: the notice goes To the account email and Cc's the address of the
      provider that is STILL linked — the just-unlinked address is not copied,
      and no address ever appears twice.
- [ ] A password reset on an account with a provider linked under a DIFFERENT
      email unlinks that identity (the reclaim) and sends NO unlink notice —
      the reset itself is the announcement.
- [ ] For a passwordless user, Settings → Profile shows a "Set a password"
      row in place of "Change password"; it sends a reset code to the
      account's email (the email field pre-filled/locked) and, after
      entering the code and a new password the app signs the user out (the
      reset ends every session, including this one) and lands on the login
      page; signing in with the new password works and Settings → Profile
      now shows "Change password" instead of "Set a password".
- [ ] Sessions list (Settings → Profile → Sessions) shows "via Google" (or
      Apple/SSO) under a session opened through a provider; a password
      session shows nothing extra.
- [ ] On an SSO-only (passwordless, single-identity) account, signing in
      again after the provider reports a changed email updates the account's
      stored email to match; the same drift on an account that has a
      password, a second linked provider, or where the new email already
      belongs to another user leaves the stored email unchanged.

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
      appear (simulate with a binary built at an older version — Docker
      `--build-arg ECONUMO_VERSION=v0.0.1`; the runtime variable no longer
      moves this, it only relabels the UI).
- [ ] Version label: with `ECONUMO_VERSION=demo-42` set at RUNTIME, the
      sidebar footer and the settings version row both read `demo-42`, while
      the update notice above still compares the real binary version (so a
      current build shows no update prompt).
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
