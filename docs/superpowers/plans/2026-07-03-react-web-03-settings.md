# React Web Migration — Plan 3 of 6: Settings Cluster

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the whole settings cluster at parity with the Vue app: the settings hub, profile (inline name edit + logout), change-password, Accounts & Folders (folder CRUD + drag-reorder of accounts across folders), Categories / Payees / Tags (CRUD, archive, drag-reorder, alphabetical sort), and the Budgets list (create, delete, set-as-default, go-to).

**Architecture:** Extends Plans 1–2 in `web-react/`. New feature folders: `features/settings` (hub/profile/password), `features/budgets` (list + API). Categories/payees/tags pages live in `features/classifications` (queries exist since Plan 2). Reordering uses **dnd-kit** (the spec's library choice). All the Plan-2 wire-contract constraints still apply (envelope, decimal strings, exact catalog strings, `X-Request-Id`, UUIDv7 client ids).

**Numbering note:** the spec's phase 3 (shared widgets) was folded into Plan 2, so this file is Plan 3 = the spec's "Settings cluster" phase.

## Scope notes (decided against the actual Vue code)

1. **CSV import/export menu rows are deferred to Plan 6** (the feature is Plan 6; a dead menu item is worse than a missing one). The hub renders without the two rows until then — flagged in the parity checklist.
2. **Shared-access/connections actions are deferred to Plan 6**: the account "Access control" action, budget Accept/Decline actions and the access-role UI all need the connections feature. Rows still render shared avatars (Plan 2 behavior); budgets rows still show `access[]` avatars and the not-accepted subtitle, but Accept/Decline/Access-control menu items are omitted until Plan 6.
3. **Category replace-on-delete is NOT built.** It is fully implemented but commented out of the Vue UI (both desktop menu and mobile context menu) — parity means plain delete only. The `deleteCategory(id, replaceId?)` API function from Plan 2 stays for Plan 6/whenever it is re-enabled.
4. **Budgets page gets create/delete/set-default/go-to only.** The Vue settings page never mounts an edit modal (update is exercised from the Budget page — Plan 5+); `reset-budget` exists server-side but the SPA never calls it — do not build either.
5. **The language row stays dormant**: `getLocaleOptions()` ships one locale, so the whole "User Interface" group is hidden (render the branch, gated by `length > 1`, exactly like Vue).

## Approved divergences introduced by this plan

- **Payee/tag edit resolves by id, not by original name.** Vue re-finds the record with `_.find(list, {name: initialValue})` — a latent bug when names collide. React keeps the id from the row that opened the dialog.
- **Server-side field errors surface on the profile name field.** Vue's name update is fire-and-forget (the server rejects names >20 chars with `TOO_LONG_ERROR` and the Vue UI silently ignores it). React shows the envelope's `errors.name` messages under the input. Client-side validation stays identical to Vue (`isValidName`, 2–64) — only the server's answer becomes visible.
- Everything else from Plans 1–2 still applies.

## Wire contract for this plan (verified against the Go source)

- **`POST /api/v1/user/update-name`** `{name}` → `{user: CurrentUserResult}`. Server validates length 3..20 (`TOO_SHORT_ERROR`/`TOO_LONG_ERROR`, message `"This value is too short."`/`"This value is too long."`) — stricter than the client's 2–64.
- **`POST /api/v1/user/update-password`** `{oldPassword, newPassword}` → `{}` (empty data). **Wrong old password → HTTP 400 `{"message":"Form validation error","code":400,"errors":{}}`** — the server's internal "Password is not correct" never reaches the wire. The client shows the generic `change_password_error` dialog on any rejection (Vue-parity).
- **`POST /api/v1/user/update-currency`** `{currency: <CODE string>}` → `{user}`. Bad code → 400 with `errors.currency: ["CurrencyCode is incorrect"]`.
- **`POST /api/v1/user/update-budget`** `{value: <budgetId>}` → `{user}`. Non-UUID → `errors.value: ["This value is not a valid UUID."]`; unknown budget → 400 with `message: "Plan not found"`.
- All four user mutations that echo `{user}` must **replace the `['user']` query cache** with the response user (it carries the refreshed options incl. the synthetic `currency_id`).
- **`GET /api/v1/budget/get-budget-list`** → `{items: [MetaResult]}` where `MetaResult = {id, ownerUserId, name, startedAt, currencyId, access: [{user:{id,avatar,name}, role: "owner"|"admin"|"user"|"guest", isAccepted: 0|1}]}`. Backend order is unspecified — the page sorts by `name` asc client-side (Vue parity). The owner appears in `access` as a synthetic `role:"owner", isAccepted:1` entry.
- **`POST /api/v1/budget/create-budget`** `{id, name, startDate, currencyId, excludedAccounts: Id[]}` → `{item: BudgetResult}` (full budget; the page only needs `item.meta`). **The client-sent id IS the entity id** (no operation-id indirection, unlike account/transaction create) — send `v7()`. Blank `currencyId` falls back server-side to the user's currency; name 3–64 → `"Budget name must be 3-64 characters"`.
- **`POST /api/v1/budget/update-budget`** `{id, name, currencyId, excludedAccounts}` → `{item: MetaResult}` (API module included for completeness; no UI this plan). **`POST /api/v1/budget/delete-budget`** `{id}` → `{}`.
- Category/payee/tag/folder/account mutation payloads are exactly the Plan-2 API modules (already built and tested); the pages must send `position` as the **0-based array index** and only for items whose index changed (`getChangedPositions` semantics), via `{changes:[{id,position}]}` (+ `folderId` for accounts).
- **Sort-mode is not a view mode**: choosing "Alphabetically (A-Z/Z-A)" computes the new order client-side and **persists it via order-\*-list** — the lists always render by `position` asc. Only alphabetical asc/desc is reachable (the usage-count options are commented out in Vue).
- Settings lists show **own items only** (`ownerUserId === me`); archived items stay inline, greyed, with the "Archived (inactive)" sublabel — no separate archived section, no type grouping for categories.
- Silent create-dedupe (case-insensitive, own items): if the name already exists, resolve with the existing item and skip the API (Plan-2 mutation hooks already do this); update-dedupe rejects instead.

---

### Task 1: User settings mutations (TDD)

**Files:** Extend `web-react/src/features/user/queries.ts`; test `web-react/src/features/user/queries.test.tsx`; extend `web-react/src/api/user.ts` if a signature is missing.

Produce `useUpdateName()`, `useUpdatePassword()`, `useUpdateCurrency()`, `useUpdateDefaultBudget()`:
- name/currency/budget mutations set `['user']` to the response `user` (extend the api functions to return `response.data.data.user` — the Plan-1 module returns `void`; change `updateName/updateCurrency/updateDefaultBudget` to `Promise<CurrentUserDto>`).
- `useUpdateDefaultBudget` additionally invalidates `['budget']` (Vue's `resetCachedBudget`).
- `useUpdatePassword` stays `Promise<void>`; rejection carries the axios error (dialog handled by the page).
- Metrics: `USER_UPDATE_NAME`, `USER_UPDATE_PASSWORD`, `USER_UPDATE_CURRENCY`, `USER_UPDATE_DEFAULT_BUDGET` on success.

- [x] **Step 1: failing tests** — MSW echoes `{user}` with a changed name → `['user']` cache equals the echoed user; update-budget invalidates `['budget']`; update-password rejects on the 400 `Form validation error` envelope.
- [x] **Step 2: implement.** **Step 3:** targeted tests → PASS.
- [x] **Step 4: commit** `feat(web-react): user settings mutations`.

---

### Task 2: `PromptDialog` + `SortDialog` shared components (TDD)

**Files:** Create `web-react/src/components/PromptDialog.tsx`, `web-react/src/components/SortDialog.tsx`; tests alongside.

- **PromptDialog** (port of `PromptDialogModal.vue`): props `{open, onClose, onSubmit(value), title, inputLabel, initialValue?, validate?: (v: string) => string | null, submitLabel, cancelLabel}` on `ResponsiveDialog` (`dismissible={false}` — Vue uses `no-backdrop-dismiss`); single autofocused input; the validate callback returns the exact catalog message or null; submit on Enter/button.
- **SortDialog** (port of `SortDialogModal.vue`): props `{open, onClose, onPick(direction: 'asc' | 'desc')}`; title `modals.sort.header` ("Sorting"); exactly two option buttons — `modals.sort.mode.alphabet.asc` ("Alphabetically (A-Z)") / `.desc` ("Alphabetically (Z-A)") — plus Cancel. (The usage-count options are dead in Vue; do not render them.)

- [x] **Step 1: failing tests** (prompt: validation message blocks submit, valid value submits and closes, initialValue seeds edit mode; sort: pick fires direction). **Step 2: implement.** **Step 3:** PASS.
- [x] **Step 4: commit** `feat(web-react): prompt and sort dialogs`.

---

### Task 3: dnd-kit + `SortableList` (TDD)

**Files:** `pnpm add @dnd-kit/core @dnd-kit/sortable @dnd-kit/utilities`; create `web-react/src/components/SortableList.tsx`; test alongside.

A generic vertical sortable list: `{items: {id}[], onReorder(ids: string[]), renderItem(item, handleProps), disabled?}` built on `DndContext` + `SortableContext` + `verticalListSortingStrategy`; drag starts only from the handle (`handleProps` spread onto the grip icon — Vue's `.sortable-control` with `drag_indicator`); keyboard sensor included. `onReorder` fires with the full new id order after drop. Cross-container drag (accounts between folders) is composed in Task 6 with a second-level `DndContext` — keep `SortableList` single-container.

Helper `getChangedPositions(current: {id, position}[], orderedIds: string[]): {id, position}[]` — 0-based index diff, changed items only (exact Vue port; also handles the "no changes → empty array" case). Put it in `web-react/src/lib/ordering.ts` with unit tests.

- [x] **Step 1: failing tests** — `getChangedPositions` (no-op, swap, insert), SortableList renders items and calls `onReorder` (simulate via keyboard sensor: space, arrow, space). **Step 2: implement.** **Step 3:** PASS + `pnpm build`.
- [x] **Step 4: commit** `feat(web-react): dnd-kit sortable list and position diff helper`.

---

### Task 4: Settings hub page (TDD)

**Files:** Create `web-react/src/features/settings/SettingsPage.tsx`; swap `/settings` in `routes.tsx`; test alongside.

Parity with `Settings.vue` (NO redirect — the hub is a real page at every breakpoint; on compact viewports the shell already shows only the workspace):
- Mobile back button → `/` (`navigateTo('home', true)` equivalent). Titles: mobile `pages.settings.settings.header` ("Settings") / desktop `pages.settings.settings.header_desktop` ("Service settings").
- User block (avatar `?s=100`… Vue hub uses 50px — use the hub's 50) → links to `/settings/profile`.
- Group label `pages.settings.settings.groups.service` ("Service"), then menu rows in Vue order:
  1. **Full sync** (`pages.settings.sync.menu_item`) — button → invalidate all queries; hint shows the last-loaded time (min `dataUpdatedAt` across the core queries, formatted `Y-m-d H:i:s`, `-` when unknown).
  2. Shared access (`modules.connections.pages.settings.menu_item`) → `/settings/connections` (EmptyPage until Plan 6).
  3. Budgets (`modules.budget.page.settings.menu_item`) → `/settings/budgets`.
  4. Accounts and Folders (`pages.settings.accounts.menu_item`) → `/settings/accounts`.
  5. Categories (`modules.classifications.categories.pages.settings.menu_item`) → `/settings/categories`.
  6. Payees (`modules.classifications.payees.pages.settings.menu_item`) → `/settings/payees`.
  7. Tags (`modules.classifications.tags.pages.settings.menu_item`) → `/settings/tags`.
  8. *(CSV import/export rows deferred to Plan 6.)*
  9. **Default currency** (`pages.settings.currency.menu_item`): inline `CurrencySelect` bound to the user's currency **code**; change → `useUpdateCurrency({currency: code})` immediately.
- "User Interface" group + Language row only when `getLocaleOptions().length > 1` (currently hidden).

- [x] **Step 1: failing tests** — menu rows render with exact labels and navigate; currency change posts `{currency:"EUR"}` and the user cache updates; language group absent with one locale.
- [x] **Step 2: implement.** **Step 3:** PASS.
- [x] **Step 4: commit** `feat(web-react): settings hub page`.

---

### Task 5: Profile page (TDD)

**Files:** Create `web-react/src/features/settings/ProfilePage.tsx`; swap `/settings/profile`; test alongside.

Parity with `Profile.vue`:
- Header `modules.user.page.settings.profile.header` ("User profile"); mobile back → `/settings`; desktop breadcrumb "Service settings" → `/settings`.
- User block: avatar (`?s=100`), name, email, and the **Logout** link (`pages.settings.settings.logout`) → ConfirmDialog with `modules.user.modal.sign_out.{title,question}` + actions `.action.logout` ("Logout") / `.action.cancel` ("Stay") → navigate `/logout` (the Plan-1 LogoutPage does the rest). Mobile toolbar has a power icon triggering the same dialog.
- **Name**: inline input, saved on blur/Enter when changed; client rules `isNotEmpty` → `modules.user.form.user.name.validation.required_field`, `isValidName` → `...invalid_name` ("Enter your name"); server `errors.name` messages shown beneath (approved divergence).
- **E-mail**: disabled/readonly input, label/placeholder from `modules.user.form.user.email.*`.
- Security group (`modules.user.page.settings.profile.groups.security`): row `modules.user.page.settings.profile.change_password.menu_item` → `/settings/profile/change-password`.

- [x] **Step 1: failing tests** — name blur posts `{name}` and cache updates; server 400 with `errors.name` shows the message; logout confirm shows exact copy and navigates; email input disabled.
- [x] **Step 2: implement.** **Step 3:** PASS.
- [x] **Step 4: commit** `feat(web-react): profile page with inline name edit and logout confirm`.

---

### Task 6: Change-password page (TDD)

**Files:** Create `web-react/src/features/settings/ChangePasswordPage.tsx`; swap the route; test alongside.

Parity with `ChangePassword.vue` (exact keys — they cross two namespaces):
- Fields and rules, validated on submit:
  - current: `isNotEmpty` → `modules.user.form.change_password.password.validation.invalid_password` ("Enter current password");
  - new: `isValidPassword` → `modules.user.form.user.password.validation.invalid_password` ("Password must be at least 4 characters"); `≠ old` → `modules.user.form.change_password.new_password.validation.not_equals`;
  - retry: `isNotEmpty` → `modules.user.form.user.password_retry.validation.required_field`; `isValidPassword` → `modules.user.form.user.password_retry.validation.invalid_password` ("Re-enter password"); `=== new` → `modules.user.form.change_password.new_password_retry.validation.not_equals` ("Passwords do not match").
- Submit → `LoadingDialog` (`modules.user.modal.change_password_loading.label` "Please wait") → `useUpdatePassword({oldPassword, newPassword})` (retry never sent).
- Success: clear the form, show a dialog with `modules.user.modal.change_password_success.text` + Close (`elements.button.close.label`); **stay on the page**.
- Failure (incl. wrong old password → generic 400): dialog `modules.user.modal.change_password_error.{header,text}` + Close.
- Breadcrumbs: "Service settings" → `/settings`, "Profile" (`modules.user.page.settings.profile.menu_item`) → `/settings/profile`; mobile back → `/settings/profile`.

- [x] **Step 1: failing tests** — all validation messages exact; success flow posts `{oldPassword,newPassword}` only and shows the success text; MSW 400 shows the error dialog.
- [x] **Step 2: implement.** **Step 3:** PASS.
- [x] **Step 4: commit** `feat(web-react): change password page`.

---

### Task 7: Accounts & Folders settings page (TDD)

**Files:** Create `web-react/src/features/accounts/AccountsSettingsPage.tsx` (+ small subcomponents as needed); swap `/settings/accounts`; test alongside.

Parity with `pages/Settings/Accounts.vue`:
- Header `pages.settings.accounts.header` ("Accounts and Folders"); toolbar: **Create folder** (`pages.settings.accounts.create_folder`, desktop button / mobile icon) and mobile back → `/settings`.
- One section per folder (ALL folders, hidden ones too — hidden get a `visibility_off` marker), ordered by position. Folder header: name + per-folder actions menu (`more_vert`):
  - **Move up / Move down** (`elements.button.up.label` / `down.label`; up hidden for index 0, down hidden for last) → swap positions → `useOrderFolders` with the changed `{id, position}` pairs.
  - **Rename** (`elements.button.edit.label`) → `PromptDialog` (`pages.settings.accounts.update_folder_modal.header` "Change name", validation `isNotEmpty` → `elements.form.account.folder.validation.empty_name`, `isValidFolderName` → `...error_name_length`) → `useUpdateFolder`.
  - **Hide/Show** (`elements.button.hide.label` / `show.label`) → `useHideFolder`/`useShowFolder`.
  - **Delete** (`elements.button.delete.label`, hidden for index 0) → ConfirmDialog (`pages.settings.accounts.delete_folder_modal.title` "Delete folder?", question `...delete_folder_modal.question` "Do you want to delete the folder «{folder}»?") → `useReplaceFolder({id, replaceId: <last other folder>})` (Vue picks `_.last` of the remaining folders — accounts move there).
  - Create folder → `PromptDialog` (`pages.settings.accounts.create_folder_modal.header` "Create new folder", same validation) → `useCreateFolder`.
- Account rows inside each folder, **drag-reorderable across folders** (dnd-kit; handle = grip icon): row = handle + `EntityIcon` + name + balance. Drop → rebuild the flat global order (folder by folder), set the moved account's `folderId` to the target folder, diff with `getChangedPositions` semantics extended with `folderId`, → `useOrderAccounts` (`{changes:[{id, folderId, position}]}`).
- Account row actions: desktop `more_vert` menu — **Edit** → the Plan-2 `AccountDialog`; **Delete** → ConfirmDialog (question `pages.settings.accounts.delete_account_modal.question` "Are you sure you want to remove the account «{account}»?") → `useDeleteAccount`. Mobile row tap → a details dialog (`pages.settings.accounts.preview_account_modal.header` "Account details": icon, name, balance, currency) with Edit/Delete buttons. *(Access control deferred to Plan 6.)*
- Empty state: `pages.settings.accounts.list_empty_create` ("Add") + `...list_empty_new_account` ("new account") → opens `AccountDialog`.

- [x] **Step 1: failing tests** — folder create/rename via PromptDialog post the right payloads; move-down posts swapped positions; delete-folder confirm posts `{id, replaceId}`; account delete confirm removes from cache; cross-folder reorder posts `{id, folderId, position}` changes (drive `onReorder` directly if dnd simulation is brittle).
- [x] **Step 2: implement.** **Step 3:** PASS + full suite + build.
- [x] **Step 4: commit** `feat(web-react): accounts and folders settings page with drag reorder`.

---

### Task 8: Categories settings page (TDD)

**Files:** Create `web-react/src/features/classifications/CategoriesPage.tsx`, `web-react/src/features/classifications/CategoryDialog.tsx`; extend `features/classifications/queries.ts` with `useUpdateCategory`, `useArchiveCategory`, `useUnarchiveCategory`, `useDeleteCategory`, `useOrderCategories` (+ same for payees/tags here or in Task 9); swap `/settings/categories`; tests alongside.

Parity with `Categories.vue` (+ its composables/modals):
- Header `modules.classifications.categories.pages.settings.header` ("Categories"); create button `...pages.settings.create_category` ("Create category"); sort button (`blocks.list.order_list` "Reorder list", only when >1 item) → `SortDialog` → order own list by name asc/desc → `useOrderCategories` with the changed positions.
- **One flat list** of OWN categories (both types mixed), position order, dnd-kit reorder (whole list, no type constraint). Row: handle, `EntityIcon(icon)`, name (archived rows greyed + sublabel `...pages.settings.archived_item` "Archived (inactive)"), archive **toggle** (checked = active; instant, no confirm) → archive/unarchive + invalidate `['budget']`, actions menu Edit/Delete.
- **CategoryDialog** (create/edit): type toggle Income/Expense (`...forms.category.type.income/.expense`; disabled on edit), name (rules: `isNotEmpty` → `...forms.category.name.validation.required_field`, `isValidCategoryName` → `...invalid_name`, maxLength 64), icon grid (same `availableIcons`, default `defaultCategoryIcon`). Create posts `{id: v7(), name, type, icon}` (no accountId); edit posts `{id, name, icon}` (type never sent).
- Delete → ConfirmDialog (title `...modals.delete.title` "Delete category?", label = the category name) → `useDeleteCategory(id)` (`mode:'delete'`); on success drop the category from cache + null out `categoryId` on cached transactions + invalidate `['budget']` (Vue's `TRANSACTIONS_CATEGORY_DELETE`).
- Empty state `blocks.list.list_empty`.

- [x] **Step 1: failing tests** — create payload exact (type/icon, no accountId); edit payload has no type; archive toggle hits archive/unarchive; delete confirm posts `{id, mode:'delete'}` and clears the category from a cached transaction; A-Z sort posts changed positions.
- [x] **Step 2: implement.** **Step 3:** PASS.
- [x] **Step 4: commit** `feat(web-react): categories settings page`.

---

### Task 9: Payees + Tags settings pages (TDD)

**Files:** Create `web-react/src/features/classifications/ClassificationListPage.tsx` (shared pattern) + thin `PayeesPage.tsx` / `TagsPage.tsx`; extend queries with the payee/tag update/archive/delete/order mutations; swap both routes; tests alongside.

The two pages are structurally identical (Vue keeps two monolithic copies — React shares one component parameterized by config): header/create/sort toolbar, own-items list (position order, dnd-kit reorder, archived inline), archive toggle, `PromptDialog` for create/edit (**edit by id — approved fix**), delete ConfirmDialog.

Per-entity config (exact keys):
- Payees: header `modules.classifications.payees.pages.settings.header` ("Payees") — but the desktop `<h4>` uses `...pages.settings.menu_item` ("Payees (senders, recipients)") like Vue; create `...create_payee` ("Create a new payee"); dialog headers `...modals.create.header` / `...modals.edit.header` ("Update payee"); name rules → `...forms.payee.name.validation.{required_field,invalid_name}`; delete title `...modals.delete.title` ("Delete payee?"). Payee mutations do NOT touch `['budget']`.
- Tags: header `...tags.pages.settings.header` ("Tags"); create `...create_tag` ("Create a new tag"); dialog headers `...modals.create.header` ("Create a new tag") / `...modals.edit.header` ("Edit tag"); rules `...forms.tag.name.validation.*`; delete title `...modals.delete.title` ("Delete tag?"). Tag archive/delete invalidate `['budget']`.
- Delete also scrubs the id from cached transactions (`payeeId`/`tagId` → null).

- [x] **Step 1: failing tests** (parameterized where possible) — create/edit/delete payloads per entity; edit uses the id even when the name matches another row; archive toggle; A-Z sort persists positions; tag delete invalidates budget, payee delete does not.
- [x] **Step 2: implement.** **Step 3:** PASS.
- [x] **Step 4: commit** `feat(web-react): payees and tags settings pages`.

---

### Task 10: Budget list API + queries (TDD)

**Files:** Create `web-react/src/api/budget.ts`, `web-react/src/api/dto/budget.ts`, `web-react/src/features/budgets/queries.ts`; tests for the API + hooks.

DTOs (list-scoped only — the full `BudgetResult` envelope arrives in Plan 5):
```ts
export type BudgetRole = 'owner' | 'admin' | 'user' | 'guest'
export interface BudgetAccessDto { user: UserDto; role: BudgetRole; isAccepted: 0 | 1 }
export interface BudgetMetaDto { id: Id; ownerUserId: Id; name: string; startedAt: string; currencyId: Id; access: BudgetAccessDto[] }
```
API: `getBudgetList(): Promise<BudgetMetaDto[]>`; `createBudget(form: {id: Id; name: string; startDate: string | null; currencyId: Id; excludedAccounts: Id[]}): Promise<BudgetMetaDto>` (returns `data.item.meta`); `updateBudget(form: {id, name, currencyId, excludedAccounts}): Promise<BudgetMetaDto>` (`data.item`); `deleteBudget(id): Promise<void>`.

Queries: `useBudgets()` (key `['budgets']`, staleTime 10min, select: name-asc sort); `useCreateBudget` (append meta + invalidate `['user']`? — Vue refetches user data after create; do `invalidateQueries(['user'])`), `useDeleteBudget` (remove from `['budgets']`, invalidate `['budget']` + `['user']`). Metrics `BUDGET_CREATE`/`BUDGET_DELETE`.

Also add `['budgets']` to the shell's `useIsFullyLoaded` set? — Vue includes budgets in `isFullyLoaded`. **Yes**: add `useBudgets()` to the loading gate (and to the Task-4 lastSync min).

- [x] **Step 1: failing tests** with wire-exact fixtures (access array incl. the synthetic owner entry, `isAccepted` ints) — list sorted by name; create posts the exact payload and the client id becomes the entity id (echo asserts same id); delete removes from cache.
- [x] **Step 2: implement (incl. the loading-gate addition).** **Step 3:** PASS + full suite.
- [x] **Step 4: commit** `feat(web-react): budget list API and queries`.

---

### Task 11: Budgets settings page (TDD)

**Files:** Create `web-react/src/features/budgets/BudgetsPage.tsx`, `web-react/src/features/budgets/BudgetDialog.tsx`; swap `/settings/budgets`; tests alongside.

Parity with `Budgets.vue` (minus deferred access actions):
- Header `modules.budget.page.settings.header` ("Budgets"); create button `...create_budget` ("Create a new budget"); empty state `blocks.list.list_empty`.
- Rows sorted by name asc: **default-budget bookmark** (filled `turned_in` disabled when `userOption(budget) === id`; outline `turned_in_not` clickable → `useUpdateDefaultBudget(id)`), name (truncated; rows where my access entry is missing/not accepted get greyed + subtitle `...level.<role>` + " - " + `...not_accepted` — data-driven, actionable Accept comes in Plan 6), shared avatars when `access.length > 1`.
- Row menu: **Go to the budget** (`...list_actions.go_to`) → set default (if not already) then navigate `/budget`; **Delete** (owner only) → ConfirmDialog (`...delete_modal.title` "Delete the budget?", question `...delete_modal.question` "Are you sure you want to delete {name}?") → `useDeleteBudget`.
- **BudgetDialog** (create): name (rules `modules.budget.form.budget.name.validation.{required_field,invalid_name}`, maxLength 64), `CurrencySelect` (default = user currency; required → `modules.budget.form.budget_envelope.currency.validation.required_field`), accounts include/exclude toggle list (`modules.budget.modal.budget_form.accounts` "Accounts"; all own accounts, toggled-off ids go to `excludedAccounts`). Header `modules.budget.page.settings.create_modal.header` ("Create a new budget"); submit `elements.button.create.label`. Posts `{id: v7(), name, startDate: '', currencyId, excludedAccounts}` (Vue sends `startDate: ''`). Client dedupe: same-name own budget resolves without an API call (Vue guard). Failure → dialog `modules.budget.modal.generic_error.{header,description}`.

- [x] **Step 1: failing tests** — rows name-sorted with the default bookmark on the user's `budget` option; set-as-default posts `{value:id}` and re-marks; create posts the exact payload and appends the row; delete confirm removes it.
- [x] **Step 2: implement.** **Step 3:** PASS + full suite + build + lint.
- [x] **Step 4: commit** `feat(web-react): budgets settings page`.

---

### Task 12: Settings parity check (manual, gate for Plan 3)

**Files:** none (fix divergences with tests before closing).

- [x] **Step 1: run** backend (scratch sqlite, inline env — see Plan 2 Task 18), React dev server; the Vue reference at `:8181`. Seed: user + a few accounts/folders/categories/payees/tags/transactions + a second currency (`currency:add EUR Euro 2`).
- [x] **Step 2: walk in BOTH apps at 1280px / 375px:**

1. Hub: menu order and labels, sync row (spinner/refetch + last-sync hint), default-currency inline select (change → immediately persisted; verify via reload and the Vue app), language group hidden.
2. Profile: rename inline (blur saves; sidebar user block updates), server rejection of a >20-char name shows the message (React divergence — Vue silently fails: confirm Vue's silence), email readonly, logout confirm copy → logs out.
3. Change password: each exact validation message; wrong old password → error dialog; success → success dialog, form cleared, still on the page; re-login with the new password.
4. Accounts & Folders: create/rename folder (dupe name behavior), move up/down, hide (folder disappears from the sidebar but stays here), delete folder (accounts move to the fallback folder), drag an account between folders (order + folderId persist across reload and in the Vue app), account edit/delete, empty-state add.
5. Categories: create (type + icon), edit (type frozen), archive toggle (row greys inline; archived category disappears from the transaction-modal options), delete (transactions lose the category), drag-reorder + A-Z sort persist.
6. Payees/Tags: create/edit/delete/archive/reorder; tag changes reflected in the transaction modal pills.
7. Budgets: create (excluded accounts honored — check via the Vue budget page), set-as-default bookmark, go-to navigates to `/budget` (placeholder in React until Plan 5 — verify navigation only), delete.
8. Mobile: hub→page→back navigation single-pane; dialogs as bottom sheets; account settings page usable (context actions via tap).
9. Known temporary gaps to confirm and note (not fix): CSV rows absent, access-control/Accept/Decline absent, `/budget` and `/settings/connections` still placeholders.

- [x] **Step 3: record** results:

```bash
git commit --allow-empty -m "chore(web-react): settings parity check vs Vue app passed (desktop/mobile)"
```

---

## Plan sequence

This is Plan 3 of 6 (the spec's phase 4; phase 3 was folded into Plan 2). Remaining:

4. Budget (page split, envelopes, limits, widgets, modals, month picker) — spec phase 5.
5. Connections + onboarding + CSV import/export + deferred access-control/Accept-Decline + the `web/` swap commit — spec phase 6.
