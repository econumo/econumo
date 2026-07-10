# Avatar Icons (replacing Gravatar) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace Gravatar avatars with a user-selectable Material icon + background color, stored as one `"<icon>:<color>"` string in the repurposed `avatar` field/column.

**Architecture:** The `users.avatar_url` column is renamed to `avatar` and backfilled to `face:fuchsia`; registration assigns a random default through an injectable `AvatarPicker` seam (deterministic stub in tests); a new `POST /api/v1/user/update-avatar` endpoint stores the joined value; the SPA renders every avatar through one new `UserAvatar` component (Material `EntityIcon` on a colored square) and picks via a dialog reusing the existing `IconPicker`.

**Tech Stack:** Go (stdlib HTTP, sqlc, dbtest/apiparity/enginecompare suites), React 19 + Vite + vitest, Tailwind 4, Material Symbols font (already bundled), TanStack Query.

**Spec:** `docs/superpowers/specs/2026-07-09-avatar-icons-design.md`

## Global Constraints

- Wire value format: `"<icon>:<color>"`, e.g. `"face:fuchsia"`. The JSON field stays named `avatar` in every embed.
- Standard backfill/stub value: `face:fuchsia` (exactly this string).
- Color allowlist (order is contract): `red, orange, amber, yellow, lime, green, emerald, teal, cyan, sky, blue, indigo, violet, purple, fuchsia, pink`.
- Icons are NOT allowlisted server-side; format check only: `^[a-z0-9_]{1,64}$`.
- Exact validation strings: blank → `"This value should not be blank."` code `IS_BLANK_ERROR`; bad icon format → `"This value is not valid."` code `INVALID_FORMAT_ERROR`; unknown color → `"The value you selected is not a valid choice."` code `NO_SUCH_CHOICE_ERROR`.
- Never hand-edit golden files; regenerate with `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/` and inspect the diff.
- Comments sparingly, per CLAUDE.md; keep swag `@` blocks.
- Go verification per task: `go build ./... && go vet ./... && gofmt -l . | grep -v web/` (expect no output from gofmt) plus the named tests. Frontend: `cd web && pnpm test` / `pnpm lint`.
- Commit after every task (worktree branch `worktree-cryptic-foraging-nebula`).

---

### Task 1: Rename `avatar_url` → `avatar` end-to-end (mechanical; behavior unchanged)

The DB column, sqlc queries/generated code, and every Go identifier get the new name. Register still writes a Gravatar URL into the renamed field — behavior changes in Task 3. Existing tests must stay green and goldens must NOT change.

**Files:**
- Create: `internal/infra/storage/migrations/sqlite/20260709000000.sql`
- Create: `internal/infra/storage/migrations/pgsql/20260709000000.sql`
- Modify: `internal/infra/storage/sqlc/query/sqlite/users.sql`, `.../sqlite/user_read.sql`, `.../pgsql/users.sql`, `.../pgsql/user_read.sql`
- Regenerate: `internal/infra/storage/sqlc/gen/{sqlite,pgsql}/*` (via sqlc)
- Modify: `internal/model/user.go`, `internal/model/user_view.go`, `internal/user/repo/repo.go`, `internal/user/repo/read.go` (+ `sqlite.go`/`pgsql.go` if they reference the field), `internal/user/read.go`, `internal/user/usecase.go`, `internal/user/register.go`, `internal/user/admin.go`, `internal/user/migrate.go`, `internal/server/glue_budget.go`, `internal/server/glue_userlookup.go`, `internal/test/fixture/entities.go`, any `*_test.go` the compiler flags

**Interfaces:**
- Produces: `model.User.Avatar`, `model.Header.Avatar`, `model.UserViewRow.Avatar` (renamed from `AvatarURL`); generated sqlc field `Avatar` (renamed from `AvatarUrl`); DB column `users.avatar`.

- [ ] **Step 1: Write the migration (both engines, identical content)**

`internal/infra/storage/migrations/sqlite/20260709000000.sql` AND `internal/infra/storage/migrations/pgsql/20260709000000.sql`:

```sql
-- Avatars become selectable "<icon>:<color>" values (Material icon name +
-- color slug); the Gravatar URL era ends. Backfill every existing user to the
-- standard brand default.
ALTER TABLE users RENAME COLUMN avatar_url TO avatar;
UPDATE users SET avatar = 'face:fuchsia';
```

- [ ] **Step 2: Rename the column in the four query files**

In `internal/infra/storage/sqlc/query/{sqlite,pgsql}/users.sql` and `internal/infra/storage/sqlc/query/{sqlite,pgsql}/user_read.sql`, replace every `avatar_url` with `avatar` (SELECT lists, INSERT column lists, and the upsert's `avatar_url = excluded.avatar_url` → `avatar = excluded.avatar`).

- [ ] **Step 3: Regenerate sqlc code**

Run: `go generate ./internal/infra/storage/sqlc` (sqlc v1.30.0 is at `~/go/bin/sqlc`)
Expected: `gen/{sqlite,pgsql}/models.go` and `users.sql.go`/`user_read.sql.go` now use field `Avatar`; `git diff --stat internal/infra/storage/sqlc/gen` shows only user-related files.

- [ ] **Step 4: Rename the Go identifiers (compiler-driven)**

Run `go build ./...` and fix every error. The full rename list:

- `internal/model/user.go`: `Header.AvatarURL` → `Header.Avatar`; `User.AvatarURL` → `User.Avatar`; `NewUser(..., avatarURL, ...)` param → `avatar` (godoc: "avatar value" not "avatar URL"); `UpdateEmail(identifier, encryptedEmail, avatarURL string, ...)` → param `avatar` (signature arity unchanged in this task).
- `internal/model/user_view.go`: `UserViewRow.AvatarURL` → `Avatar`.
- `internal/user/repo/repo.go`: `AvatarUrl: u.AvatarURL` → `Avatar: u.Avatar`; `Header{..., AvatarURL: row.AvatarUrl}` → `Avatar: row.Avatar`; `AvatarURL: row.AvatarUrl` → `Avatar: row.Avatar` (reconstitute).
- `internal/user/repo/read.go`: `AvatarURL: row.AvatarUrl` → `Avatar: row.Avatar`.
- `internal/user/read.go:122` and `internal/user/usecase.go:137`: `u.AvatarURL` → `u.Avatar`.
- `internal/user/migrate.go:70`: `u.AvatarURL` → `u.Avatar`.
- `internal/server/glue_budget.go:222`, `internal/server/glue_userlookup.go:50`: `h.AvatarURL` → `h.Avatar`.
- `internal/test/fixture/entities.go:69`: INSERT column list `avatar_url` → `avatar` (keep passing `u.Avatar` unchanged).
- Any `_test.go` files the compiler flags (e.g. `internal/user/repo/repo_integration_test.go`).

Run: `grep -rn "avatar_url\|AvatarUrl\|AvatarURL" internal/ cmd/ --include="*.go" | grep -v "/gen/"`
Expected: no output.

- [ ] **Step 5: Run the full Go suite**

Run: `go test ./...`
Expected: PASS everywhere; `git status` shows NO changes under `internal/test/apiparity/testdata/` (goldens untouched — fixture avatars are still `""` and register still writes the Gravatar URL).

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "refactor: rename users.avatar_url to avatar (column, queries, model)"
```

---

### Task 2: Avatar helpers + random picker (`internal/user/avatar.go`) — TDD

**Files:**
- Create: `internal/user/avatar.go`
- Create: `internal/user/avatar_test.go`

**Interfaces:**
- Produces: `user.DefaultAvatar` const (`"face:fuchsia"`); `user.AvatarColors []string`; `user.RandomAvatarIcons []string`; `user.IsValidAvatarIcon(string) bool`; `user.IsValidAvatarColor(string) bool`; `user.JoinAvatar(icon, color string) string`; `user.RandomAvatarPicker` + `user.NewRandomAvatarPicker()`; `user.FixedAvatarPicker` (string type with `Pick()`). Tasks 3–5 and the sync test (Task 10) consume these.

- [ ] **Step 1: Write the failing tests**

`internal/user/avatar_test.go`:

```go
package user_test

import (
	"strings"
	"testing"

	appuser "github.com/econumo/econumo/internal/user"
)

func TestJoinAvatar(t *testing.T) {
	if got := appuser.JoinAvatar("face", "fuchsia"); got != "face:fuchsia" {
		t.Fatalf("JoinAvatar = %q, want face:fuchsia", got)
	}
}

func TestDefaultAvatarIsValid(t *testing.T) {
	icon, color, ok := strings.Cut(appuser.DefaultAvatar, ":")
	if !ok || !appuser.IsValidAvatarIcon(icon) || !appuser.IsValidAvatarColor(color) {
		t.Fatalf("DefaultAvatar %q is not a valid icon:color", appuser.DefaultAvatar)
	}
}

func TestIsValidAvatarIcon(t *testing.T) {
	valid := []string{"face", "account_circle", "gamepad_2", "a"}
	for _, v := range valid {
		if !appuser.IsValidAvatarIcon(v) {
			t.Errorf("IsValidAvatarIcon(%q) = false, want true", v)
		}
	}
	invalid := []string{"", "Face", "with space", "semi:colon", "dash-name", strings.Repeat("a", 65)}
	for _, v := range invalid {
		if appuser.IsValidAvatarIcon(v) {
			t.Errorf("IsValidAvatarIcon(%q) = true, want false", v)
		}
	}
}

func TestIsValidAvatarColor(t *testing.T) {
	if len(appuser.AvatarColors) != 16 {
		t.Fatalf("AvatarColors len = %d, want 16", len(appuser.AvatarColors))
	}
	for _, c := range appuser.AvatarColors {
		if !appuser.IsValidAvatarColor(c) {
			t.Errorf("IsValidAvatarColor(%q) = false, want true", c)
		}
	}
	for _, c := range []string{"", "magenta", "FUCHSIA", "neon"} {
		if appuser.IsValidAvatarColor(c) {
			t.Errorf("IsValidAvatarColor(%q) = true, want false", c)
		}
	}
}

func TestRandomAvatarPickerAlwaysValid(t *testing.T) {
	p := appuser.NewRandomAvatarPicker()
	seen := map[string]bool{}
	for range 500 {
		v := p.Pick()
		seen[v] = true
		icon, color, ok := strings.Cut(v, ":")
		if !ok || !appuser.IsValidAvatarIcon(icon) || !appuser.IsValidAvatarColor(color) {
			t.Fatalf("Pick() = %q, not a valid icon:color", v)
		}
	}
	if len(seen) < 10 {
		t.Errorf("Pick() produced only %d distinct values in 500 draws — not random", len(seen))
	}
}

func TestFixedAvatarPicker(t *testing.T) {
	if got := appuser.FixedAvatarPicker("pets:teal").Pick(); got != "pets:teal" {
		t.Fatalf("FixedAvatarPicker.Pick() = %q, want pets:teal", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/user/ -run 'Avatar' -v`
Expected: FAIL — `undefined: appuser.JoinAvatar` etc.

- [ ] **Step 3: Implement `internal/user/avatar.go`**

```go
// Avatar helpers: the "<icon>:<color>" avatar value format, the color
// allowlist, and the random default picker used at registration.
package user

import (
	"math/rand/v2"
	"regexp"
)

// DefaultAvatar is the standard value existing rows were backfilled to by the
// 20260709000000 migration; test harnesses also pin it as the deterministic
// registration default.
const DefaultAvatar = "face:fuchsia"

// AvatarColors is the canonical background-color allowlist. The frontend
// mirrors it (web/src/lib/avatars.ts) and a sync test asserts exact equality,
// so order and spelling are contract.
var AvatarColors = []string{
	"red", "orange", "amber", "yellow", "lime", "green", "emerald", "teal",
	"cyan", "sky", "blue", "indigo", "violet", "purple", "fuchsia", "pink",
}

// RandomAvatarIcons is the curated subset random registration defaults draw
// from. Every name must exist in the frontend's availableIcons (asserted by a
// frontend sync test); update-avatar itself accepts any well-formed name.
var RandomAvatarIcons = []string{
	"face", "account_circle", "pets", "stars", "celebration", "lightbulb",
	"extension", "support", "savings", "wallet", "home", "alarm",
	"fingerprint", "shopping_basket", "credit_card", "store",
}

var avatarIconRe = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)

// IsValidAvatarIcon checks the Material-ligature name format only; the icon
// universe is owned by the frontend (same precedent as category icons).
func IsValidAvatarIcon(icon string) bool { return avatarIconRe.MatchString(icon) }

func IsValidAvatarColor(color string) bool {
	for _, c := range AvatarColors {
		if c == color {
			return true
		}
	}
	return false
}

func JoinAvatar(icon, color string) string { return icon + ":" + color }

// RandomAvatarPicker is the production AvatarPicker: a uniform random
// icon+color for each new user.
type RandomAvatarPicker struct{}

func NewRandomAvatarPicker() RandomAvatarPicker { return RandomAvatarPicker{} }

func (RandomAvatarPicker) Pick() string {
	return JoinAvatar(
		RandomAvatarIcons[rand.IntN(len(RandomAvatarIcons))],
		AvatarColors[rand.IntN(len(AvatarColors))],
	)
}

// FixedAvatarPicker always returns its literal value; test harnesses use it so
// golden responses stay deterministic.
type FixedAvatarPicker string

func (p FixedAvatarPicker) Pick() string { return string(p) }
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/user/ -run 'Avatar' -v`
Expected: PASS (all six tests).

- [ ] **Step 5: Commit**

```bash
git add internal/user/avatar.go internal/user/avatar_test.go
git commit -m "feat: avatar value helpers, color allowlist, random picker"
```

---

### Task 3: Registration default via the AvatarPicker seam; drop Gravatar

**Files:**
- Modify: `internal/user/ports.go`, `internal/user/usecase.go`, `internal/user/register.go`, `internal/user/admin.go`, `internal/user/migrate.go`, `internal/model/user.go`
- Modify: `internal/server/server.go` (BuildAPI signature + wiring), `cmd/econumo/main.go`, `internal/cli/container.go`, `internal/test/apiparity/harness.go`
- Modify: `internal/test/fixture/entities.go` (default avatar for seeded users)
- Modify: `internal/user/admin_integration_test.go` (newUserSvc + new assertions), any other `NewService` call sites the compiler flags
- Regenerate: apiparity goldens

**Interfaces:**
- Consumes: `user.FixedAvatarPicker`, `user.NewRandomAvatarPicker`, `user.DefaultAvatar` (Task 2).
- Produces: `user.AvatarPicker` interface; `user.NewService(repo, tx, encode, hasher, jwtSvc, currency, budgets, passwordRequests, mailer, avatars, clk, allowRegistration)` (new `avatars AvatarPicker` param before `clk`); `server.BuildAPI(cfg, db, jwtSvc, clk, avatars)` (new 5th param); `model.User.UpdateEmail(identifier, encryptedEmail string, now time.Time)` (avatar param REMOVED).

- [ ] **Step 1: Write the failing integration test**

Append to `internal/user/admin_integration_test.go`:

```go
func TestAdminCreateUserAssignsPickedAvatar(t *testing.T) {
	db := dbtest.New(t)
	svc, enc, _ := newUserSvc(t, db)
	repo := userrepo.NewRepo(db.Engine, db.TX)
	ctx := context.Background()

	if _, err := svc.AdminCreateUser(ctx, "Avatar Tester", "avatar@econumo.test", "secretpass"); err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}
	u, err := repo.GetByIdentifier(ctx, enc.Hash("avatar@econumo.test"))
	if err != nil {
		t.Fatalf("GetByIdentifier: %v", err)
	}
	if u.Avatar != appuser.DefaultAvatar {
		t.Fatalf("Avatar = %q, want the stub picker value %q", u.Avatar, appuser.DefaultAvatar)
	}
}

func TestAdminChangeEmailKeepsAvatar(t *testing.T) {
	db := dbtest.New(t)
	svc, enc, _ := newUserSvc(t, db)
	repo := userrepo.NewRepo(db.Engine, db.TX)
	ctx := context.Background()

	if _, err := svc.AdminCreateUser(ctx, "Keep Avatar", "keep@econumo.test", "secretpass"); err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}
	if err := svc.AdminChangeEmail(ctx, "keep@econumo.test", "kept@econumo.test"); err != nil {
		t.Fatalf("AdminChangeEmail: %v", err)
	}
	u, err := repo.GetByIdentifier(ctx, enc.Hash("kept@econumo.test"))
	if err != nil {
		t.Fatalf("GetByIdentifier: %v", err)
	}
	if u.Avatar != appuser.DefaultAvatar {
		t.Fatalf("Avatar = %q after email change, want unchanged %q", u.Avatar, appuser.DefaultAvatar)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/user/ -run 'TestAdminCreateUserAssignsPickedAvatar|TestAdminChangeEmailKeepsAvatar' -v`
Expected: FAIL — first compile (newUserSvc lacks the picker) after Step 3's seam lands it fails on the gravatar-URL assertion until Step 4 completes. (It's fine that failure mode shifts as you work through the steps.)

- [ ] **Step 3: Add the seam**

Append to `internal/user/ports.go`:

```go
// AvatarPicker supplies the avatar value for newly created users. Production
// wiring picks randomly (RandomAvatarPicker); test harnesses pin a fixed value
// (FixedAvatarPicker) so golden responses stay deterministic.
type AvatarPicker interface {
	Pick() string
}
```

In `internal/user/usecase.go`: add field `avatars AvatarPicker` to `Service`, add param `avatars AvatarPicker` to `NewService` between `mailer` and `clock`, assign it. Delete `md5Hex` and the now-unused `crypto/md5` import.

- [ ] **Step 4: Use it and drop Gravatar**

- `internal/user/register.go` `createUser`: replace `avatarURL := fmt.Sprintf("https://www.gravatar.com/avatar/%s", md5Hex(loweredEmail))` with `avatar := s.avatars.Pick()`; pass `avatar` to `model.NewUser`; drop the unused `fmt` import.
- `internal/model/user.go`: `UpdateEmail(identifier, encryptedEmail string, now time.Time)` — remove the avatar param and the `u.Avatar =` line; update its godoc ("replaces the encrypted email and identifier together").
- `internal/user/admin.go` `AdminChangeEmail`: delete the gravatar line; call `u.UpdateEmail(newIdentifier, encryptedEmail, s.clock.Now())`; drop unused `fmt` import if now unused.
- `internal/user/migrate.go`: `u.UpdateEmail(newIdent, plain, s.clock.Now())`; delete the "Avatar is the gravatar…" comment.

- [ ] **Step 5: Rewire every constructor call site**

- `internal/server/server.go`: `func BuildAPI(cfg config.Config, db *sql.DB, jwtSvc *jwt.JWT, clk port.Clock, avatars appuser.AvatarPicker) http.Handler`; pass `avatars` into `appuser.NewService(...)` between `resetMailer` and `clk`.
- `cmd/econumo/main.go:222`: `server.BuildAPI(cfg, db, jwtSvc, clock.New(), appuser.NewRandomAvatarPicker())` (import `appuser "github.com/econumo/econumo/internal/user"`).
- `internal/cli/container.go`: pass `appuser.NewRandomAvatarPicker()` in its `appuser.NewService(...)` call (same position).
- `internal/test/apiparity/harness.go:82`: `server.BuildAPI(cfg, db.Raw, jwtSvc, clk, appuser.FixedAvatarPicker(appuser.DefaultAvatar))` (add the import).
- `internal/user/admin_integration_test.go` `newUserSvc`: pass `appuser.FixedAvatarPicker(appuser.DefaultAvatar)` in the same position.
- Fix any remaining `NewService`/`BuildAPI` call sites `go build ./...` flags (e.g. `internal/user/migrate_test.go`, api harness tests) the same way: `FixedAvatarPicker(DefaultAvatar)` in tests.

- [ ] **Step 6: Seeded fixture users get the standard value**

`internal/test/fixture/entities.go`, in `func (b *Builder) User(...)` with the other zero-field defaults:

```go
if u.Avatar == "" {
	u.Avatar = "face:fuchsia"
}
```

And extend the `User` struct's `Avatar` field comment: `// default "face:fuchsia"`.

- [ ] **Step 7: Run tests, regenerate goldens, inspect**

Run: `go test ./internal/user/... ./internal/server/... ./internal/cli/...`
Expected: PASS (including the two new tests).

Run: `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/`
Run: `git diff --stat internal/test/apiparity/testdata/ && git diff internal/test/apiparity/testdata/ | grep '^[+-]' | grep -v '^[+-][+-]' | grep -iv avatar | head`
Expected: many goldens change; every changed line is an `"avatar"` value (`""` or a gravatar URL → `"face:fuchsia"`). The second command must print NOTHING (no non-avatar changes). If it prints anything, STOP and investigate.

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "feat: random default avatar at registration via AvatarPicker seam; drop Gravatar"
```

---

### Task 4: `update-avatar` DTO + model mutator + service use case — TDD

**Files:**
- Modify: `internal/model/user.go` (mutator), `internal/model/user_dto.go` (DTO)
- Create: `internal/model/user_dto_avatar_test.go`
- Modify: `internal/user/profile.go` (use case)
- Create: `internal/user/avatar_update_integration_test.go`

**Interfaces:**
- Consumes: `user.IsValidAvatarIcon`, `user.IsValidAvatarColor`, `user.JoinAvatar` (Task 2); `Service.mutate`, `Service.toCurrentUser` (existing).
- Produces: `model.UpdateAvatarRequest{Icon, Color string}` (JSON `icon`, `color`) with `Validate()`; `model.UpdateAvatarResult{User CurrentUserResult}` (JSON `user`); `(*model.User).UpdateAvatar(avatar string, now time.Time)`; `(*user.Service).UpdateAvatar(ctx, userID vo.Id, req model.UpdateAvatarRequest) (*model.UpdateAvatarResult, error)`. Task 5's handler consumes the service method.

- [ ] **Step 1: Write the failing DTO validation test**

`internal/model/user_dto_avatar_test.go`:

```go
package model_test

import (
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
)

func TestUpdateAvatarRequestValidate(t *testing.T) {
	cases := []struct {
		name       string
		req        model.UpdateAvatarRequest
		wantFields map[string]string // key -> code
	}{
		{"valid", model.UpdateAvatarRequest{Icon: "face", Color: "fuchsia"}, nil},
		{"both blank", model.UpdateAvatarRequest{}, map[string]string{"icon": "IS_BLANK_ERROR", "color": "IS_BLANK_ERROR"}},
		{"blank icon", model.UpdateAvatarRequest{Color: "red"}, map[string]string{"icon": "IS_BLANK_ERROR"}},
		{"blank color", model.UpdateAvatarRequest{Icon: "face"}, map[string]string{"color": "IS_BLANK_ERROR"}},
		{"whitespace icon", model.UpdateAvatarRequest{Icon: "   ", Color: "red"}, map[string]string{"icon": "IS_BLANK_ERROR"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			if tc.wantFields == nil {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			verr, ok := errs.AsValidation(err)
			if !ok {
				t.Fatalf("Validate() = %v, want *ValidationError", err)
			}
			if len(verr.Fields) != len(tc.wantFields) {
				t.Fatalf("got %d field errors %v, want %d", len(verr.Fields), verr.Fields, len(tc.wantFields))
			}
			for _, f := range verr.Fields {
				if code, ok := tc.wantFields[f.Key]; !ok || f.Code != code {
					t.Errorf("unexpected field error %+v", f)
				}
			}
		})
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/model/ -run TestUpdateAvatarRequestValidate -v`
Expected: FAIL — `undefined: model.UpdateAvatarRequest`.

- [ ] **Step 3: Implement DTO + mutator**

Append to `internal/model/user_dto.go` (new section between `update-budget` and `remind-password`, matching the file's section-comment style):

```go
// ---------------------------------------------------------------------------
// update-avatar
// ---------------------------------------------------------------------------

// UpdateAvatarRequest is the update-avatar request body. Icon is a Material
// ligature name; color must be one of the avatar color slugs (tier-2, in the
// service).
type UpdateAvatarRequest struct {
	Icon  string `json:"icon"`
	Color string `json:"color"`
}

// Validate enforces NotBlank on both fields; format/choice checks are tier 2.
func (r UpdateAvatarRequest) Validate() error {
	var fields []errs.FieldError
	if strings.TrimSpace(r.Icon) == "" {
		fields = append(fields, errs.FieldError{Key: "icon", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	if strings.TrimSpace(r.Color) == "" {
		fields = append(fields, errs.FieldError{Key: "color", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

// UpdateAvatarResult is the update-avatar response.
type UpdateAvatarResult struct {
	User CurrentUserResult `json:"user"`
}
```

Append to `internal/model/user.go` next to `UpdateName`:

```go
func (u *User) UpdateAvatar(avatar string, now time.Time) {
	u.Avatar = avatar
	u.UpdatedAt = now
}
```

- [ ] **Step 4: Run DTO tests**

Run: `go test ./internal/model/ -run TestUpdateAvatarRequestValidate -v`
Expected: PASS.

- [ ] **Step 5: Write the failing service test**

`internal/user/avatar_update_integration_test.go`:

```go
package user_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/test/dbtest"
	appuser "github.com/econumo/econumo/internal/user"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

func TestUpdateAvatar(t *testing.T) {
	db := dbtest.New(t)
	svc, enc, _ := newUserSvc(t, db)
	repo := userrepo.NewRepo(db.Engine, db.TX)
	ctx := context.Background()

	id, err := svc.AdminCreateUser(ctx, "Avatar Updater", "upd@econumo.test", "secretpass")
	if err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}

	res, err := svc.UpdateAvatar(ctx, id, model.UpdateAvatarRequest{Icon: "pets", Color: "teal"})
	if err != nil {
		t.Fatalf("UpdateAvatar: %v", err)
	}
	if res.User.Avatar != "pets:teal" {
		t.Fatalf("result avatar = %q, want pets:teal", res.User.Avatar)
	}
	u, err := repo.GetByIdentifier(ctx, enc.Hash("upd@econumo.test"))
	if err != nil {
		t.Fatalf("GetByIdentifier: %v", err)
	}
	if u.Avatar != "pets:teal" {
		t.Fatalf("persisted avatar = %q, want pets:teal", u.Avatar)
	}
}

func TestUpdateAvatarRejectsBadValues(t *testing.T) {
	db := dbtest.New(t)
	svc, _, _ := newUserSvc(t, db)
	ctx := context.Background()

	id, err := svc.AdminCreateUser(ctx, "Avatar Rejecter", "rej@econumo.test", "secretpass")
	if err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}

	cases := []struct {
		name string
		req  model.UpdateAvatarRequest
	}{
		{"bad icon format", model.UpdateAvatarRequest{Icon: "Not-Valid", Color: "teal"}},
		{"icon with colon", model.UpdateAvatarRequest{Icon: "face:extra", Color: "teal"}},
		{"unknown color", model.UpdateAvatarRequest{Icon: "face", Color: "neon"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.UpdateAvatar(ctx, id, tc.req); err == nil {
				t.Fatal("UpdateAvatar succeeded, want validation error")
			}
		})
	}
}
```

- [ ] **Step 6: Run to verify failure**

Run: `go test ./internal/user/ -run 'TestUpdateAvatar' -v`
Expected: FAIL — `svc.UpdateAvatar undefined`.

- [ ] **Step 7: Implement the use case**

Append to `internal/user/profile.go`:

```go
// UpdateAvatar validates the icon format and color choice (tier-2), stores the
// joined "<icon>:<color>" value, and returns the refreshed current user.
func (s *Service) UpdateAvatar(ctx context.Context, userID vo.Id, req model.UpdateAvatarRequest) (*model.UpdateAvatarResult, error) {
	icon := strings.TrimSpace(req.Icon)
	color := strings.TrimSpace(req.Color)
	var fields []errs.FieldError
	if !IsValidAvatarIcon(icon) {
		fields = append(fields, errs.FieldError{Key: "icon", Message: "This value is not valid.", Code: "INVALID_FORMAT_ERROR"})
	}
	if !IsValidAvatarColor(color) {
		fields = append(fields, errs.FieldError{Key: "color", Message: "The value you selected is not a valid choice.", Code: "NO_SUCH_CHOICE_ERROR"})
	}
	if len(fields) > 0 {
		return nil, errs.NewValidation("Validation failed", fields...)
	}
	u, err := s.mutate(ctx, userID, func(u *model.User, now time.Time) error {
		u.UpdateAvatar(JoinAvatar(icon, color), now)
		return nil
	})
	if err != nil {
		return nil, err
	}
	cur, err := s.toCurrentUser(ctx, u)
	if err != nil {
		return nil, err
	}
	return &model.UpdateAvatarResult{User: cur}, nil
}
```

Add `"strings"` to `internal/user/profile.go` imports if absent.

- [ ] **Step 8: Run tests**

Run: `go test ./internal/user/ ./internal/model/`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/model/user.go internal/model/user_dto.go internal/model/user_dto_avatar_test.go internal/user/profile.go internal/user/avatar_update_integration_test.go
git commit -m "feat: update-avatar use case with icon-format and color-choice validation"
```

---

### Task 5: HTTP endpoint, OpenAPI docs, apiparity scenario, goldens, CLAUDE.md

**Files:**
- Create: `internal/user/api/avatar.go`
- Modify: `internal/user/api/routes.go`
- Regenerate: committed OpenAPI docs (`make swagger`)
- Modify: `internal/test/apiparity/catalogue_user.go`, `internal/test/apiparity/guard_test.go` (`minRoutes`)
- Regenerate: goldens
- Modify: `CLAUDE.md`

**Interfaces:**
- Consumes: `(*user.Service).UpdateAvatar` (Task 4), `endpoint.Handle` (existing).
- Produces: `POST /api/v1/user/update-avatar` (auth), response envelope `{"success":true,"message":"","data":{"user":{...}}}`.

- [ ] **Step 1: Write the handler**

`internal/user/api/avatar.go` (modeled exactly on `name.go`):

```go
package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
)

var _ = apidoc.JsonResponseError{}
var _ = model.UpdateAvatarResult{}

// UpdateAvatar handles POST /api/v1/user/update-avatar (auth). Validates the
// icon name format and color choice, then stores "<icon>:<color>" and returns
// the refreshed user.
//
// @Summary     Update avatar
// @Description Updates the authenticated user's avatar (Material icon name + color slug) and returns the refreshed user.
// @Tags        User
// @Accept      json
// @Produce     json
// @Param       request body     model.UpdateAvatarRequest true "Update avatar request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.UpdateAvatarResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/update-avatar [post]
func (h *Handlers) UpdateAvatar(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.UpdateAvatar)
}
```

- [ ] **Step 2: Register the route**

`internal/user/api/routes.go`: change the doc comment `mounts the 13 user endpoints` → `mounts the 14 user endpoints`, and add after the `update-name` line:

```go
mux.Handle("POST /api/v1/user/update-avatar", auth(h.UpdateAvatar))
```

- [ ] **Step 3: Regenerate OpenAPI docs and build**

Run: `make swagger && go build ./...`
Expected: docs regenerate with the new path; build passes.

- [ ] **Step 4: Add apiparity coverage (route guard forces this)**

In `internal/test/apiparity/catalogue_user.go`, inside the `user_writes` scenario after the `update-budget` call:

```go
{Label: "update-avatar", Method: "POST", Path: "/api/v1/user/update-avatar", Auth: "owner",
	Body: map[string]any{"icon": "pets", "color": "teal"}},
// Pins the tier-1 blank envelope and the tier-2 format/choice envelope.
{Label: "err:update-avatar-blank", Method: "POST", Path: "/api/v1/user/update-avatar", Auth: "owner",
	Body: map[string]any{"icon": "", "color": ""}},
{Label: "err:update-avatar-bad-values", Method: "POST", Path: "/api/v1/user/update-avatar", Auth: "owner",
	Body: map[string]any{"icon": "Not-Valid", "color": "neon"}},
```

In `internal/test/apiparity/guard_test.go`: `const minRoutes = 84` → `85`.

- [ ] **Step 5: Regenerate goldens, inspect, run the suite**

Run: `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/`
Run: `git diff internal/test/apiparity/testdata/ | head -80` and `git status --short internal/test/apiparity/testdata/ | grep '^??'`
Expected: three NEW golden files for the update-avatar labels; the `user_writes` scenario's later calls (`update-password`, `get-user-data-after`) now show `"avatar": "pets:teal"`. Verify the error goldens pin the exact strings: blank → both fields `IS_BLANK_ERROR`; bad values → `INVALID_FORMAT_ERROR` (icon) + `NO_SUCH_CHOICE_ERROR` (color).

Run: `go test ./...`
Expected: PASS (scenario/route guards satisfied).

- [ ] **Step 6: Update CLAUDE.md**

In the `### Encodings, messages, routes` section, add a bullet:

```markdown
- `avatar` (user embeds) → `"<icon>:<color>"`, e.g. `"face:fuchsia"` — a Material
  icon ligature name plus a color slug from the 16-slug allowlist in
  `internal/user/avatar.go` (mirrored by `web/src/lib/avatars.ts`). Set via
  `POST /api/v1/user/update-avatar`; new users get a random default; Gravatar
  is gone.
```

Run: `grep -rin gravatar CLAUDE.md` — update or remove any remaining mention.

- [ ] **Step 7: Full smoke tier + commit**

Run: `make go-test`
Expected: PASS (build, vet, gofmt, docs-fresh, tests, coverage gate).

```bash
git add -A
git commit -m "feat: POST /api/v1/user/update-avatar endpoint with parity coverage"
```

---

### Task 6: Frontend `lib/avatars.ts` + `UserAvatar` component — TDD

**Files:**
- Create: `web/src/lib/avatars.ts`, `web/src/lib/avatars.test.ts`
- Create: `web/src/components/UserAvatar.tsx`, `web/src/components/UserAvatar.test.tsx`

**Interfaces:**
- Produces: `avatarColors: string[]` (16, backend order); `avatarColorClasses: Record<string, string>`; `defaultAvatar = 'face:fuchsia'`; `splitAvatar(avatar: string): { icon: string; color: string }` (splits on LAST colon; unknown/missing color → `'fuchsia'`); `joinAvatar(icon: string, color: string): string`; `<UserAvatar avatar={string} size?: 'xs'|'sm'|'md'|'card'|'xl'` (default `'md'`) `className?: string />` rendering `data-testid="user-avatar"` with `data-avatar` attr. Tasks 7–9 consume these.

- [ ] **Step 1: Write the failing lib tests**

`web/src/lib/avatars.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { avatarColors, avatarColorClasses, defaultAvatar, joinAvatar, splitAvatar } from './avatars'

describe('avatars', () => {
  it('has 16 colors each with a background class', () => {
    expect(avatarColors).toHaveLength(16)
    for (const color of avatarColors) {
      expect(avatarColorClasses[color], color).toMatch(/^bg-/)
    }
  })

  it('fuchsia renders the brand magenta', () => {
    expect(avatarColorClasses.fuchsia).toBe('bg-econumo-magenta')
  })

  it('joins and splits round-trip', () => {
    expect(joinAvatar('face', 'teal')).toBe('face:teal')
    expect(splitAvatar('face:teal')).toEqual({ icon: 'face', color: 'teal' })
  })

  it('splits on the last colon', () => {
    expect(splitAvatar('weird:name:teal')).toEqual({ icon: 'weird:name', color: 'teal' })
  })

  it('falls back to fuchsia for unknown or missing color', () => {
    expect(splitAvatar('face:neon').color).toBe('fuchsia')
    expect(splitAvatar('just_an_icon').color).toBe('fuchsia')
    expect(splitAvatar('just_an_icon').icon).toBe('just_an_icon')
  })

  it('default avatar is face on fuchsia', () => {
    expect(defaultAvatar).toBe('face:fuchsia')
    expect(splitAvatar(defaultAvatar)).toEqual({ icon: 'face', color: 'fuchsia' })
  })
})
```

- [ ] **Step 2: Run to verify failure**

Run: `cd web && pnpm test -- run src/lib/avatars.test.ts` (use the project's vitest invocation; `pnpm test` runs all)
Expected: FAIL — module not found.

- [ ] **Step 3: Implement `web/src/lib/avatars.ts`**

```ts
// The avatar value format is "<icon>:<color>" — a Material ligature name plus
// a color slug. The color list mirrors the backend allowlist in
// internal/user/avatar.go (order is contract; a sync test asserts equality).
export const avatarColors = [
  'red', 'orange', 'amber', 'yellow', 'lime', 'green', 'emerald', 'teal',
  'cyan', 'sky', 'blue', 'indigo', 'violet', 'purple', 'fuchsia', 'pink',
] as const
export type AvatarColor = (typeof avatarColors)[number]

// fuchsia is the brand magenta so the migration default reads as Econumo.
export const avatarColorClasses: Record<AvatarColor, string> = {
  red: 'bg-red-500',
  orange: 'bg-orange-500',
  amber: 'bg-amber-500',
  yellow: 'bg-yellow-500',
  lime: 'bg-lime-500',
  green: 'bg-green-500',
  emerald: 'bg-emerald-500',
  teal: 'bg-teal-500',
  cyan: 'bg-cyan-500',
  sky: 'bg-sky-500',
  blue: 'bg-blue-500',
  indigo: 'bg-indigo-500',
  violet: 'bg-violet-500',
  purple: 'bg-purple-500',
  fuchsia: 'bg-econumo-magenta',
  pink: 'bg-pink-500',
}

export const defaultAvatar = 'face:fuchsia'

export function joinAvatar(icon: string, color: string): string {
  return `${icon}:${color}`
}

const isAvatarColor = (v: string): v is AvatarColor => (avatarColors as readonly string[]).includes(v)

export function splitAvatar(avatar: string): { icon: string; color: AvatarColor } {
  const at = avatar.lastIndexOf(':')
  const icon = at > 0 ? avatar.slice(0, at) : avatar
  const color = at > 0 ? avatar.slice(at + 1) : ''
  return { icon, color: isAvatarColor(color) ? color : 'fuchsia' }
}
```

- [ ] **Step 4: Run lib tests — PASS expected**

- [ ] **Step 5: Write the failing component test**

`web/src/components/UserAvatar.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { UserAvatar } from './UserAvatar'

describe('UserAvatar', () => {
  it('renders the icon glyph on the color background', () => {
    render(<UserAvatar avatar="pets:teal" />)
    const el = screen.getByTestId('user-avatar')
    expect(el).toHaveAttribute('data-avatar', 'pets:teal')
    expect(el.className).toContain('bg-teal-500')
    expect(el).toHaveTextContent('pets')
  })

  it('falls back to fuchsia for an unknown color', () => {
    render(<UserAvatar avatar="face:neon" />)
    expect(screen.getByTestId('user-avatar').className).toContain('bg-econumo-magenta')
  })

  it('is decorative (hidden from the accessibility tree)', () => {
    render(<UserAvatar avatar="face:fuchsia" />)
    expect(screen.getByTestId('user-avatar')).toHaveAttribute('aria-hidden', 'true')
  })

  it('applies size and extra classes', () => {
    render(<UserAvatar avatar="face:fuchsia" size="xl" className="rounded-none" />)
    const el = screen.getByTestId('user-avatar')
    expect(el.className).toContain('size-24')
    expect(el.className).toContain('rounded-none')
  })
})
```

- [ ] **Step 6: Run to verify failure, then implement `web/src/components/UserAvatar.tsx`**

```tsx
import { EntityIcon } from '@/components/EntityIcon'
import { avatarColorClasses, splitAvatar } from '@/lib/avatars'
import { cn } from '@/lib/utils'

// One size per current render site: xs=connection preview (20px), sm=share
// dialog + onboarding (32px), md=connection rows + sidebar rail (40px),
// card=sidebar user card (48px), xl=profile page (96px).
const sizeClasses = {
  xs: 'size-5 rounded-full text-sm',
  sm: 'size-8 rounded-full text-lg',
  md: 'size-10 rounded-full text-xl',
  card: 'size-12 rounded-xl text-2xl',
  xl: 'size-24 rounded-3xl text-5xl',
} as const

interface UserAvatarProps {
  avatar: string
  size?: keyof typeof sizeClasses
  className?: string
}

// The single avatar render path: the "<icon>:<color>" value as a Material
// glyph on a colored square. Decorative — the adjacent user name carries the
// accessible label.
export function UserAvatar({ avatar, size = 'md', className }: UserAvatarProps) {
  const { icon, color } = splitAvatar(avatar)
  return (
    <span
      aria-hidden="true"
      data-testid="user-avatar"
      data-avatar={avatar}
      className={cn(
        'flex shrink-0 select-none items-center justify-center text-white',
        sizeClasses[size],
        avatarColorClasses[color],
        className,
      )}
    >
      <EntityIcon name={icon} />
    </span>
  )
}
```

- [ ] **Step 7: Run tests + lint**

Run: `cd web && pnpm test && pnpm lint`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add web/src/lib/avatars.ts web/src/lib/avatars.test.ts web/src/components/UserAvatar.tsx web/src/components/UserAvatar.test.tsx
git commit -m "feat(web): avatar value helpers and UserAvatar component"
```

---

### Task 7: Replace every avatar `<img>` with `UserAvatar`; update fixtures

**Files:**
- Modify: `web/src/components/UserCard.tsx`, `web/src/app/layouts/ApplicationLayout.tsx`, `web/src/features/connections/ConnectionsPage.tsx`, `web/src/features/connections/ShareAccessDialog.tsx`, `web/src/features/connections/PreviewConnectionDialog.tsx`, `web/src/features/connections/DeclineAccessDialog.tsx`, `web/src/features/connections/AccessLevelDialog.tsx`, `web/src/features/onboarding/OnboardingPage.tsx` (Step icon only — copy changes in Task 9)
- Modify: `web/src/test/fixtures.ts` and any test the change breaks

**Interfaces:**
- Consumes: `UserAvatar` (Task 6).
- Produces: no `<img src={...avatar...}>` remains anywhere; fixtures carry `"<icon>:<color>"` values.

- [ ] **Step 1: Update fixtures first**

`web/src/test/fixtures.ts`: replace `'https://avatars.test/ada'` with `'face:emerald'` (both occurrences) and `'https://avatars.test/partner'` with `'pets:sky'`.

- [ ] **Step 2: Swap each render site**

Mapping (delete each `<img …avatar…>` and its `?s=N` param):

- `UserCard.tsx`: `<img src={\`${user.avatar}?s=…\`} …>` → `<UserAvatar avatar={user.avatar} size={size === 'lg' ? 'xl' : 'card'} />`; update the file's top comment (rounded-square avatar wording stays true).
- `ApplicationLayout.tsx` (rail block): → `<UserAvatar avatar={user.avatar} size="md" className="rounded-xl" />`.
- `ConnectionsPage.tsx` (size-10 rounded-full): → `<UserAvatar avatar={connection.user.avatar} size="md" />`.
- `ShareAccessDialog.tsx` (size-8): → `<UserAvatar avatar={entry.user.avatar} size="sm" />`.
- `PreviewConnectionDialog.tsx` (size-5): → `<UserAvatar avatar={item.owner.avatar} size="xs" />`.
- `DeclineAccessDialog.tsx` (size-10): → `<UserAvatar avatar={owner.avatar} size="md" />`.
- `AccessLevelDialog.tsx` (size-10): → `<UserAvatar avatar={user.avatar} size="md" />`.
- `OnboardingPage.tsx` `Step` component: the `avatar` prop keeps its `string` type but now receives the raw value; replace `<img src={avatar} …>` with `<UserAvatar avatar={avatar} size="sm" />`; at the call site pass `avatar={user?.avatar}` (drop the `?s=30` template).

- [ ] **Step 3: Verify no img/gravatar remains**

Run: `grep -rn "avatar}?s=\|gravatar" web/src --include="*.tsx" --include="*.ts" | grep -v test | grep -v "components/ui/"`
Expected: only `OnboardingPage.tsx` gravatar COPY lines remain (reworked in Task 9); no `<img>` avatar renders anywhere.

- [ ] **Step 4: Run the web suite and repair assertions**

Run: `cd web && pnpm test`
Expected: failures only in tests that queried the old `<img>` by `alt` text or URL. Fix each by querying `screen.getByTestId('user-avatar')` / `getAllByTestId` and asserting `data-avatar` where the old test asserted the URL. Do not weaken what a test proves (if it asserted WHICH user's avatar rendered, keep asserting the value).

Run: `cd web && pnpm test && pnpm lint`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat(web): render all avatars through UserAvatar"
```

---

### Task 8: API client + mutation + metric — TDD

**Files:**
- Modify: `web/src/api/user.ts`, `web/src/api/user.test.ts`, `web/src/features/user/queries.ts`, `web/src/features/user/queries.test.tsx`, `web/src/lib/metrics.ts`

**Interfaces:**
- Consumes: existing `api`/`apiUrl` client, `queryKeys.user`, `trackEvent`.
- Produces: `updateAvatar(icon: string, color: string): Promise<CurrentUserDto>`; `useUpdateAvatar()` mutation (input `{icon: string; color: string}`); `METRICS.USER_UPDATE_AVATAR = 'appUserUpdateAvatar'`. Task 9 consumes the hook.

- [ ] **Step 1: Write failing tests**

Add to `web/src/api/user.test.ts` a case cloned from the existing `updateName` test (same msw/mocking idiom the file already uses) asserting: POST to `/api/v1/user/update-avatar` with body `{"icon":"pets","color":"teal"}` returns the unwrapped `user` object.

Add to `web/src/features/user/queries.test.tsx` a `useUpdateAvatar` case cloned from the existing `useUpdateName` test: on success the user query data is replaced with the returned user.

- [ ] **Step 2: Run to verify failure**

Run: `cd web && pnpm test`
Expected: FAIL — `updateAvatar` / `useUpdateAvatar` undefined.

- [ ] **Step 3: Implement**

`web/src/api/user.ts`, after `updateName`:

```ts
export async function updateAvatar(icon: string, color: string): Promise<CurrentUserDto> {
  const response = await api.post<CurrentUserResponseDto>(apiUrl('/api/v1/user/update-avatar'), { icon, color })
  return response.data.data.user
}
```

`web/src/lib/metrics.ts`, next to `USER_UPDATE_NAME`:

```ts
USER_UPDATE_AVATAR: 'appUserUpdateAvatar',
```

`web/src/features/user/queries.ts`, after `useUpdateName`:

```ts
export function useUpdateAvatar() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ icon, color }: { icon: string; color: string }) => userApi.updateAvatar(icon, color),
    onSuccess: (user) => {
      queryClient.setQueryData(queryKeys.user, user)
      trackEvent(METRICS.USER_UPDATE_AVATAR)
    },
  })
}
```

- [ ] **Step 4: Run tests — PASS expected; commit**

```bash
git add web/src/api/user.ts web/src/api/user.test.ts web/src/features/user/queries.ts web/src/features/user/queries.test.tsx web/src/lib/metrics.ts
git commit -m "feat(web): update-avatar API client, mutation, metric"
```

---

### Task 9: AvatarPickerDialog + ProfilePage trigger + onboarding rework + locales — TDD

**Files:**
- Create: `web/src/components/AvatarPickerDialog.tsx`, `web/src/components/AvatarPickerDialog.test.tsx`
- Modify: `web/src/components/UserCard.tsx` (clickable avatar), `web/src/features/settings/ProfilePage.tsx`, `web/src/features/settings/ProfilePage.test.tsx`, `web/src/features/onboarding/OnboardingPage.tsx`, `web/src/locales/en-US.ts`

**Interfaces:**
- Consumes: `IconPicker` (existing, props `value/onChange/aria-label`), `ResponsiveDialog` (props `open/onOpenChange/title/children/footer`), `useUpdateAvatar` (Task 8), `useUserData`, `UserAvatar`, `avatarColors`/`avatarColorClasses`/`splitAvatar`/`joinAvatar` (Task 6).
- Produces: `<AvatarPickerDialog open onClose />`; `UserCard` gains optional `onAvatarClick?: () => void` + `avatarLabel?: string`.

- [ ] **Step 1: Add locale keys**

`web/src/locales/en-US.ts`, inside the top-level `'modals'` object:

```ts
'avatar_picker': {
  'title': 'Choose your avatar',
  'icons': 'Avatar icons',
  'change': 'Change avatar',
},
```

(Existing keys reused: `elements.button.save.label`, `elements.button.cancel.label`.)

- [ ] **Step 2: Write the failing dialog test**

`web/src/components/AvatarPickerDialog.test.tsx` (follow the file-local render/providers idiom of `ProfilePage.test.tsx` — QueryClient + i18n wrappers; mock `@/api/user`):

```tsx
// Behavior to pin (write with the project's existing test scaffolding):
// 1. Given user data with avatar "face:fuchsia", opening the dialog shows the
//    preview via getByTestId('user-avatar') with data-avatar "face:fuchsia".
// 2. Clicking a color swatch (getByRole('radio', { name: 'teal' })) and an icon
//    (getByRole('option', { name: 'pets' })) updates the preview's data-avatar
//    to "pets:teal".
// 3. Clicking Save (getByRole('button', { name: 'Save' })) calls the mocked
//    updateAvatar with ('pets', 'teal') and onClose fires.
// 4. Cancel calls onClose without calling updateAvatar.
```

Write all four as real test cases, then:

Run: `cd web && pnpm test` — Expected: FAIL (component missing).

- [ ] **Step 3: Implement `web/src/components/AvatarPickerDialog.tsx`**

```tsx
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { IconPicker } from '@/components/IconPicker'
import { UserAvatar } from '@/components/UserAvatar'
import { avatarColors, avatarColorClasses, joinAvatar, splitAvatar } from '@/lib/avatars'
import { cn } from '@/lib/utils'
import { useUpdateAvatar, useUserData } from '@/features/user/queries'

interface AvatarPickerDialogProps {
  open: boolean
  onClose: () => void
}

// Icon + color picker for the user's avatar, seeded from the current value.
export function AvatarPickerDialog({ open, onClose }: AvatarPickerDialogProps) {
  const { t } = useTranslation()
  const { data: user } = useUserData()
  const updateAvatar = useUpdateAvatar()
  const current = splitAvatar(user?.avatar ?? '')
  const [icon, setIcon] = useState(current.icon)
  const [color, setColor] = useState(current.color)

  useEffect(() => {
    if (open && user) {
      const v = splitAvatar(user.avatar)
      setIcon(v.icon)
      setColor(v.color)
    }
  }, [open, user])

  const save = () => {
    updateAvatar.mutate(
      { icon, color },
      {
        onSuccess: () => onClose(),
      },
    )
  }

  return (
    <ResponsiveDialog open={open} onOpenChange={(o) => !o && onClose()} title={t('modals.avatar_picker.title')}>
      <div className="flex flex-col gap-4">
        <div className="flex justify-center">
          <UserAvatar avatar={joinAvatar(icon, color)} size="xl" />
        </div>
        <div role="radiogroup" aria-label={t('modals.avatar_picker.title')} className="flex flex-wrap justify-center gap-2">
          {avatarColors.map((c) => (
            <button
              key={c}
              type="button"
              role="radio"
              aria-checked={c === color}
              aria-label={c}
              title={c}
              className={cn('size-7 rounded-full', avatarColorClasses[c], c === color ? 'ring-2 ring-ring ring-offset-2 ring-offset-background' : '')}
              onClick={() => setColor(c)}
            />
          ))}
        </div>
        <IconPicker value={icon} onChange={setIcon} aria-label={t('modals.avatar_picker.icons')} />
        <div className="flex justify-end gap-2">
          <Button variant="outline" onClick={onClose}>
            {t('elements.button.cancel.label')}
          </Button>
          <Button onClick={save} disabled={updateAvatar.isPending}>
            {t('elements.button.save.label')}
          </Button>
        </div>
      </div>
    </ResponsiveDialog>
  )
}
```

(Adjust `Button` import/variants to the project's actual `ui/button` API if it differs — copy from a neighboring dialog.)

- [ ] **Step 4: Make the profile avatar clickable**

`web/src/components/UserCard.tsx`: add optional props `onAvatarClick?: () => void` and `avatarLabel?: string`; when `onAvatarClick` is set, wrap the `UserAvatar` in:

```tsx
<button type="button" aria-label={avatarLabel} onClick={onAvatarClick} className="shrink-0 rounded-3xl transition-opacity hover:opacity-80 focus-visible:ring-2 focus-visible:ring-ring">
  <UserAvatar avatar={user.avatar} size={size === 'lg' ? 'xl' : 'card'} />
</button>
```

`web/src/features/settings/ProfilePage.tsx`: add `const [avatarOpen, setAvatarOpen] = useState(false)`; pass `onAvatarClick={() => setAvatarOpen(true)} avatarLabel={t('modals.avatar_picker.change')}` to the `UserCard size="lg"`; render `<AvatarPickerDialog open={avatarOpen} onClose={() => setAvatarOpen(false)} />` beside the existing dialogs.

Add to `ProfilePage.test.tsx`: clicking the `Change avatar` button opens the dialog (assert `getByRole('dialog')` contains the picker title).

- [ ] **Step 5: Rework the onboarding avatar step**

`web/src/features/onboarding/OnboardingPage.tsx`: add `const [avatarOpen, setAvatarOpen] = useState(false)` and render `<AvatarPickerDialog open={avatarOpen} onClose={() => setAvatarOpen(false)} />`; replace the step's gravatar paragraph with:

```tsx
<p>
  Pick an icon and a color for your avatar — it represents you in shared accounts.{' '}
  <button type="button" onClick={() => setAvatarOpen(true)} className="text-econumo-purple underline underline-offset-2">
    Choose your avatar
  </button>
</p>
```

Run: `grep -rn "gravatar" web/src --include="*.tsx" --include="*.ts"` — Expected: no output.

- [ ] **Step 6: Run the full web suite**

Run: `cd web && pnpm test && pnpm lint`
Expected: PASS (including the new dialog + profile tests; fix any onboarding test asserting the old copy).

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "feat(web): avatar picker dialog wired to profile and onboarding"
```

---

### Task 10: Backend↔frontend sync-guard test

**Files:**
- Create: `web/src/lib/avatarSync.test.ts`

**Interfaces:**
- Consumes: `internal/user/avatar.go` source (read via node `fs`), `avatarColors` (Task 6), `availableIcons` (existing `web/src/lib/icons.ts`).

- [ ] **Step 1: Write the test (it should pass immediately — it guards future drift)**

```ts
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import { avatarColors } from './avatars'
import { availableIcons } from './icons'

// Guards the two cross-language contracts in internal/user/avatar.go:
// the color allowlist must match exactly, and every icon the backend can
// randomly assign must exist in the frontend icon set.
function goStringSlice(src: string, varName: string): string[] {
  const m = src.match(new RegExp(`${varName} = \\[\\]string\\{([^}]*)\\}`, 's'))
  if (!m) {
    throw new Error(`${varName} slice not found in avatar.go`)
  }
  return [...m[1].matchAll(/"([a-z0-9_]+)"/g)].map((x) => x[1])
}

describe('backend avatar constants stay in sync', () => {
  // vitest runs with cwd = web/, so the Go source is one level up.
  const goSrc = readFileSync(resolve(process.cwd(), '../internal/user/avatar.go'), 'utf8')

  it('color allowlists are identical (names and order)', () => {
    expect(goStringSlice(goSrc, 'AvatarColors')).toEqual([...avatarColors])
  })

  it('every backend random icon exists in availableIcons', () => {
    const backendIcons = goStringSlice(goSrc, 'RandomAvatarIcons')
    expect(backendIcons.length).toBeGreaterThan(0)
    for (const icon of backendIcons) {
      expect(availableIcons, `backend icon "${icon}" missing from availableIcons`).toContain(icon)
    }
  })

  it('the standard default is a backend random icon on fuchsia', () => {
    expect(goSrc).toContain('DefaultAvatar = "face:fuchsia"')
    expect(availableIcons).toContain('face')
  })
})
```

- [ ] **Step 2: Run it**

Run: `cd web && pnpm test`
Expected: PASS. If the icon-membership case fails, fix `RandomAvatarIcons` in `internal/user/avatar.go` to names that exist in `availableIcons` (then rerun `go test ./internal/user/` and regenerate nothing — the subset isn't in goldens except via random production picks).

- [ ] **Step 3: Commit**

```bash
git add web/src/lib/avatarSync.test.ts
git commit -m "test(web): guard avatar color/icon sync with the Go backend"
```

---

### Task 11: Full verification sweep

**Files:** none new — verification + any fallout fixes.

- [ ] **Step 1: Whole-repo greps**

Run: `grep -rin "gravatar" --include="*.go" --include="*.ts" --include="*.tsx" --include="*.sql" . | grep -v node_modules | grep -v docs/`
Expected: no output (docs/ may keep historical references).

Run: `grep -rn "avatar_url\|AvatarUrl\|AvatarURL" internal/ cmd/ web/src --include="*.go" --include="*.ts" --include="*.tsx" | grep -v "/gen/"`
Expected: no output.

- [ ] **Step 2: Full Go tier**

Run: `make go-test`
Expected: PASS — build, vet, gofmt, OpenAPI-docs-fresh, tests, coverage gate ≥ 72.

- [ ] **Step 3: Engine comparison + pgsql repo suite (the strongest contract)**

Run: `make test`
Expected: PASS — includes the sqlite-vs-PostgreSQL enginecompare suite (byte-identical responses across engines, which exercises the new migration + renamed queries on BOTH engines) and the frontend suite. Requires Docker for the Postgres container; if unavailable, set `DATABASE_TEST_PGSQL_URL` or flag this step as blocked rather than skipping silently.

- [ ] **Step 4: Golden diff final read-through**

Run: `git log --oneline main..HEAD` and `git diff main -- internal/test/apiparity/testdata/ | grep '^[+-]' | grep -v '^[+-][+-]' | grep -iv avatar | head`
Expected: the second command prints nothing — across the whole branch, golden changes are avatar-value changes and the three new update-avatar goldens only.

- [ ] **Step 5: Commit any fixes**

```bash
git add -A
git commit -m "test: full-suite verification fixes for avatar icons" # only if fixes were needed
```
