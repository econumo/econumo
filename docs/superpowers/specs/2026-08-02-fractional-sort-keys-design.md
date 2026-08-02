# Fractional sort keys for user-defined ordering

**Date:** 2026-08-02
**Status:** Approved, ready for implementation planning

## Problem

Every user-sortable entity stores its order as an `int16` `position` column. The
seven reorder endpoints take *absolute* positions from the client, so the client
owns the ordering algorithm: rebuild the whole index sequence, diff it against
what it last saw, and send every row whose index moved. Dragging an item to the
top of a 50-item list sends 50 changes and writes 50 full-row upserts.

Two costs follow from that:

- **Client complexity.** `web/src/lib/ordering.ts`, `accountOrdering.ts` and
  `elementMove.ts` exist only to compute and diff index sequences.
- **Write amplification.** One drag rewrites O(N) rows.

There is also a latent bug: `getChangedPositions` compares `item.position !== index`,
i.e. it assumes positions are dense 0-based indices, but nothing guarantees that.
There is no unique constraint on `(owner, position)`, so positions duplicate and
go sparse, and the client reports spurious changes.

Concurrency and the `int16` ceiling are explicitly **not** motivations.

## Approach

Two changes that are separable but reinforce each other:

1. **Relative-move API.** Endpoints take `{id, afterId}` instead of
   `{changes: [{id, position}]}`. The server resolves the anchor. This is what
   removes the client-side algorithm.
2. **Fractional string sort keys.** The stored order is a base-62 string; a
   midpoint between any two keys always exists, so a move writes exactly one row
   and nothing ever needs renumbering.

The sort key **never leaves the server**. Responses keep the frozen `position`
integer, now computed as a dense 0-based index of the sorted list. That preserves
the wire contract, fixes the latent density bug, makes the key format free to
change later, and forces the relative-move API to be the only way to reorder.

## Scope

All six tables, including `budgets_elements`.

## 1. Storage & the key

### Column

`position` is replaced by `sort_key` on all six tables:

| Table | Key is ordered within |
| --- | --- |
| `categories`, `tags`, `payees` | `user_id` (owner) |
| `folders` (account folders) | `user_id` |
| `accounts_options` | `user_id` — one global sequence; folder is a separate attribute |
| `budgets_folders` | `budget_id` |
| `budgets_elements` | `(budget_id, folder_id)` |

- sqlite: `sort_key TEXT NOT NULL DEFAULT ''`
- pgsql: `sort_key TEXT COLLATE "C" NOT NULL DEFAULT ''`

The `COLLATE "C"` is load-bearing. SQLite sorts `TEXT` byte-wise (`BINARY`);
PostgreSQL sorts by database collation, which folds case and orders digits
against letters differently. `enginecompare` demands byte-identical responses
from both engines, so without `COLLATE "C"` the suite fails. Reads stay
`ORDER BY sort_key, id` — the id tie-break is retained as a backstop.

`position` is dropped in the same migration; no second source of truth. It is
not indexed anywhere, so sqlite `ALTER TABLE … DROP COLUMN` works without a
table rebuild. Dropping it also removes pgsql's `budgets_folders CHECK (position >= 0)`
and the `SMALLINT UNSIGNED` sqlite/pgsql skew.

No composite `(user_id, sort_key)` indexes. These lists are tens of rows per
user; the existing `user_id` index suffices and every added index is another
engine-parity surface.

### `internal/shared/sortkey`

A new dependency-free kernel package (imports nothing internal, per the archtest
rule):

```go
type Key string

// "" on either side means ±infinity, so Between("", "") is the first key of an
// empty list, Between(last, "") appends and Between("", first) prepends.
func Between(prev, next Key) (Key, error)

// The seed key for an empty list, chosen by the list's growth direction.
type Growth int
const (
    GrowsUp   Growth = iota // creation appends: seed "a0"
    GrowsDown               // creation prepends: seed "c000"
)
func Seed(g Growth) Key

type Item struct {
    ID  string
    Key Key
}

// siblings must be pre-sorted by Key. An afterID not present in siblings
// appends to the end. Errors only on a malformed sibling key.
func Place(siblings []Item, afterID string) (Key, error)
```

`""` carries two unrelated meanings that must not be conflated: as a `Between`
*argument* it is the ±infinity sentinel, and as a *stored column value* it marks
a row excluded from ordering (see `PositionUnset` below). Excluded rows are
never passed to `Place` as siblings, so the two never meet.

### Key format

This is the `fractional-indexing` / Figma algorithm, not naive string bisection.
The alphabet is base-62 in ASCII-ascending order
(`0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz`), so
lexicographic order equals byte order equals the intended order, and comparison
uses the prefix rule (`"abc" < "abc0"`).

A key is a **magnitude prefix**, an integer part, and optional fractional digits.
The prefix encodes how many integer digits follow: `a` = 1, `b` = 2, `c` = 3, and
so on. Appending increments the integer part rather than bisecting:

```
a0 → a1 → … → a9 → aA → … → aZ → aa → … → az → b10 → b11 → … → bzz → c100 → …
```

Keys therefore stay 2-4 characters under append-heavy use, which matters because
**append is the dominant operation** — categories, tags, payees, accounts and
account folders all append on create. Naive bisection toward "+infinity" would
converge geometrically (`002` → `V` → `k` → `s` → `w` → `y` → `z` → `zV` …),
burning a character roughly every six appends; the magnitude prefix is what
avoids that.

Inserting *between* two keys still appends fractional digits
(`c000` → `c000V` → `c001`), so a midpoint always exists. There is no exhaustion
case and therefore no rebalancing fallback to maintain — that is the property
that lets the renumbering code be deleted rather than merely relocated.

Below `a0` the magnitude letters invert: uppercase `A`-`Z` count *downward*
(`a0` → `Zz` → `Zy` → …). This is the trickiest part of the algorithm and the
easiest to implement wrong, which is why the seed policy below keeps it off the
common path.

### Seed policy

The key for the first row in an empty list depends on which direction that list
grows on create:

| Lists | Direction | Seed |
| --- | --- | --- |
| categories, tags, payees, accounts, account folders | append | `a0` |
| budget folders, budget envelope elements | prepend | `c000` |

Seeding the prepend-oriented lists at `c000` buys 238,328 prepends of headroom
in the plain digit/lowercase space. Without it those two tables would drop into
the negative magnitudes on their *second* row, making the inverted-magnitude
branch the norm rather than a rarity.

Magnitudes differ across users and across tables by design — keys are only ever
compared within one scope, never across scopes.

`Place` lives here rather than being copied into seven feature packages. It is
pure list arithmetic over a pre-sorted slice, with no database dependency, so the
kernel is the right home and features never import each other to reach it.

### Model

`Position int16` becomes `SortKey sortkey.Key` on `model.Category`, `Tag`,
`Payee`, `Folder`, `BudgetFolder`, `BudgetElement`. `UpdatePosition` becomes
`UpdateSortKey`. `model.PositionUnset` becomes the empty key (which sorts first
byte-wise — harmless, since unset rows are excluded from lists anyway).
`validatePositionField` and `internal/model/position_validation_test.go` are
replaced by key-charset validation.

## 2. API

Seven routes are removed and seven added, so the `minRoutes = 101` floor in
`internal/test/apiparity/guard_test.go` needs no edit.

| Removed | Added |
| --- | --- |
| `POST /api/v1/category/order-category-list` | `POST /api/v1/category/move-category` |
| `POST /api/v1/tag/order-tag-list` | `POST /api/v1/tag/move-tag` |
| `POST /api/v1/payee/order-payee-list` | `POST /api/v1/payee/move-payee` |
| `POST /api/v1/account/order-account-list` | `POST /api/v1/account/move-account` |
| `POST /api/v1/account/order-folder-list` | `POST /api/v1/account/move-folder` |
| `POST /api/v1/budget/order-folder-list` | `POST /api/v1/budget/move-folder` |
| `POST /api/v1/budget/move-element-list` | `POST /api/v1/budget/move-element` |

`move-` rather than `sort-`: it fits `/{module}/{action}-{subject}` with a real
verb, and budget already used the word.

### Requests

The batch concept is gone — a drag moves one thing.

```jsonc
// move-category / move-tag / move-payee
{ "id": "<uuid>", "afterId": "<uuid>" | null }   // null = move to first

// move-folder (account)
{ "id": "<uuid>", "afterId": "<uuid>" | null }

// move-folder (budget)
{ "budgetId": "<uuid>", "id": "<uuid>", "afterId": "<uuid>" | null }

// move-account
{ "id": "<uuid>", "afterId": "<uuid>" | null, "folderId": "<uuid>" | null }

// move-element (budget)
{ "budgetId": "<uuid>", "id": "<uuid>", "folderId": "<uuid>" | null, "afterId": "<uuid>" | null }
```

`afterId` alone, not `afterId` + `beforeId`: the client always knows what it
dropped onto, and one field is the point of the exercise. `folderId` is always
sent (nullable) rather than optional, so there is no absent-versus-null
distinction to encode in Go.

The server computes `sortkey.Between(key(afterId), key(nextSiblingOf(afterId)))`
and writes exactly one row.

### Anchor resolution degrades, it does not error

A well-formed `afterId` that is not in the caller's scope (deleted concurrently,
another user's row, a different folder) causes the item to append to the end.
This matches today's behaviour — `OrderCategoryList` silently ignores non-owned
ids and `OrderAccountList` silently skips unavailable accounts — and it avoids
introducing a new `errs` code, which would mean editing **11 locale catalogues**
plus satisfying the two-way `errs.AllCodes` ↔ `errors.*` parity guard. A
malformed UUID still returns 400 through existing validation.

### Responses

Response shapes are unchanged. Classification, account and folder moves return
the full list (the SPA's `replaceAll(items)` depends on it); budget moves return
`{}` / `{budget_id, element_ids}`.

`position` remains a JSON integer, now a **computed dense 0-based index**:

| Result DTO | Index scope |
| --- | --- |
| `CategoryResult`, `TagResult`, `PayeeResult` | the returned merged own + shared list |
| `AccountResult` | the user's account list (one global sequence) |
| `AccountFolderResult`, `BudgetFolderResult` | the user's / budget's folder list |
| `ParentElementResult` | within its folder, or within the no-folder group |

### MCP

`move_element` (`internal/budget/mcp/mcp.go:326`) loses its `items[]` array for
the singular shape. `moveElementResult{budget_id, element_ids}` is unchanged.
No other MCP tool exposes ordering.

## 3. Migration & backfill

One migration pair, `internal/infra/storage/migrations/{sqlite,pgsql}/<ts>.sql`.
Three steps per table — add, backfill, drop — for all six tables in one file, in
the single transaction the runner already wraps around it.

Migrations in this repo are embedded `.sql` only; there is no Go-migration hook.
The backfill is therefore expressed in pure SQL on both engines, using
`row_number()` for the rank in the current `(position, id)` order and a 3-char
base-62 encoding via `substr`:

```sql
-- sqlite
ALTER TABLE categories ADD COLUMN sort_key TEXT NOT NULL DEFAULT '';

UPDATE categories SET sort_key = (
  SELECT 'c'
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n / 3844) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n /   62) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 +  r.n         % 62, 1)
  FROM (SELECT id, row_number() OVER (PARTITION BY user_id ORDER BY position, id) - 1 AS n
        FROM categories) r
  WHERE r.id = categories.id
);

ALTER TABLE categories DROP COLUMN position;
```

The pgsql sibling is identical apart from
`ADD COLUMN sort_key TEXT COLLATE "C" NOT NULL DEFAULT ''`. `substr` is 1-based,
`||` concatenates and `/` is integer division in both engines.

The literal `'c'` is the magnitude prefix for a 3-digit integer part. Every row
in a scope shares it, so ordering within the scope is decided purely by the three
base-62 digits, and 3 digits holds 238,328 rows per scope. Backfilled lists
therefore run `c000`, `c001`, `c002`, … and the next created category appends as
`c003` — which is also why the prepend-oriented seed in §1 is `c000`: it puts
fresh installs and migrated ones in the same magnitude.

No stride is applied. Ranks encode densely because insertion between two adjacent
keys needs no numeric gap — `Between` appends a fractional digit
(`Between("c000","c001") == "c000V"`).

Partitions match the key scopes above: `user_id` for categories, tags, payees,
folders and `accounts_options`; `budget_id` for `budgets_folders`;
`(budget_id, folder_id)` for `budgets_elements`, where `NULL` folders group
together in both engines.

Afterwards: edit `query/{sqlite,pgsql}/*.sql` (SELECT lists, the seven
`ORDER BY position, id` clauses, and `position = excluded.position` in each
upsert) and run `sqlc generate`. Comments in those `.sql` files must stay
**ASCII-only** — an em dash mangles sqlc v1.30's sqlite codegen through a
byte-offset bug.

There is no down migration (this repo has none) and no separate backfill
command; it runs on boot like every other migration. `data:import-sqlite` is
unaffected, since it already refuses a schema-version mismatch.

## 4. Use cases

### Deleted

| File / symbol | Reason |
| --- | --- |
| `internal/{category,tag,payee}/order.go` | three near-identical `Order*List` use cases become one-screen `Move*` use cases |
| `internal/account/order.go` `OrderAccountList` | → `MoveAccount` |
| `internal/account/folder_usecase.go:220` `OrderFolderList` | → `MoveFolder` |
| `internal/account/folder_usecase.go:202` `resetFolderPositions` | delete no longer renumbers |
| `internal/budget/folders.go:33-57`, `:126-145` renumber loops | create/delete no longer renumber |
| `internal/budget/move.go:76-106` `shiftElements` | envelope create becomes `Between("", firstKey)` |
| `internal/account/repo/repo.go:155` `MaxPosition` | a full Go-side scan of the user's options rows, used only to seed a position |
| `CountByOwner` (category/tag/payee repos) | used only to seed `SetPosition(count)`; confirm no other callers, then drop |
| `model.PositionChange`, `AccountPositionChange`, `FolderPositionChange`, `OrderFolderListItem`, `MoveElementListItem`, all `Order*Request` | replaced by the seven `Move*Request` DTOs |
| `validatePositionField`, `internal/model/position_validation_test.go` | replaced by key-charset validation |

### Shape of every move use case

Load the scope's siblings ordered by key → `sortkey.Place(siblings, afterID)` →
save one row. No new repository queries: every one of these paths already loads
its sibling list, so head/tail lookups happen in Go. That is what allows
`MaxPosition` and `CountByOwner` to be deleted.

### `restoreElementsOrder` survives, renamed and roughly halved

`internal/budget/move.go:122-292` loses its `sort.SliceStable` + `renumber`
closure and the `posMax = 0x7fff` sort sentinel. What remains was never about
ordering: creating missing `budgets_elements` rows for participating envelopes,
categories and tags; forcing archived and envelope-child rows to the unset key
and no folder; pruning rows whose entity no longer participates. It is renamed
`syncElements`. Its four callers (`envelopes.go:76,161,204`, `accounts.go:195`)
are unchanged.

### Creation-time placement

Uniform: append via `Between(lastKey, "")` for categories, tags, payees, accounts
and account folders; prepend via `Between("", firstKey)` for budget folders and
envelope elements, preserving today's front-insert behaviour.

When the list is empty there is no neighbour to anchor against, so the seed comes
from `sortkey.Seed` per the §1 policy — `a0` for the five append-oriented lists,
`c000` for the two prepend-oriented ones. Concretely: a brand-new user's first
category is stored as `a0` and reports `position: 0`; their next four are
`a1`-`a4`.

### Access rules

Preserved exactly. Classification moves stay owner-only and silently no-op for a
sharee (issue #108) — `internal/{tag,payee}/api/order_shared_test.go` are renamed,
not rewritten. Budget moves stay `canUpdate`-gated. Account placement stays
per-user via `accounts_options`.

### Incidental fixes

- `MoveAccount` writes membership and key in a single transaction and rebuilds
  the response list inside it. Today `OrderAccountList` rebuilds outside the
  transaction (`internal/account/order.go:93`).
- Account folder sorts gain the `id` tie-break they lack today
  (`internal/account/read.go:97`, `usecase.go:313`, `folder_usecase.go:207,250`).

### Intended observable change

The presentation-only "Uncategorized" budget element currently reports
`position: 32767` (`uncategorizedPosition`, `builder_structure_build.go:14`).
Under dense indexing it reports the next real index. It is still last in the
list; the golden value changes on purpose.

## 5. Frontend

The client-side ordering algorithm collapses to one function:

```ts
// web/src/lib/ordering.ts  (replaces getChangedPositions)
export function afterIdFromDrop(orderedIds: string[], movedId: string): string | null {
  const i = orderedIds.indexOf(movedId)
  return i > 0 ? orderedIds[i - 1] : null
}
```

| File | Change |
| --- | --- |
| `web/src/lib/ordering.ts` | `getChangedPositions` → `afterIdFromDrop` |
| `web/src/features/classifications/ClassificationList.tsx:186-201` | `rebuildFullOrder` + `commitOrder` → one anchor, one call |
| `web/src/features/accounts/accountOrdering.ts` | `buildAccountChanges` deleted; `bucketsFromAccounts` / `moveAccount` keep only dnd-kit bucketing and lose the flat cross-folder position sequence |
| `web/src/features/budgets/elementMove.ts` | shrinks to computing the drop anchor. The local `arrangement` scaffolding exists only to stop the table snapping back during server renumbering; with a known destination it should be replaceable by a normal optimistic update — verify during implementation rather than assuming |
| `web/src/api/{category,tag,payee,account,budget}.ts`, `web/src/api/dto/*.ts` | new request shapes; `position: number` stays on all result types |
| `web/src/features/*/queries.ts` | `useOrder*` → `useMove*`, same `replaceAll(items)` from the response |

`METRICS` keys are frozen and route-independent (`ACCOUNT_ORDER_LIST`,
`CATEGORY_ORDER_LIST`, …), so they are reused unchanged and
`metrics-coverage.test.ts` stays green.

Gate: run `pnpm exec tsc -b`. vitest and oxlint do not type-check, and a
request-shape mismatch is exactly the kind of error that slips past them.

## 6. Testing

- **New** `internal/shared/sortkey` unit tests: `Between` ordering invariants,
  `Place` anchor resolution including the unknown-anchor append path, and a
  property test that a random sequence of moves always yields the expected
  order. Repeated same-spot insertion is tested explicitly to pin the
  no-exhaustion claim.
- **New** `sortkey` tests for the two paths the seed policy makes rare, since
  rare is exactly when a latent bug survives to production:
  - **Magnitude rollover in both directions.** Appending across `az` → `b10` and
    `bzz` → `c100`, and prepending across `a0` → `Zz` and `A0` → the next
    magnitude down. Assert the emitted keys sort correctly *as bytes*, not just
    that the function returns without error.
  - **Sustained prepend from a `c000` seed**, asserting keys stay in the
    digit/lowercase space for the full 3-digit range rather than dropping into
    the inverted uppercase magnitudes early.
- **New** key-length regression test: 1,000 sequential appends from a fresh seed
  must stay within 4 characters. This is the property the magnitude prefix exists
  to provide, and naive bisection would silently pass every ordering test while
  failing this one.
- **New** migration test alongside `migration_20260730_test.go` and
  `email_verified_backfill_test.go`, asserting the backfill preserves
  `(position, id)` order within every scope, including `NULL`-folder budget
  elements.
- **New** explicit engine-parity test for `COLLATE "C"`: identical keys, both
  engines, byte-identical ordering. This is the riskiest assumption in the
  design and gets its own test rather than relying on `enginecompare` catching
  it indirectly.
- **Rewrite** `internal/test/apiparity/catalogue_orderlists.go` into
  `catalogue_moves.go` with seven move scenarios.
  `testdata/golden/order_lists.golden` becomes orphaned and must be deleted —
  the guard errors on orphans (`guard_test.go:135`).
- **Regenerate** the other 19 goldens containing `"position"` with
  `UPDATE_GOLDEN=1`, then **inspect the diff**. Sparse and duplicate positions
  collapsing to dense indices is expected; any change to the *relative order* of
  items is a real bug.
- **Regenerate** `internal/test/mcpparity` goldens for `move_element` and
  `tools/list` (its input schema changed).
- `make test` (including `test-repo-pgsql`) and the 80% coverage gate.

## 7. Build order

1. `internal/shared/sortkey` package and tests (pure, no dependencies)
2. Migration, `sqlc generate`, model field swap
3. Repository layer
4. Use cases and routes, feature by feature: classifications → accounts → budget
5. MCP
6. Frontend
7. Golden regeneration and inspection

## 8. Rollout

This is a **breaking API change with no rollback**: seven routes removed,
`position` dropped from six tables, no down migration.

Self-hosters are safe by construction — the SPA is embedded in the same binary,
so there is no version skew. Anything driving `order-*-list` directly breaks.
The release needs a minor version bump, an explicit release-note callout, and
the "back up your database first" line the other one-way migrations carry.

## Explicitly out of scope

- Concurrent-edit conflict resolution. Fractional keys make concurrent moves
  commute as a side effect, but no conflict-resolution machinery is being built.
- Exposing the sort key on the wire.
- Any change to how shared classifications from different owners interleave in
  the merged list. That ordering is arbitrary today and stays arbitrary.
