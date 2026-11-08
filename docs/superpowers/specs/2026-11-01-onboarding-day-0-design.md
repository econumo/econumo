# Onboarding redesign: Day 0

## Why

The goal of onboarding is to get a new user entering transactions regularly, so they
see their budget and plan. Onboarding is a sequence of steps over 45 days, not a
configuration wizard. At day 45 a successful user has logged transactions through a
full month, seen that month against a plan, and planned three or more future months.

This document specifies Day 0, the first piece of that sequence. The later pieces are
listed under "Later work" so Day 0 does not block them.

Twillingate data (project 9, new users from 2026-09-09 to 2026-10-19, about 40 users,
a few of them probably test accounts):

- 9 returned more than a day after their first event, none more than 7 days after it.
- 5 finished the current six-step onboarding page.
- 9 created a budget.
- About a third touch currencies in their first actions, often before any account.
- One CSV import, three invites sent.

People set things up, add a few transactions by hand, and leave. Few of them reach a
budget, and their real balances and history rarely get in.

## Goals

- A new user reaches a working budget, with real balances, within a few minutes.
- Currency stops being a stumbling block.
- The next useful action after setup fits the device the user is on.

## Non-goals

- Inviting someone, and the invited person's path (see "Later work").
- In-product guidance for creating a budget. Dmitry postponed it.
- Guessing a transaction's payee and category. It gets its own spec.
- Emails. They live outside the Econumo app (see "Later work").

## Anti-goals

Day 0 has failed, even if it works, if:

- it asks for past history before the user gets anything useful;
- it forces a transaction on someone sitting at a desk during setup, because
  transactions are normally added later, on the go;
- it asks users to get categories right before they can continue;
- it adds a top banner (that slot is reserved, see CLAUDE.md);
- it changes the balance of an account because of an import the user did not ask to
  balance.

## The flow

There is one path for everyone, whether they budget alone or plan to invite someone
later. There is no "How will you use Econumo?" screen in this version. Someone holding
an invite code uses the existing accept-invite option on the Connections page and also
gets their own budget from this flow.

### Step 1: Accounts as of today

An inline list headed "Where's your money right now?", with the subtext that today's
balances are enough and no history is needed.

- Each row is an account name and a balance. Filling in a row makes the next empty row
  appear. Rows can be removed before continuing.
- The icon is picked automatically from the name (cash, savings, card as the default).
- Each row shows a currency, guessed from the user's location, changeable by tapping.
- Each row has a **Savings** switch, switched on automatically when the name looks like
  savings, in the user's language. The user can flip it.
- At least one account is needed to continue. A blank balance counts as 0.
- There is no import on this screen. Import writes to the server immediately and creates
  categories too, which would bypass the checklist and enter the budget. Import is offered
  after setup, on the budget page, where Keep current balance protects the typed balances.

Below the list, a **Currencies** block:

- Every currency the rows use is selected and cannot be removed while a row uses it.
- "+ Add currency" searches for extras, for example EUR for travel.
- "Main: USD · based on your accounts" shows the main currency, which is the one most
  accounts use. Tapping changes it.
- A note says more currencies can be added later in Settings.

When the step completes, only the selected currencies stay shown for this user (the
existing per-user `hide-currency` / `show-currency` behaviour), so every later currency
picker is a short list. The main currency is saved through the existing `update-currency`
behaviour.

### Step 2: Categories

A checklist with everything ticked; the user unticks what they don't need. Two groups:

- **Expenses:** 10 categories (draft: Groceries, Housing, Utilities, Transport, Eating
  out, Health, Shopping, Entertainment, Kids, Subscriptions).
- **Income:** Salary, Other income.

Each row can be renamed inline, and its icon changed by tapping it. Each group ends in
an inline "add your own" row, in the same style as the accounts list.

### Step 3: Budget (no screen)

When the user continues from categories, a budget is created automatically:

- it contains every account from step 1, with the Savings switches applied as budget
  savings membership;
- it contains the kept categories;
- it starts this month and uses the main currency;
- limits are empty;
- it becomes the user's default budget.

Onboarding is then complete and disappears. The user lands on the budget page.

### Skipping

Setup is not a requirement. Every step has a "Skip setup" link, and the rest of the app
stays reachable (the Connections page, for an invite holder). Skipping marks onboarding
complete and keeps whatever was already written: accounts entered so far stay, and no
budget is created. The user lands on the budget page with no budget, which shows the
existing empty state plus the same get-ready cards and Inbox next steps. Leaving through
the menu without skipping does not complete onboarding; the flow comes back on the home
page.

### The budget page after Day 0

While the budget has no data, the page shows "get ready" cards in place of the empty
budget. They are not banners. Which cards show depends on the device and the server:

| Device | Cards, in order |
|---|---|
| Phone | Add to home screen. Set up Apple Wallet import (iPhone only). |
| Desktop | Set your plan. Bring in transactions (CSV import). |

- Add to home screen: iPhone gets a short Safari "Share, Add to Home Screen" guide,
  Android gets the browser's install prompt. It is skipped when already opened from the
  home screen.
- The Apple Wallet card shows on an iPhone (there is no feature flag any more). CSV import is
  always available. SimpleFIN is unannounced and out of scope: no card, and Day 0 does not
  change it. A card that doesn't fit the device is not shown.
- A card disappears once done, and all cards can be skipped.
- There is no prompt to add a first transaction.
- The cards show only until the first transaction is added: "no data" means no
  transactions other than opening corrections (see below). Once a transaction exists, or
  every card is done or skipped, the normal budget page shows. The Inbox next steps stay
  until done or dismissed.
- The same items appear in the Inbox as next steps (`web/src/features/inbox`, fed by
  `useInbox()`), so skipping a card loses nothing, and someone who signed up on a desktop
  sees the phone items the first time they open Econumo on a phone.
- Next steps do not count toward the sidebar badge, because nothing is waiting on the user.
  Each can be dismissed for good. They show only to users who went through this flow, which
  a new `users_options` value records. Done and dismissed state lives there too, except add
  to home screen, which is per device.

## Import and the typed balance

A balance entered on an account is stored as a correction transaction dated today
(`internal/account/create.go`). Importing older transactions into that account would
then move today's balance.

The import form therefore gets one **Keep current balance** checkbox, applied to each
existing account the import touches:

- Corrections carry no marker, so an "opening correction" is recognised by shape: the
  account's only transaction, with no category, payee, tag or description.
- It applies to CSV import only. SimpleFIN is out of scope (unannounced), and Apple Wallet
  never needs it.
- Ticked by default when every existing target account holds nothing but its opening
  correction (or nothing at all), unticked otherwise. The user can change it either way.
- When ticked: each account's balance is read before the import. After the import, today's
  balance is restored:
  - an account in the opening-only state has that correction moved to just before its
    earliest imported transaction, with its amount recalculated, and removed if it comes to
    zero;
  - any other account, including one with no correction, gets a new correction for the
    difference just before its earliest imported transaction. Existing rows are never moved.
- The import result says so ("Kept today's balance of 2,340.00").
- CSV import uploads in chunks, so this runs as a finishing step once the upload ends. After
  a partial import it balances what landed and reports the failure. If the tab closes
  mid-import, no adjustment happens; that is accepted, and the balance can be corrected by
  hand.
- An import that creates a new account has no typed balance to keep.

An unticked import behaves exactly as imports do today. The balance is known to be true
only on Day 0, which is why the default depends on the account's state: a later import
that silently adjusted a correction could hide a real mistake, such as duplicates of
transactions typed by hand.

## Calls left to the builder (mine, not Dmitry's)

- How the Day 0 writes are sent (several existing endpoints, or one new one), provided a
  failure can't strand a user half set up and a retry can't duplicate accounts or budgets.
- The exact savings-name matching, icon matching and location-to-currency mapping.
- Layout and copy, within the mockups' shape.

## Decided by me, to check

- **Existing users:** once, at release, a migration marks onboarding complete for anyone
  not `completed` who already has an account. After that, only finishing the flow completes
  it, so the check does not re-run while someone is in the flow.
- **The base currency stays visible,** because the app refuses to hide it. A user whose
  accounts use another currency sees one extra entry in pickers.
- **Missing exchange rates:** the app converts 1:1 when a rate is missing. The Currencies
  block shows a short note when a selected currency has no rate, so a self-hosted instance
  without rates doesn't mislead silently. Conversion itself is not changed here.
- **The `/onboarding` route and nav link stay** for users in the flow, and go away on
  completion, as they do today. The old six-step page is removed.
- **"Has no data"** on the budget page means no transactions in any member account.
- **No AI in Day 0** (Dmitry): all lists are static, and no payees are created.
- **Fixed category names** come from the `locales/` catalogues, so they are translated in
  all 11 languages.
- **Success measure:** the share of new users who log transactions on 3 separate days in
  their first week, alongside Day 0 completion and return rates.

## Constraints from the project

- `make go-test` and `pnpm test` pass. New routes get apiparity scenarios and goldens, and
  transaction writes keep `transactionsRevision` correct (`internal/test/revisionguard`),
  including moving and resizing a correction.
- Both SQLite and PostgreSQL behave identically.
- Every new user action fires an analytics event through `METRICS`.
- All strings go in every `locales/<lang>.json`.
- `docs/regression-test-plan.md` is updated in the same PR, with accurate 📱 markers.
- CLAUDE.md is updated where it describes onboarding or imports.
- Works on phone, tablet and desktop, and in the iOS home-screen PWA, which is in daily use.

## Later work, agreed in outline

**The rest of the 45-day journey,** as Inbox items:

- Days 1 to 7: optional next steps (invite someone, CSV import, Apple Wallet, connect an
  AI assistant through MCP) and logging progress ("logged on 3 of 7 days").
- Day 7: an AI plan suggestion, only when AI is configured and no plan is set. Without AI
  there is no suggestion at all, and no rule-based fallback.
- Month end: a review of spending against the plan, plus a balance check.
- Days 30 to 45: a prompt to plan the next 3 months.

**AI suggestions on Day 0** (postponed by Dmitry): categories and extra currencies for
the user's country and language, savings detection, and 5 to 10 local payees, only when AI
is configured. Agreed rules if it comes back: send only country and language, show the
static list first and swap suggestions in, never overwrite the user's edits, and treat a
failure as AI being off.

**Emails** are not part of the Econumo app. A separate cloud-side piece, on Resend for now,
sends them based on events, only when a step isn't done. Self-hosted instances get the
in-app steps only.

**Invitations,** a separate flow, with these findings to start from:

- Invite codes expire after 5 minutes (`internal/model/connection_invite.go`) and there is
  no invite link. A longer-lived link is needed.
- A budget can only be granted to a connected user and must then be accepted. Accepting
  adds the joiner's accounts that exist at that moment, so a joiner should accept after
  creating them.
- Joining probably puts both people's categories in one budget, so there may be two
  "Groceries" rows. This has not been tested.
- Private accounts need an answer here: every account starts in the automatic budget, and
  removing one is blocked once it has transactions from before the current month.
- First-screen wording agreed for that flow: "Just me" / "Together with someone" ("I'll set
  it up and invite them") / "Someone invited me" ("Enter the code from their invite").

## Revision 2026-11-08: Getting started as one stepper page

Decided with Dmitry after reviewing screenshots of the first version. This section replaces
"The flow", "The budget page after Day 0" and the cards/Inbox parts of "Skipping" above;
"Import and the typed balance" still applies.

### Layout

One Getting started page: a stepper with check marks on top, the current step's content
below. Desktop shows a horizontal stepper (number, label; done = filled check, skipped =
dashed, current = outlined); phones show "Getting started · N of M" with a thin segmented
progress bar. Steps can be revisited by clicking them. Visual direction (approved mockup):
rows grouped in one rounded card with soft dividers, icon tile + name + control on the
right, no bold, the accent colour only for progress and the primary button.

### Steps, in order

0. **Import (optional).** "Bring in your history": a CSV export creates accounts,
   categories and transactions (the existing CSV import). "I don't have one" skips it. A
   successful import marks steps 0, 1 and 2 done and moves to step 3.
1. **Categories.** The checklist from before, checkbox moved to the right of each row.
   Existing categories (from an import or a revisit) are listed ticked and locked.
2. **Accounts and currencies.** As before (today's balances, savings switch, currencies
   block), in the compact row style. Existing accounts are listed locked. After an import
   no balance is asked for.
3. **Budget and plan.** Entering the step creates the budget (all accounts, savings
   flags, kept categories) if none exists. Then "Plan this month": an Income group (expected
   income per income category) and an Expenses group (one amount per expense category), a
   running "Planned X of Y" line (planned spending against expected income, or against the
   money in the accounts when no income is planned), and "Save plan" writes this month's
   limits for both groups. "Skip for now" is allowed.
4. **Install as an app.** Only where the browser can install (or instructions exist);
   hidden when already running as an installed app.
5. **Apple Wallet.** Only on iPhone or Mac; leads to the existing Apple Wallet setup.

### Completion and surfaces

- Every step can be skipped. Onboarding completes when every visible step is done or
  skipped; then the page and its nav link go away and Home shows the budget. A "Skip
  setup" link completes everything at once.
- Steps marked done from elsewhere still count: setting a limit (plan), a CSV import
  (import), an Apple Wallet source (apple_wallet), installing (home screen).
- The get-ready cards on the budget page and the Inbox "Next steps" section are removed:
  this page is the only home of these steps.
- Phones open Getting started right after sign-up, like desktop. The old install popup and
  banner stay hidden while onboarding is incomplete (the install step replaces them).
