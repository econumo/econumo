# Reporting Labels — Plan 1: Backend Core (label entity + transaction attachment)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `labels` entity (reporting-only classification) as a new `internal/label/` feature package mirroring `internal/tag/`, add an `icon` column to `tags`, and let a transaction carry many labels via a join table.

**Architecture:** Vertical feature slice. `internal/label/` holds behaviour only; the entity/DTOs live in `internal/model`. Persistence uses the engine-adapter pattern (a `querier` interface in sqlite-generated types, with `sqlite.go` passthrough and `pgsql.go` whole-struct conversion adapters). Labels never enter the budget-element code path — that isolation is the whole point of a separate table.

**Tech Stack:** Go (stdlib `net/http`, `log/slog`), sqlc (per-engine codegen), SQLite (`modernc.org/sqlite`) + PostgreSQL (`jackc/pgx/v5`), `internal/test/dbtest` + `internal/test/apiparity`.

**Spec:** `docs/superpowers/specs/2026-08-02-multi-tag-labels-design.md` — this plan covers Delivery phases **1 and 2**.

## Global Constraints

- **Every schema change needs TWO migration files** — `internal/infra/storage/migrations/sqlite/<version>.sql` AND `.../pgsql/<version>.sql`, same version string. sqlite ids are `TEXT`, pgsql ids are `UUID`; sqlite timestamps `DATETIME`, pgsql `TIMESTAMP(0) WITHOUT TIME ZONE`. The sqlite file carries the explanatory comment; the pgsql sibling says "See the sqlite sibling for the semantics."
- Migrations are discovered by `//go:embed sqlite/*.sql` in `internal/infra/storage/migrations/embed.go` and sorted lexically — **no registry file to edit**, dropping the `.sql` in is enough.
- **sqlc query `.sql` files must be ASCII-only.** An em dash in a query comment mangles sqlc v1.30's sqlite codegen (byte-offset bug). Use plain `-` in `query/**.sql` comments.
- Regenerate with `sqlc generate` (config `internal/infra/storage/sqlc/sqlc.yaml`). The config is directory-based (`queries: "./query/sqlite"` / `"./query/pgsql"`) so **new `.sql` files need no `sqlc.yaml` edit**.
- **pgsql adapters convert whole structs** (`tagRow(t)`, `pgsqlgen.UpsertTagParams(p)`). These only compile if both engines' generated structs are field-identical **in the same order**. Keep the two `.sql` column lists byte-identical apart from placeholders.
- Wire shapes are frozen (see CLAUDE.md): `isArchived` is int `0`/`1` (not bool); datetimes are `"2006-01-02 15:04:05"` via `datetime.Layout`; envelope is `{success, message, data}`.
- Validation strings are exact and asserted by tests. New label strings mirror tag's: `"Label name must be 3-64 characters"`, `"Label already exists."`, `"Labels list is empty"`.
- Comments: only exceptional scenarios / non-obvious rationale. No godoc restating a signature, no section dividers, no references to the removed PHP implementation.
- TDD: failing test → minimal code → green → commit. One commit per task.
- Verify with `make go-test` (build + vet + gofmt + OpenAPI-fresh + sqlite unit/integration + coverage gate ≥80). Regenerate goldens with `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/` then **INSPECT the diff**.
- Branch: work on `feature/reporting-labels-spec` (the spec branch) — one PR for all phases.

## Naming decided up front (used across all three plans)

| Concept | Name |
|---|---|
| Entity | `model.Label` |
| Read row | `model.LabelViewRow` |
| Wire DTO | `model.LabelResult` |
| Package | `internal/label` (`apptag`-style alias: `applabel`) |
| Repo | `internal/label/repo` (`labelrepo`) |
| Join table | `transactions_labels` |
| Icon default | tags `"tag"`, labels `"label"` |

---

### Task 1: Migration — `labels` table, `tags.icon`, `transactions_labels`

**Files:**
- Create: `internal/infra/storage/migrations/sqlite/20260803000000.sql`
- Create: `internal/infra/storage/migrations/pgsql/20260803000000.sql`
- Test: `internal/infra/storage/migrate/migrate_test.go` (existing suite must stay green)

**Interfaces:**
- Consumes: nothing.
- Produces: tables `labels(id, user_id, name, icon, position, is_archived, created_at, updated_at)`, `transactions_labels(transaction_id, label_id)`, and column `tags.icon`.

- [ ] **Step 1: Write the failing test**

Create `internal/infra/storage/migrate/labels_schema_test.go`:

```go
package migrate_test

import (
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
)

func TestLabelsSchemaExists(t *testing.T) {
	db := dbtest.New(t)
	var n int
	if err := db.Get(&n, db.Rebind(`SELECT COUNT(*) FROM labels`)); err != nil {
		t.Fatalf("labels table missing: %v", err)
	}
	if err := db.Get(&n, db.Rebind(`SELECT COUNT(*) FROM transactions_labels`)); err != nil {
		t.Fatalf("transactions_labels table missing: %v", err)
	}
	if err := db.Get(&n, db.Rebind(`SELECT COUNT(icon) FROM tags`)); err != nil {
		t.Fatalf("tags.icon column missing: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/infra/storage/migrate/ -run TestLabelsSchemaExists -v`
Expected: FAIL — `labels table missing: no such table: labels`

- [ ] **Step 3: Write the sqlite migration**

Create `internal/infra/storage/migrations/sqlite/20260803000000.sql`:

```sql
-- Reporting labels: a second, budget-neutral classification. Unlike tags, a
-- transaction may carry many labels, so the link is a join table rather than a
-- column on transactions. Labels never become budgets_elements rows, which is
-- what keeps their (deliberately overlapping) totals out of envelope math.
CREATE TABLE labels
(
    id            TEXT     NOT NULL
    , user_id     TEXT     NOT NULL
    , name        VARCHAR(64) NOT NULL
    , icon        TEXT     NOT NULL DEFAULT 'label'
    , position    SMALLINT NOT NULL DEFAULT 0
    , is_archived BOOLEAN  NOT NULL DEFAULT 0
    , created_at  DATETIME NOT NULL
    , updated_at  DATETIME NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE INDEX user_id_idx_labels ON labels (user_id);

CREATE TABLE transactions_labels
(
    transaction_id TEXT NOT NULL
    , label_id     TEXT NOT NULL
    , PRIMARY KEY (transaction_id, label_id)
    , FOREIGN KEY (transaction_id) REFERENCES transactions (id) ON DELETE CASCADE
    , FOREIGN KEY (label_id) REFERENCES labels (id) ON DELETE CASCADE
);
-- The PK's leading transaction_id serves the budget aggregation (which joins
-- from the transactions it already walks); this reverse index serves
-- delete-label and filter-by-label.
CREATE INDEX label_id_idx_transactions_labels ON transactions_labels (label_id);

-- Tags gain a persisted icon so the render path reads the stored value
-- everywhere. Existing rows backfill to the previously hardcoded budget-view
-- fallback, so no rendering changes for them.
ALTER TABLE tags ADD COLUMN icon TEXT NOT NULL DEFAULT 'tag';
```

- [ ] **Step 4: Write the pgsql migration**

Create `internal/infra/storage/migrations/pgsql/20260803000000.sql`:

```sql
-- See the sqlite sibling for the semantics.
CREATE TABLE labels
(
    id            UUID     NOT NULL
    , user_id     UUID     NOT NULL
    , name        VARCHAR(64) NOT NULL
    , icon        TEXT     NOT NULL DEFAULT 'label'
    , position    SMALLINT NOT NULL DEFAULT 0
    , is_archived BOOLEAN  NOT NULL DEFAULT false
    , created_at  TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , updated_at  TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE INDEX user_id_idx_labels ON labels (user_id);

CREATE TABLE transactions_labels
(
    transaction_id UUID NOT NULL
    , label_id     UUID NOT NULL
    , PRIMARY KEY (transaction_id, label_id)
    , FOREIGN KEY (transaction_id) REFERENCES transactions (id) ON DELETE CASCADE
    , FOREIGN KEY (label_id) REFERENCES labels (id) ON DELETE CASCADE
);
CREATE INDEX label_id_idx_transactions_labels ON transactions_labels (label_id);

ALTER TABLE tags ADD COLUMN icon TEXT NOT NULL DEFAULT 'tag';
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/infra/storage/migrate/ -run TestLabelsSchemaExists -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/infra/storage/migrations/sqlite/20260803000000.sql \
        internal/infra/storage/migrations/pgsql/20260803000000.sql \
        internal/infra/storage/migrate/labels_schema_test.go
git commit -m "feat(db): add labels, transactions_labels, and tags.icon"
```

---

### Task 2: Carry `tags.icon` through the tag stack

The new column changes the sqlc-generated `Tag` struct, which breaks the existing whole-struct conversions. This task keeps the tag feature compiling and starts returning `icon` on the wire.

**Files:**
- Modify: `internal/infra/storage/sqlc/query/sqlite/tags.sql`, `.../pgsql/tags.sql`
- Modify: `internal/infra/storage/sqlc/query/sqlite/tag_read.sql`, `.../pgsql/tag_read.sql`
- Modify: `internal/model/tag.go` (add `Icon`), `internal/model/tag_dto.go` (add `Icon` to `TagResult`), `internal/model/tag_view.go` (add `Icon`)
- Modify: `internal/tag/repo/repo.go` (`hydrate`, `Save`), `internal/tag/repo/read.go` (`TagListView`)
- Modify: `internal/tag/usecase.go` (`toResult`, `newTagName` unchanged), `internal/tag/read.go` (`toViewResult`), `internal/tag/create.go` (seed icon)
- Test: `internal/tag/repo/repo_integration_test.go`, `internal/model/tag_test.go`

**Interfaces:**
- Consumes: the `tags.icon` column from Task 1.
- Produces: `model.Tag.Icon string`; `model.TagResult.Icon string` (JSON `icon`); `model.TagViewRow.Icon string`; `model.DefaultTagIcon = "tag"`.

- [ ] **Step 1: Write the failing test**

Add to `internal/model/tag_test.go`:

```go
func TestNewTagSeedsDefaultIcon(t *testing.T) {
	now := time.Now()
	tag := model.NewTag(vo.NewId(), vo.NewId(), "Travel", now)
	if tag.Icon != model.DefaultTagIcon {
		t.Fatalf("icon = %q, want %q", tag.Icon, model.DefaultTagIcon)
	}
	if model.DefaultTagIcon != "tag" {
		t.Fatalf("DefaultTagIcon = %q, want \"tag\"", model.DefaultTagIcon)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/ -run TestNewTagSeedsDefaultIcon -v`
Expected: FAIL — `tag.Icon undefined` / `model.DefaultTagIcon undefined`

- [ ] **Step 3: Add the column to both engines' queries**

In `internal/infra/storage/sqlc/query/sqlite/tags.sql`, add `icon` to every column list, in this position (after `name`), and to the upsert:

```sql
-- name: GetTagByID :one
SELECT id, user_id, name, icon, position, is_archived, created_at, updated_at
FROM tags
WHERE id = ?
;

-- name: ListTagsByOwner :many
SELECT id, user_id, name, icon, position, is_archived, created_at, updated_at
FROM tags
WHERE user_id = ?
ORDER BY position, id
;

-- name: UpsertTag :exec
INSERT INTO tags (id, user_id, name, icon, position, is_archived, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    user_id     = excluded.user_id,
    name        = excluded.name,
    icon        = excluded.icon,
    position    = excluded.position,
    is_archived = excluded.is_archived,
    updated_at  = excluded.updated_at
;
```

Apply the identical edit to `.../pgsql/tags.sql` with `$1..$8` placeholders. In both `tag_read.sql` files add `t.icon` after `t.name` in the `GetTagListView` select list.

- [ ] **Step 4: Regenerate sqlc**

Run: `cd internal/infra/storage/sqlc && sqlc generate`
Expected: `gen/sqlite/models.go` and `gen/pgsql/models.go` both gain `Icon string` on `Tag`, in the same field position.

- [ ] **Step 5: Add `Icon` to the model**

In `internal/model/tag.go`, add the constant and field, and seed it in `NewTag`:

```go
// DefaultTagIcon is the icon every tag is created with. The value is persisted
// per row (not re-derived at render time) so a user-selectable icon later needs
// no schema or rendering change.
const DefaultTagIcon = "tag"

type Tag struct {
	ID         vo.Id
	UserID     vo.Id
	Name       string
	Icon       string
	Position   int16
	IsArchived bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func NewTag(id, userID vo.Id, name string, now time.Time) *Tag {
	return &Tag{ID: id, UserID: userID, Name: name, Icon: DefaultTagIcon, CreatedAt: now, UpdatedAt: now}
}
```

- [ ] **Step 6: Thread `Icon` through DTO, view row, repo, and services**

`internal/model/tag_dto.go` — add to `TagResult` after `Name`:

```go
	Icon        string `json:"icon"`
```

`internal/model/tag_view.go` — add to `TagViewRow` after `Name`:

```go
	Icon       string
```

`internal/tag/repo/repo.go` — in `Save`, add `Icon: t.Icon,` to `upsertParams`; in `hydrate`, add `Icon: row.Icon,` to the returned `&model.Tag{...}`.

`internal/tag/repo/read.go` — in `TagListView`, add `Icon: t.Icon,` to the `model.TagViewRow{...}` literal.

`internal/tag/usecase.go` — in `toResult`, add `Icon: t.Icon,`. `internal/tag/read.go` — in `toViewResult`, add `Icon: r.Icon,`.

Existing rows loaded before this change already have `'tag'` from the migration default, so no fallback branch is needed.

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./internal/model/ ./internal/tag/... -v`
Expected: PASS

- [ ] **Step 8: Regenerate API goldens and inspect**

Run: `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ && git diff --stat internal/test/apiparity/testdata/golden/`
Expected: only tag-related goldens change, each adding `"icon": "tag"`. **Inspect the diff** — any other change means unintended behaviour drift.

- [ ] **Step 9: Commit**

```bash
git add internal/infra/storage/sqlc internal/model/tag.go internal/model/tag_dto.go \
        internal/model/tag_view.go internal/tag internal/test/apiparity/testdata/golden
git commit -m "feat(tag): persist and return a per-row icon"
```

---

### Task 3: sqlc queries for labels

**Files:**
- Create: `internal/infra/storage/sqlc/query/sqlite/labels.sql`, `.../pgsql/labels.sql`
- Create: `internal/infra/storage/sqlc/query/sqlite/label_read.sql`, `.../pgsql/label_read.sql`

**Interfaces:**
- Consumes: the `labels` table from Task 1.
- Produces: generated `GetLabelByID`, `CountLabelsByOwner`, `ListLabelsByOwner`, `UpsertLabel`, `DeleteLabel`, `GetLabelListView` in both `gen/sqlite` and `gen/pgsql`, plus the `Label` model struct.

- [ ] **Step 1: Write the write-side sqlite queries**

Create `internal/infra/storage/sqlc/query/sqlite/labels.sql` (ASCII only — no em dashes):

```sql
-- Write-side queries for the label module. The read-side query lives in
-- label_read.sql to keep the CQRS boundary visible (matching tags.sql vs
-- tag_read.sql). Unlike tags, a label's icon IS persisted from the start.

-- name: GetLabelByID :one
SELECT id, user_id, name, icon, position, is_archived, created_at, updated_at
FROM labels
WHERE id = ?
;

-- name: CountLabelsByOwner :one
-- New-label position = count of the owner's existing labels.
SELECT COUNT(*) FROM labels WHERE user_id = ?
;

-- name: ListLabelsByOwner :many
SELECT id, user_id, name, icon, position, is_archived, created_at, updated_at
FROM labels
WHERE user_id = ?
ORDER BY position, id
;

-- name: UpsertLabel :exec
INSERT INTO labels (id, user_id, name, icon, position, is_archived, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    user_id     = excluded.user_id,
    name        = excluded.name,
    icon        = excluded.icon,
    position    = excluded.position,
    is_archived = excluded.is_archived,
    updated_at  = excluded.updated_at
;

-- name: DeleteLabel :exec
-- transactions_labels rows for this label are removed by ON DELETE CASCADE;
-- unlike tags there is no SET NULL, because the link is a join table.
DELETE FROM labels WHERE id = ?
;
```

- [ ] **Step 2: Write the pgsql sibling**

Create `internal/infra/storage/sqlc/query/pgsql/labels.sql` — identical SQL with `$1..$8` placeholders and the header comment `-- Write-side queries for the label module (PostgreSQL variant: $N placeholders).` Keep the column lists byte-identical to sqlite so the generated structs stay convertible.

- [ ] **Step 3: Write the read-side queries**

Create `internal/infra/storage/sqlc/query/sqlite/label_read.sql`:

```sql
-- Read-model query for the label module (CQRS read side).

-- name: GetLabelListView :many
-- Available labels: the user's OWN labels plus the labels of every user who has
-- shared an account WITH this user. The user id is repeated positionally, which
-- generates a two-field Params struct.
SELECT l.id, l.user_id, l.name, l.icon, l.position, l.is_archived, l.created_at, l.updated_at
FROM labels l
WHERE l.user_id = ?
   OR l.user_id IN (
       SELECT a.user_id
       FROM accounts_access aa
       JOIN accounts a ON a.id = aa.account_id
       WHERE aa.user_id = ? AND aa.is_accepted = 1
   )
ORDER BY l.position, l.id
;
```

Create `internal/infra/storage/sqlc/query/pgsql/label_read.sql` — same, but `$1` **reused** for both positions (keeps the generated param a bare `string`) and `aa.is_accepted = true`.

- [ ] **Step 4: Regenerate and verify it builds**

Run: `cd internal/infra/storage/sqlc && sqlc generate && cd - && go build ./...`
Expected: build succeeds; `gen/sqlite/models.go` and `gen/pgsql/models.go` both contain a `Label` struct with identical field order.

- [ ] **Step 5: Commit**

```bash
git add internal/infra/storage/sqlc
git commit -m "feat(label): add sqlc queries for labels"
```

---

### Task 4: `model.Label` entity

**Files:**
- Create: `internal/model/label.go`
- Test: `internal/model/label_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `model.Label` struct; `model.NewLabel(id, userID vo.Id, name string, now time.Time) *Label`; methods `SetPosition(int16)`, `UpdateName(string, time.Time)`, `UpdatePosition(int16, time.Time)`, `Archive(time.Time)`, `Unarchive(time.Time)`; `model.DefaultLabelIcon = "label"`.

- [ ] **Step 1: Write the failing test**

Create `internal/model/label_test.go`:

```go
package model_test

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

func TestNewLabelSeedsIconAndTimestamps(t *testing.T) {
	now := time.Now()
	l := model.NewLabel(vo.NewId(), vo.NewId(), "Kid A", now)
	if l.Icon != "label" {
		t.Fatalf("icon = %q, want \"label\"", l.Icon)
	}
	if !l.CreatedAt.Equal(now) || !l.UpdatedAt.Equal(now) {
		t.Fatal("timestamps not seeded from now")
	}
	if l.IsArchived {
		t.Fatal("new label must not be archived")
	}
}

func TestLabelMutatorsBumpUpdatedAtOnlyOnChange(t *testing.T) {
	created := time.Now()
	later := created.Add(time.Hour)
	l := model.NewLabel(vo.NewId(), vo.NewId(), "Kid A", created)

	l.UpdateName("Kid A", later)
	if !l.UpdatedAt.Equal(created) {
		t.Fatal("no-op rename must not bump UpdatedAt")
	}
	l.UpdateName("Kid B", later)
	if !l.UpdatedAt.Equal(later) {
		t.Fatal("real rename must bump UpdatedAt")
	}

	l2 := model.NewLabel(vo.NewId(), vo.NewId(), "Pet", created)
	l2.Unarchive(later)
	if !l2.UpdatedAt.Equal(created) {
		t.Fatal("no-op unarchive must not bump UpdatedAt")
	}
	l2.Archive(later)
	if !l2.IsArchived || !l2.UpdatedAt.Equal(later) {
		t.Fatal("archive must set the flag and bump UpdatedAt")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/ -run TestNewLabel -v`
Expected: FAIL — `undefined: model.NewLabel`

- [ ] **Step 3: Write the entity**

Create `internal/model/label.go`:

```go
package model

import (
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// DefaultLabelIcon is the icon every label is created with. Like a tag's, the
// value is persisted per row rather than re-derived at render time, so a
// user-selectable icon later needs no schema or rendering change.
const DefaultLabelIcon = "label"

// Label is the reporting-classification aggregate root. It deliberately has no
// budget role: a label never becomes a budgets_elements row, which is what lets
// several labels on one transaction each count the full amount without
// corrupting envelope math.
type Label struct {
	ID         vo.Id
	UserID     vo.Id
	Name       string
	Icon       string
	Position   int16
	IsArchived bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func NewLabel(id, userID vo.Id, name string, now time.Time) *Label {
	return &Label{ID: id, UserID: userID, Name: name, Icon: DefaultLabelIcon, CreatedAt: now, UpdatedAt: now}
}

// SetPosition sets the initial position at creation; it is part of
// construction, so it does not bump UpdatedAt.
func (l *Label) SetPosition(position int16) { l.Position = position }

func (l *Label) UpdateName(name string, now time.Time) {
	if l.Name != name {
		l.Name = name
		l.UpdatedAt = now
	}
}

func (l *Label) UpdatePosition(position int16, now time.Time) {
	if l.Position != position {
		l.Position = position
		l.UpdatedAt = now
	}
}

func (l *Label) Archive(now time.Time) {
	if !l.IsArchived {
		l.IsArchived = true
		l.UpdatedAt = now
	}
}

func (l *Label) Unarchive(now time.Time) {
	if l.IsArchived {
		l.IsArchived = false
		l.UpdatedAt = now
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/model/ -run Label -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/model/label.go internal/model/label_test.go
git commit -m "feat(label): add the Label entity"
```

---

### Task 5: Label DTOs, view row, and error codes

**Files:**
- Create: `internal/model/label_dto.go`, `internal/model/label_view.go`
- Modify: `internal/shared/errs/codes.go`
- Modify: `locales/en.json`, `locales/ru.json` (and every other shipped `locales/*.json` — the i18n guard requires key parity across all languages)

**Interfaces:**
- Consumes: `model.Label`.
- Produces: `LabelResult`, `CreateLabelRequest`/`CreateLabelResult`, `UpdateLabelRequest`/`UpdateLabelResult`, `ArchiveLabelRequest`/`ArchiveLabelResult`, `UnarchiveLabelRequest`/`UnarchiveLabelResult`, `DeleteLabelRequest`/`DeleteLabelResult`, `OrderLabelListRequest`/`OrderLabelListResult`, `GetLabelListResult`, `model.LabelViewRow`; error codes `errs.CodeLabelNameLength`, `errs.CodeLabelAlreadyExists`, `errs.CodeLabelListEmpty`.

- [ ] **Step 1: Write the failing test**

Create `internal/model/label_dto_test.go`:

```go
package model_test

import (
	"testing"

	"github.com/econumo/econumo/internal/model"
)

func TestCreateLabelRequestValidate(t *testing.T) {
	if err := (model.CreateLabelRequest{Id: "", Name: "Kid"}).Validate(); err == nil {
		t.Fatal("blank id must fail validation")
	}
	if err := (model.CreateLabelRequest{Id: "op", Name: "  "}).Validate(); err == nil {
		t.Fatal("blank name must fail validation")
	}
	if err := (model.CreateLabelRequest{Id: "op", Name: "Kid"}).Validate(); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
}

func TestOrderLabelListRequestRejectsEmpty(t *testing.T) {
	if err := (model.OrderLabelListRequest{}).Validate(); err == nil {
		t.Fatal("empty changes list must fail validation")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/ -run Label -v`
Expected: FAIL — `undefined: model.CreateLabelRequest`

- [ ] **Step 3: Add the error codes**

In `internal/shared/errs/codes.go`, add next to the tag codes:

```go
	CodeLabelNameLength    = "label.name_length"
	CodeLabelAlreadyExists = "label.already_exists"
	CodeLabelListEmpty     = "label.list_empty"
```

and add all three to the known-codes slice (`AllCodes`) in the same file.

- [ ] **Step 4: Write the DTOs**

Create `internal/model/label_dto.go`. It mirrors `tag_dto.go` exactly; `LabelResult` carries `icon` from the start:

```go
package model

import (
	"strings"

	"github.com/econumo/econumo/internal/shared/errs"
)

// LabelResult is one label in the API. isArchived is an int 0/1 (NOT bool) and
// the timestamps are "2006-01-02 15:04:05" - the same frozen wire shapes as
// TagResult.
type LabelResult struct {
	Id          string `json:"id"`
	OwnerUserId string `json:"ownerUserId"`
	Name        string `json:"name"`
	Icon        string `json:"icon"`
	Position    int    `json:"position"`
	IsArchived  int    `json:"isArchived"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// CreateLabelRequest carries the same optional accountId as CreateTagRequest: a
// label added in the context of a shared account belongs to the account OWNER,
// not the caller.
type CreateLabelRequest struct {
	Id        string  `json:"id"`
	Name      string  `json:"name"`
	AccountId *string `json:"accountId"`
}

func (r CreateLabelRequest) Validate() error {
	var fields []errs.FieldError
	if strings.TrimSpace(r.Id) == "" {
		fields = append(fields, errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	if strings.TrimSpace(r.Name) == "" {
		fields = append(fields, errs.FieldError{Key: "name", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

type CreateLabelResult struct {
	Item LabelResult `json:"item"`
}

type UpdateLabelRequest struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

func (r UpdateLabelRequest) Validate() error {
	var fields []errs.FieldError
	if strings.TrimSpace(r.Id) == "" {
		fields = append(fields, errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	if strings.TrimSpace(r.Name) == "" {
		fields = append(fields, errs.FieldError{Key: "name", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

type UpdateLabelResult struct {
	Item LabelResult `json:"item"`
}

type ArchiveLabelRequest struct {
	Id string `json:"id"`
}

func (r ArchiveLabelRequest) Validate() error {
	if strings.TrimSpace(r.Id) == "" {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	return nil
}

type ArchiveLabelResult struct{}

type UnarchiveLabelRequest struct {
	Id string `json:"id"`
}

func (r UnarchiveLabelRequest) Validate() error {
	if strings.TrimSpace(r.Id) == "" {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	return nil
}

type UnarchiveLabelResult struct{}

type DeleteLabelRequest struct {
	Id string `json:"id"`
}

func (r DeleteLabelRequest) Validate() error {
	if strings.TrimSpace(r.Id) == "" {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	return nil
}

type DeleteLabelResult struct{}

type OrderLabelListRequest struct {
	Changes []PositionChange `json:"changes"`
}

func (r OrderLabelListRequest) Validate() error {
	if len(r.Changes) == 0 {
		return &errs.ValidationError{Msg: "Labels list is empty", MsgCode: errs.CodeLabelListEmpty}
	}
	var fields []errs.FieldError
	for _, c := range r.Changes {
		fields = append(fields, validatePositionField("position", c.Position)...)
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

type OrderLabelListResult struct {
	Items []LabelResult `json:"items"`
}

type GetLabelListResult struct {
	Items []LabelResult `json:"items"`
}
```

`PositionChange` and `validatePositionField` already exist in `category_dto.go` / `budget_dto.go` — **do not redeclare them**.

- [ ] **Step 5: Write the view row**

Create `internal/model/label_view.go`:

```go
package model

// LabelViewRow is the read-side row shape the label ReadModel returns. It is
// declared here so the app layer does not import infra.
type LabelViewRow struct {
	ID         string
	UserID     string
	Name       string
	Icon       string
	Position   int16
	IsArchived bool
	CreatedAt  string // already formatted "2006-01-02 15:04:05" by the repo
	UpdatedAt  string
}
```

- [ ] **Step 6: Add the error catalogue keys**

In `locales/en.json`, add a `label` block under `errors` mirroring `errors.tag`:

```json
"label": {
  "name_length": "Label name must be {min}-{max} characters",
  "already_exists": "Label already exists.",
  "list_empty": "Labels list is empty"
},
```

Add the same three keys, translated, to **every** other `locales/*.json`. The i18n guard asserts two-way coverage between `errs.AllCodes` and the `errors.*` catalogue keys, plus `{var}` placeholder-set parity per key — `name_length` must keep both `{min}` and `{max}` in every language.

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./internal/model/ ./internal/test/i18ntest/ -v`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/model/label_dto.go internal/model/label_view.go internal/model/label_dto_test.go \
        internal/shared/errs/codes.go locales
git commit -m "feat(label): add label DTOs, view row, and error codes"
```

---

### Task 6: Label repository

**Files:**
- Create: `internal/label/repository.go`, `internal/label/ports.go`
- Create: `internal/label/repo/repo.go`, `internal/label/repo/sqlite.go`, `internal/label/repo/pgsql.go`, `internal/label/repo/read.go`
- Test: `internal/label/repo/repo_integration_test.go`

**Interfaces:**
- Consumes: `model.Label`, `model.LabelViewRow`, generated `GetLabelByID`/`ListLabelsByOwner`/`CountLabelsByOwner`/`UpsertLabel`/`DeleteLabel`/`GetLabelListView`.
- Produces: `label.Repository` interface (`NextIdentity`, `GetByID`, `ListByOwner`, `CountByOwner`, `Save`, `Delete`); `label.ReadModel` interface (`LabelListView(ctx, userID string) ([]model.LabelViewRow, error)`); `label.AccountAccess` port (`AccountOwner`, `HasAdminGrant`); `labelrepo.NewRepo(driver string, tx *backend.TxManager) *Repo`; `labelrepo.NewReadRepo(driver string, tx *backend.TxManager) *ReadRepo`.

- [ ] **Step 1: Write the failing test**

Create `internal/label/repo/repo_integration_test.go`:

```go
package repo_test

import (
	"context"
	"testing"

	labelrepo "github.com/econumo/econumo/internal/label/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestLabelRepoSaveLoadDelete(t *testing.T) {
	db := dbtest.New(t)
	txm := dbtest.TxManager(t, db)
	r := labelrepo.NewRepo(dbtest.Driver(), txm)
	ctx := context.Background()
	user := fixture.User(t, db)

	l := model.NewLabel(vo.NewId(), user.ID, "Kid A", dbtest.FixedTime())
	l.SetPosition(0)
	if err := r.Save(ctx, l); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := r.GetByID(ctx, l.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "Kid A" || got.Icon != model.DefaultLabelIcon {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}

	n, err := r.CountByOwner(ctx, user.ID)
	if err != nil || n != 1 {
		t.Fatalf("count = %d, err = %v; want 1, nil", n, err)
	}

	if err := r.Delete(ctx, l.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := r.GetByID(ctx, l.ID); err == nil {
		t.Fatal("expected NotFound after delete")
	}
}
```

Adjust the `dbtest`/`fixture` helper names to match the existing `internal/tag/repo/repo_integration_test.go` — read that file first and copy its harness calls verbatim.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/label/repo/ -v`
Expected: FAIL — package `internal/label/repo` does not exist

- [ ] **Step 3: Write the domain interfaces**

Create `internal/label/repository.go`:

```go
package label

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Repository is the label aggregate's persistence port. A missing label returns
// an *errs.NotFoundError so the HTTP layer maps it consistently.
type Repository interface {
	NextIdentity() vo.Id
	GetByID(ctx context.Context, id vo.Id) (*model.Label, error)
	ListByOwner(ctx context.Context, userID vo.Id) ([]*model.Label, error)
	CountByOwner(ctx context.Context, userID vo.Id) (int, error)
	Save(ctx context.Context, l *model.Label) error
	// Delete removes a label; its transactions_labels rows go with it via the
	// ON DELETE CASCADE FK.
	Delete(ctx context.Context, id vo.Id) error
}
```

Create `internal/label/ports.go`:

```go
// Ports: the consumer-side interfaces this feature declares for capabilities
// other features provide. Implementations are wired in internal/server.
package label

import (
	"context"

	"github.com/econumo/econumo/internal/shared/vo"
)

// AccountAccess resolves shared-account ownership for the create-for-account
// path, so a label created against a shared account is owned by that account's
// owner rather than by the caller.
type AccountAccess interface {
	AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error)
	HasAdminGrant(ctx context.Context, accountID, userID vo.Id) (bool, error)
}
```

- [ ] **Step 4: Write the repo, engine adapters, and read repo**

Create `internal/label/repo/repo.go` mirroring `internal/tag/repo/repo.go`, with `Icon` carried in both directions:

```go
// Package repo implements label.Repository.
package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	domlabel "github.com/econumo/econumo/internal/label"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type (
	labelRow     = sqlitegen.Label
	upsertParams = sqlitegen.UpsertLabelParams
)

type querier interface {
	GetLabelByID(ctx context.Context, db backend.DBTX, id string) (labelRow, error)
	ListLabelsByOwner(ctx context.Context, db backend.DBTX, userID string) ([]labelRow, error)
	CountLabelsByOwner(ctx context.Context, db backend.DBTX, userID string) (int64, error)
	UpsertLabel(ctx context.Context, db backend.DBTX, p upsertParams) error
	DeleteLabel(ctx context.Context, db backend.DBTX, id string) error
}

type Repo struct {
	tx *backend.TxManager
	q  querier
}

var _ domlabel.Repository = (*Repo)(nil)

func NewRepo(driver string, tx *backend.TxManager) *Repo {
	switch driver {
	case "sqlite":
		return &Repo{tx: tx, q: sqliteQuerier{}}
	case "postgresql":
		return &Repo{tx: tx, q: pgsqlQuerier{}}
	default:
		panic("labelrepo: unknown database driver " + driver)
	}
}

func (r *Repo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

func (r *Repo) NextIdentity() vo.Id { return vo.NewId() }

func (r *Repo) GetByID(ctx context.Context, id vo.Id) (*model.Label, error) {
	row, err := r.q.GetLabelByID(ctx, r.db(ctx), id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Label not found")
		}
		return nil, err
	}
	return hydrate(row)
}

func (r *Repo) ListByOwner(ctx context.Context, userID vo.Id) ([]*model.Label, error) {
	rows, err := r.q.ListLabelsByOwner(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	out := make([]*model.Label, 0, len(rows))
	for _, row := range rows {
		l, herr := hydrate(row)
		if herr != nil {
			return nil, herr
		}
		out = append(out, l)
	}
	return out, nil
}

func (r *Repo) CountByOwner(ctx context.Context, userID vo.Id) (int, error) {
	n, err := r.q.CountLabelsByOwner(ctx, r.db(ctx), userID.String())
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

func (r *Repo) Save(ctx context.Context, l *model.Label) error {
	return r.q.UpsertLabel(ctx, r.db(ctx), upsertParams{
		ID:         l.ID.String(),
		UserID:     l.UserID.String(),
		Name:       l.Name,
		Icon:       l.Icon,
		Position:   l.Position,
		IsArchived: l.IsArchived,
		CreatedAt:  l.CreatedAt,
		UpdatedAt:  l.UpdatedAt,
	})
}

func (r *Repo) Delete(ctx context.Context, id vo.Id) error {
	return r.q.DeleteLabel(ctx, r.db(ctx), id.String())
}

func hydrate(row labelRow) (*model.Label, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	userID, err := vo.ParseId(row.UserID)
	if err != nil {
		return nil, err
	}
	return &model.Label{ID: id, UserID: userID, Name: row.Name, Icon: row.Icon,
		Position: row.Position, IsArchived: row.IsArchived,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}, nil
}
```

Create `internal/label/repo/sqlite.go` — a `sqliteQuerier struct{}` with one passthrough method per querier method, each calling `sqlitegen.New(db).<Method>(ctx, ...)`. Create `internal/label/repo/pgsql.go` — a `pgsqlQuerier struct{}` whose methods call `pgsqlgen.New(db).<Method>` and convert with `labelRow(l)` / `pgsqlgen.UpsertLabelParams(p)`, looping to convert slices. Copy the exact shape of `internal/tag/repo/sqlite.go` and `pgsql.go`.

Create `internal/label/repo/read.go` mirroring `internal/tag/repo/read.go`: a `readQuerier` interface with `GetLabelListView`, a `ReadRepo` implementing `domlabel.ReadModel`, formatting timestamps with `datetime.Layout`, and the two engine querier types (sqlite passes `sqlitegen.GetLabelListViewParams{UserID: userID, UserID_2: userID}`; pgsql passes the bare `userID` and converts rows).

Add the read port to `internal/label/read.go` in Task 7.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/label/... -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/label
git commit -m "feat(label): add the label repository and engine adapters"
```

---

### Task 7: Label service (write + read use cases)

**Files:**
- Create: `internal/label/usecase.go`, `internal/label/create.go`, `internal/label/update.go`, `internal/label/archive.go`, `internal/label/delete.go`, `internal/label/order.go`, `internal/label/read.go`
- Test: `internal/label/create_test.go`, `internal/label/access_test.go`

**Interfaces:**
- Consumes: `label.Repository`, `label.AccountAccess`, `port.TxRunner`, `port.OperationGuard`, `port.Clock`, `label.ReadModel`.
- Produces: `label.NewService(repo Repository, tx port.TxRunner, ops port.OperationGuard, clock port.Clock, read ReadModel, access AccountAccess) *Service` with methods `CreateLabel`, `UpdateLabel`, `ArchiveLabel`, `UnarchiveLabel`, `DeleteLabel`, `OrderLabelList`; `label.NewReadService(read ReadModel) *ReadService` with `GetLabelList(ctx, userID vo.Id) (*model.GetLabelListResult, error)`.

- [ ] **Step 1: Write the failing test**

Create `internal/label/create_test.go` (model it on the existing `internal/tag` service tests — read those first for the stub harness):

```go
func TestCreateLabelIsIdempotentOnOperationID(t *testing.T) {
	// second create with the same request id must fail with "Operation is locked"
}

func TestCreateLabelRejectsDuplicateNamePerOwner(t *testing.T) {
	// second create with the same name for the same owner must fail
	// with "Label already exists."
}

func TestCreateLabelForSharedAccountAssignsOwner(t *testing.T) {
	// accountId belonging to another user, caller holds an admin grant ->
	// the created label's UserID is the ACCOUNT OWNER, not the caller
}

func TestCreateLabelNameLengthBoundaries(t *testing.T) {
	// 2 runes -> error "Label name must be 3-64 characters"; 3 and 64 runes -> ok;
	// 65 runes -> error
}
```

Fill each body using the stubs from the tag tests.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/label/ -v`
Expected: FAIL — `undefined: label.NewService`

- [ ] **Step 3: Write `usecase.go`**

Create `internal/label/usecase.go` mirroring `internal/tag/usecase.go` — `Service` struct, `NewService`, `resolveAccountOwner`, `mutate`, `mutateChecked`, `toResult`, `listResults`, `ensureNameUnique`, `newLabelName`. Three things differ from tag:

```go
// newLabelName enforces the label name invariant: rune length 3..64. The
// message is exactly "Label name must be 3-64 characters" and the field key is
// "name".
func newLabelName(v string) (string, error) {
	n := len([]rune(v))
	if n < 3 || n > 64 {
		return "", errs.NewValidation("Label name must be 3-64 characters",
			errs.FieldError{
				Key: "name", Message: "Label name must be 3-64 characters", Code: errs.CodeLabelNameLength,
				Params: map[string]any{"min": 3, "max": 64},
			})
	}
	return v, nil
}
```

`ensureNameUnique` returns `&errs.ValidationError{Msg: "Label already exists.", MsgCode: errs.CodeLabelAlreadyExists}`; `mutate`/`mutateChecked` mask a foreign-owned label as `errs.NewNotFound("Label not found")`; `toResult` maps `IsArchived` to int 0/1 and formats timestamps with `datetime.Layout`, including `Icon: l.Icon`.

- [ ] **Step 4: Write the use cases**

Create `create.go`, `update.go`, `archive.go`, `delete.go`, `order.go` as direct mirrors of the tag equivalents, substituting `Label` for `Tag` throughout. `CreateLabel` seeds position from `CountByOwner` and claims the operation id via `s.ops.Claim` inside the tx exactly as `CreateTag` does.

Create `internal/label/read.go`:

```go
package label

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// ReadModel is the read-side data source, implemented by the infra ReadRepo.
type ReadModel interface {
	LabelListView(ctx context.Context, userID string) ([]model.LabelViewRow, error)
}

type ReadService struct {
	read ReadModel
}

func NewReadService(read ReadModel) *ReadService { return &ReadService{read: read} }

func (s *ReadService) GetLabelList(ctx context.Context, userID vo.Id) (*model.GetLabelListResult, error) {
	rows, err := s.read.LabelListView(ctx, userID.String())
	if err != nil {
		return nil, err
	}
	items := make([]model.LabelResult, 0, len(rows))
	for _, r := range rows {
		items = append(items, toViewResult(r))
	}
	return &model.GetLabelListResult{Items: items}, nil
}

func toViewResult(r model.LabelViewRow) model.LabelResult {
	archived := 0
	if r.IsArchived {
		archived = 1
	}
	return model.LabelResult{
		Id:          r.ID,
		OwnerUserId: r.UserID,
		Name:        r.Name,
		Icon:        r.Icon,
		Position:    int(r.Position),
		IsArchived:  archived,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/label/... -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/label
git commit -m "feat(label): add label write and read use cases"
```

---

### Task 8: Label HTTP edge and server wiring

**Files:**
- Create: `internal/label/api/handler.go`, `internal/label/api/label.go`, `internal/label/api/labellist.go`, `internal/label/api/routes.go`
- Modify: `internal/server/server.go`
- Test: `internal/label/api/harness_test.go`, `internal/label/api/label_endpoints_test.go`, `internal/label/api/shared_access_test.go`
- Modify: `internal/test/apiparity/` scenarios + goldens

**Interfaces:**
- Consumes: `label.Service`, `label.ReadService`, `middleware.TokenAuthenticator`.
- Produces: `labelapi.NewHandlers(svc *applabel.Service, read *applabel.ReadService) *Handlers`; `labelapi.RegisterAPI(h *Handlers, authn middleware.TokenAuthenticator) router.RegisterAPI`; 7 routes under `/api/v1/label/`.

- [ ] **Step 1: Write the failing test**

Create `internal/label/api/label_endpoints_test.go`, mirroring `internal/tag/api/tag_endpoints_test.go`. Cover: create returns the item with `"icon":"label"` and `"isArchived":0`; update renames; archive/unarchive return `{}`; delete returns `{}`; `get-label-list` returns own + shared labels ordered by position; a foreign label id returns 404 `"Label not found"`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/label/api/ -v`
Expected: FAIL — package does not exist

- [ ] **Step 3: Write the handlers**

Create `internal/label/api/handler.go`:

```go
// Package api wires the label module's HTTP edge.
package api

import (
	applabel "github.com/econumo/econumo/internal/label"
	"github.com/econumo/econumo/internal/web/apidoc"
)

// _ keeps the apidoc import alias visible to swag's annotation parser.
var _ = apidoc.JsonResponseOk{}

type Handlers struct {
	svc  *applabel.Service
	read *applabel.ReadService
}

func NewHandlers(svc *applabel.Service, read *applabel.ReadService) *Handlers {
	return &Handlers{svc: svc, read: read}
}
```

Create `internal/label/api/label.go` with `CreateLabel`, `UpdateLabel`, `ArchiveLabel`, `UnarchiveLabel`, `DeleteLabel` — each a one-liner delegating to `endpoint.Handle(w, r, h.svc.<Method>)`, each preceded by the full swag annotation block. Copy the annotation shape from `internal/tag/api/tag.go`, changing `Tags Tag` → `Tags Label`, the router path, and the model types. Example:

```go
// CreateLabel handles POST /api/v1/label/create-label (auth).
//
// @Summary     Create a label
// @Description Creates a reporting label for the authenticated user. Idempotent on the request id; the name must be unique among the user's labels.
// @Tags        Label
// @Accept      json
// @Produce     json
// @Param       request body     model.CreateLabelRequest true "Create label request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.CreateLabelResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/label/create-label [post]
func (h *Handlers) CreateLabel(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.CreateLabel)
}
```

Create `internal/label/api/labellist.go` with `OrderLabelList` (`endpoint.Handle`) and `GetLabelList` (`endpoint.HandleNoBody`, delegating to `h.read.GetLabelList`).

Create `internal/label/api/routes.go`:

```go
package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/web/middleware"
	"github.com/econumo/econumo/internal/web/router"
)

func RegisterAPI(h *Handlers, authn middleware.TokenAuthenticator) router.RegisterAPI {
	return func(mux *http.ServeMux) {
		authMw := middleware.Auth(authn)
		auth := func(fn http.HandlerFunc) http.Handler { return authMw(fn) }

		mux.Handle("POST /api/v1/label/create-label", auth(h.CreateLabel))
		mux.Handle("POST /api/v1/label/update-label", auth(h.UpdateLabel))
		mux.Handle("POST /api/v1/label/archive-label", auth(h.ArchiveLabel))
		mux.Handle("POST /api/v1/label/unarchive-label", auth(h.UnarchiveLabel))
		mux.Handle("POST /api/v1/label/delete-label", auth(h.DeleteLabel))
		mux.Handle("POST /api/v1/label/order-label-list", auth(h.OrderLabelList))
		mux.Handle("GET /api/v1/label/get-label-list", auth(h.GetLabelList))
	}
}
```

- [ ] **Step 4: Wire it in the composition root**

In `internal/server/server.go`, add the imports:

```go
	applabel "github.com/econumo/econumo/internal/label"
	handlerlabel "github.com/econumo/econumo/internal/label/api"
	labelrepo "github.com/econumo/econumo/internal/label/repo"
```

Next to the tag construction block (which builds `tagRepo`/`tagSvc` and already has `opGuard`, `accountAccessResolver`, `clk`, `txm` in scope), add:

```go
	labelRepo := labelrepo.NewRepo(cfg.DatabaseDriver, txm)
	labelReadRepo := labelrepo.NewReadRepo(cfg.DatabaseDriver, txm)
	labelSvc := applabel.NewService(labelRepo, txm, opGuard, clk, labelReadRepo, accountAccessResolver)
	labelReadSvc := applabel.NewReadService(labelReadRepo)
	labelHandlers := handlerlabel.NewHandlers(labelSvc, labelReadSvc)
```

And inside `router.Compose(...)`, next to the tag line:

```go
		handlerlabel.RegisterAPI(labelHandlers, authn),
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/label/... ./internal/server/... -v`
Expected: PASS

- [ ] **Step 6: Add apiparity scenarios and regenerate goldens**

Add one scenario per new route to the apiparity catalogue (follow the existing tag scenarios). The guard tests require every registered route to have a scenario and the counts never to shrink.

Run: `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ && git diff internal/test/apiparity/testdata/golden/`
Expected: 7 new goldens for the label routes; **inspect them** — confirm `"icon":"label"`, `"isArchived":0`, and the `2006-01-02 15:04:05` timestamps.

- [ ] **Step 7: Regenerate OpenAPI docs and run the full smoke tier**

Run: `make swagger && make go-test`
Expected: PASS, coverage ≥ `GO_COVER_MIN`

- [ ] **Step 8: Commit**

```bash
git add internal/label internal/server/server.go internal/test/apiparity docs
git commit -m "feat(label): expose the label REST API and wire it in BuildAPI"
```

---

### Task 9: Attach labels to a transaction (entity + persistence)

**Files:**
- Modify: `internal/model/transaction.go` (`Transaction`, `NewState`, `New`, `FromState`, `Update`)
- Create: `internal/infra/storage/sqlc/query/sqlite/transaction_labels.sql`, `.../pgsql/transaction_labels.sql`
- Modify: `internal/transaction/repo/repo.go`
- Test: `internal/model/transaction_test.go`, `internal/transaction/repo/repo_integration_test.go`

**Interfaces:**
- Consumes: `transactions_labels` table.
- Produces: `model.Transaction.LabelIDs []vo.Id` (nil/empty = none), `model.NewState.LabelIDs []vo.Id`; repo methods `ReplaceLabels(ctx, transactionID vo.Id, labelIDs []vo.Id) error` and `LabelsByTransactionIDs(ctx, ids []vo.Id) (map[string][]string, error)`.

- [ ] **Step 1: Write the failing test**

Add to `internal/model/transaction_test.go`:

```go
func TestUpdateClearsLabelsOnTransfer(t *testing.T) {
	now := time.Now()
	labels := []vo.Id{vo.NewId(), vo.NewId()}
	tx := model.New(model.NewState{
		ID: vo.NewId(), UserID: vo.NewId(), Type: model.TransactionTypeExpense,
		AccountID: vo.NewId(), Amount: "10", LabelIDs: labels,
		SpentAt: now, CreatedAt: now,
	})
	if len(tx.LabelIDs) != 2 {
		t.Fatalf("expense should keep labels, got %d", len(tx.LabelIDs))
	}

	recip := vo.NewId()
	tx.Update(model.NewState{
		Type: model.TransactionTypeTransfer, AccountID: tx.AccountID,
		AccountRecipID: &recip, Amount: "10", LabelIDs: labels, SpentAt: now,
	}, now)
	if len(tx.LabelIDs) != 0 {
		t.Fatalf("transfer must clear labels, got %d", len(tx.LabelIDs))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/ -run TestUpdateClearsLabels -v`
Expected: FAIL — `unknown field LabelIDs`

- [ ] **Step 3: Add `LabelIDs` to the entity**

In `internal/model/transaction.go`, add `LabelIDs []vo.Id` to both `Transaction` and `NewState`; carry it in `New` and `FromState`; and in `Update`, set it in **both** branches — `t.LabelIDs = nil` in the transfer branch (alongside the existing `t.TagID = nil`) and `t.LabelIDs = s.LabelIDs` in the non-transfer branch.

- [ ] **Step 4: Write the join-table queries**

Create `internal/infra/storage/sqlc/query/sqlite/transaction_labels.sql`:

```sql
-- Link rows between a transaction and its reporting labels. Writes are
-- delete-then-insert inside the caller's transaction, so a re-save is
-- idempotent and never duplicates a pair.

-- name: DeleteTransactionLabels :exec
DELETE FROM transactions_labels WHERE transaction_id = ?
;

-- name: InsertTransactionLabel :exec
INSERT INTO transactions_labels (transaction_id, label_id)
VALUES (?, ?)
ON CONFLICT (transaction_id, label_id) DO NOTHING
;

-- name: ListLabelIDsByTransaction :many
SELECT label_id FROM transactions_labels WHERE transaction_id = ?
;
```

Create the pgsql sibling with `$1`/`$2` placeholders.

- [ ] **Step 5: Implement the repo methods**

In `internal/transaction/repo/repo.go`, extend the `querier` interface with the three new methods, implement them in both `sqliteQuerier` and `pgsqlQuerier`, then add to `Repo`:

```go
// ReplaceLabels rewrites a transaction's label links. The caller runs this
// inside the same tx as Save, so the delete-then-insert is atomic.
func (r *Repo) ReplaceLabels(ctx context.Context, transactionID vo.Id, labelIDs []vo.Id) error {
	db := r.db(ctx)
	if err := r.q.DeleteTransactionLabels(ctx, db, transactionID.String()); err != nil {
		return err
	}
	for _, id := range labelIDs {
		if err := r.q.InsertTransactionLabel(ctx, db, insertLabelParams{
			TransactionID: transactionID.String(),
			LabelID:       id.String(),
		}); err != nil {
			return err
		}
	}
	return nil
}
```

For reads, add a batch loader (`hydrate` is per-row and takes no ctx, so labels must be attached after hydration):

```go
// LabelsByTransactionIDs batch-loads label ids for many transactions, keyed by
// transaction id. Loading per row would be an N+1 on every list endpoint.
func (r *Repo) LabelsByTransactionIDs(ctx context.Context, ids []vo.Id) (map[string][]string, error)
```

Implement it with a single `IN (...)` query built with the existing `placeholders(r.driver, start, n)` helper (mirroring `ListByAccountIDs`), returning `map[transactionID][]labelID`.

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/model/ ./internal/transaction/... -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/model/transaction.go internal/infra/storage/sqlc internal/transaction
git commit -m "feat(transaction): persist many labels per transaction"
```

---

### Task 10: `labelIds` on the transaction API

**Files:**
- Modify: `internal/model/transaction_dto.go` (`TransactionResult`, `CreateTransactionRequest`, `UpdateTransactionRequest`)
- Modify: `internal/transaction/` create/update use cases and the result assembler
- Test: `internal/transaction/api/` endpoint tests
- Modify: `internal/test/apiparity/` goldens

**Interfaces:**
- Consumes: `model.Transaction.LabelIDs`, `Repo.ReplaceLabels`, `Repo.LabelsByTransactionIDs`.
- Produces: wire field `labelIds []string` on create/update requests and on `TransactionResult`.

- [ ] **Step 1: Write the failing test**

Add to the transaction API tests: creating a transaction with `"labelIds": ["<id1>","<id2>"]` returns those ids in the response; updating with `"labelIds": []` clears them; a transfer with `labelIds` returns them empty; an unknown label id is rejected; a label owned by another user is rejected.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/transaction/api/ -v`
Expected: FAIL — unknown field `labelIds`

- [ ] **Step 3: Add the wire fields**

In `internal/model/transaction_dto.go`, add to `TransactionResult` (after `TagId`):

```go
	// LabelIds is always a list, never null, so clients can iterate without a
	// nil check.
	LabelIds []string `json:"labelIds"`
```

Add `LabelIds []string \`json:"labelIds"\`` to `CreateTransactionRequest` and `UpdateTransactionRequest`.

- [ ] **Step 4: Resolve and validate in the service**

In the create/update use cases: parse each `labelIds` entry with `vo.ParseId`; load them via the label repo (a new consumer-side port on the transaction feature, e.g. `LabelOwnership interface { LabelOwners(ctx, ids []vo.Id) (map[string]vo.Id, error) }`, wired in `internal/server` — features never import features); reject an id whose owner is not the transaction's account owner; then call `ReplaceLabels` inside the same tx as `Save`. Dedupe ids before writing.

When building `TransactionResult`, always emit a non-nil slice: `labelIDs := []string{}` before appending.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/transaction/... -v`
Expected: PASS

- [ ] **Step 6: Regenerate goldens and inspect**

Run: `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ && git diff internal/test/apiparity/testdata/golden/`
Expected: every transaction golden gains `"labelIds": []`. **Inspect** — no other field should move.

- [ ] **Step 7: Run the full smoke tier and commit**

```bash
make swagger && make go-test
git add internal/model internal/transaction internal/server internal/test/apiparity docs
git commit -m "feat(transaction): accept and return labelIds"
```

---

### Task 11: PostgreSQL parity check

**Files:** none (verification only)

- [ ] **Step 1: Run the repo suite against PostgreSQL**

Run: `make test-repo-pgsql`
Expected: PASS — this exercises the pgsql adapters and generated queries for labels, not just the parity catalogue.

- [ ] **Step 2: Run the engine-comparison suite**

Run: `make test`
Expected: PASS — sqlite and PostgreSQL return byte-identical responses for every scenario, including the new label routes.

- [ ] **Step 3: Commit any golden or fixture adjustments**

```bash
git add -A
git commit -m "test(label): confirm sqlite/pgsql parity for the label surface"
```

---

## Self-Review

**Spec coverage (phases 1–2):** `labels` table + `tags.icon` + join table → Task 1; sqlc → Tasks 2–3; entity → Task 4; DTOs/error codes/i18n → Task 5; repo → Task 6; use cases incl. shared-account ownership → Task 7; REST + wiring + goldens → Task 8; transaction attachment → Tasks 9–10; engine parity → Task 11. **Deferred to later plans by design:** budget surfacing + drill-down (Plan 2), recurring (Plan 2), CSV (Plan 2), MCP/analytics/frontend (Plans 2–3).

**Known gaps handed to Plan 2/3:**
- `internal/label/mcp/` is **not** in this plan — it lands with the other cross-cutting work so `mcpparity` regenerates once.
- The `recurring_transactions_labels` table is deliberately **not** created here; it ships with the recurring phase so its migration and posting logic are reviewed together.

**Type consistency:** `LabelIDs` (Go, `[]vo.Id`) vs `labelIds` (wire, `[]string`) vs `LabelIds` (DTO field) are used consistently above. `label.ReadModel.LabelListView` matches the `ReadRepo` method name in Task 6. `DefaultLabelIcon`/`DefaultTagIcon` are referenced identically in Tasks 2, 4, and 6.
