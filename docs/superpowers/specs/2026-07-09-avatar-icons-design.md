# Avatar icons (replacing Gravatar) — design

**Date:** 2026-07-09 (revised same day: reuse the existing Material icon system;
revised 2026-07-10: 7 colors, a single curated icon page, and the outline
avatar style — colored border + colored glyph on white, not a solid fill)
**Status:** Approved design, pre-implementation

## Goal

Replace Gravatar-based user avatars with a selectable icon + background color.
The user clicks their avatar on `/settings/profile`, a popup shows the
available icons and colors, they pick, and the avatar updates everywhere.
Gravatar is removed entirely — no external requests, works offline.

## Decisions (from brainstorming)

- **Icon set:** the app's existing Material Symbols system — `EntityIcon`
  renders a ligature name from `web/src/lib/icons.ts` (`availableIcons`,
  ~236 glyphs), already used by category/account/envelope icons. The avatar
  picker reuses the existing `IconPicker` carousel component. No new icon
  infrastructure; avatars share the icon language users already know.
- **Gravatar:** dropped completely. No fallback to it anywhere. The
  onboarding page's "Update your avatar" step (which links to gravatar.com)
  is reworked to open the avatar picker instead.
- **Wire shape:** the existing `avatar` JSON field is **repurposed** — same
  name, same position in every embed, but the value changes from a Gravatar
  URL to `"<icon>:<color>"` (e.g. `"face:fuchsia"`). The bundled SPA is the
  only supported client; CLAUDE.md's frozen-contract notes are updated.
- **Color:** the user picks a background color along with the icon.
- **Existing users:** all backfilled to the single standard value
  `face:fuchsia` (the Material `face` glyph on the brand magenta — the
  `fuchsia` swatch renders `--color-econumo-magenta`, `#BD51CF`).
- **New users:** registration assigns a **random** icon and a **random**
  color (random icon from a curated backend subset; see below).
- **Frontend:** exactly one component (`UserAvatar`) renders avatars
  everywhere; a future look change touches one file.

## Value format

`<icon>:<color>` — the Material ligature name and a color slug joined by a
single colon, stored and transported as one string. Ligature names are
`[a-z0-9_]+` (e.g. `account_balance`), so the colon is never ambiguous; the
frontend still splits on the **last** colon defensively.

### Icons

The icon universe is the frontend's `availableIcons` list — the backend does
NOT allowlist icon names (matching the category/account icon precedent,
where the icon is a free NotBlank string owned by the frontend). The backend
validates **format only**: non-blank, `^[a-z0-9_]{1,64}$`.

For the random new-user default, the backend keeps a small curated subset of
16 avatar-suitable Material names (e.g. `face`, `pets`, `rocket_launch`,
`favorite`, … — final list chosen at implementation from `availableIcons`).
A frontend sync test asserts this subset ⊆ `availableIcons` so a backend
name can never render as a missing glyph.

### Color allowlist (7 slugs, backend-canonical)

`red`, `orange`, `amber`, `emerald`, `teal`, `sky`, `fuchsia`

Colors ARE allowlisted server-side (unlike icons, the slugs map to a fixed
set of rendered swatches). Rendered by the frontend as Tailwind 500-level
accents (colored border + colored glyph on a white background), except
`fuchsia` → the brand magenta `#BD51CF`
(`--color-econumo-magenta`), which makes the migration default brand-colored.
The frontend mirrors the color list; the sync test asserts exact equality
(names and order).

## Data model & migration

One new migration per engine (`internal/infra/storage/migrations/{sqlite,pgsql}`):

1. `ALTER TABLE users RENAME COLUMN avatar_url TO avatar;`
2. `UPDATE users SET avatar = 'face:fuchsia';`

Both statements are supported natively by SQLite and PostgreSQL and produce
identical results. The column stays `VARCHAR(255) NOT NULL`. After the
migration the column always holds a valid `<icon>:<color>` value, so **no
read path anywhere needs fallback or detection logic** — every feature that
embeds a user (account owners, budget participants, connections,
transactions) passes the stored value through unchanged. (sqlc derives its
schema from the migrations, so `sqlc generate` renames the generated field
automatically.)

## Backend

### Model

- `model.User.AvatarURL` → `model.User.Avatar`; `NewUser` takes the avatar
  value instead of a URL. `model.Header.AvatarURL` and
  `model.UserViewRow.AvatarURL` rename to `Avatar` likewise.
- `User.UpdateEmail` drops its `avatarURL` parameter — the avatar is no
  longer email-derived. `admin change-email` and the `data:remove-salt`
  migration stop touching it. The gravatar URL computation in
  `internal/user` is removed (the identifier derivation `md5(lower(email))`
  is unrelated and stays).
- New `User.UpdateAvatar(value string, now time.Time)` mutator.
- New `internal/user/avatar.go`: the 7-color allowlist, the curated
  16-icon random subset, `IsValidAvatarColor`, the icon format check, and
  `JoinAvatar(icon, color)`.

### Registration randomness (test-determinism seam)

Random defaults would make the golden-file and engine-comparison suites
nondeterministic (register-user returns the created user). Following the
existing `Clock` seam pattern:

- `internal/user/ports.go` gains `AvatarPicker interface { Pick() string }`
  (returns a joined `<icon>:<color>` value); `user.NewService` takes it.
- Production wiring (`server.BuildAPI` and the CLI container) supplies
  `user.NewRandomAvatarPicker()` (`math/rand/v2` over subset × colors).
  `BuildAPI` gains an `avatars user.AvatarPicker` parameter so the test
  harness can inject a stub (BuildAPI's three call sites: `cmd/econumo`,
  the CLI container path, and the apiparity harness).
- Test harnesses supply a deterministic stub returning `face:fuchsia`,
  keeping goldens and enginecompare byte-stable.

`user:create` (CLI) uses the same picker as registration (both go through
`createUser`).

### API endpoint

`POST /api/v1/user/update-avatar` (auth required), registered in
`internal/user/api/routes.go` (endpoint count comment 13 → 14), handler
`internal/user/api/avatar.go` via the standard `endpoint.Handle` combinator.

Request DTO (`internal/model/user_dto.go`):

```json
{"icon": "face", "color": "emerald"}
```

Validation (exact strings, standard 400 envelope):

- blank `icon` / blank `color` → field error
  `"This value should not be blank."`, code `IS_BLANK_ERROR`.
- `icon` not matching `^[a-z0-9_]{1,64}$` → field error
  `"This value is not valid."`, code `INVALID_FORMAT_ERROR`.
- `color` not in the 7-slug allowlist → field error
  `"The value you selected is not a valid choice."`, code
  `NO_SUCH_CHOICE_ERROR`.

Success: stores `"<icon>:<color>"` via the existing `mutate`/`Save` path and
returns `{"user": <CurrentUserResult>}` — the refreshed user, same convention
as `update-name`.

## Frontend

### `UserAvatar` component (the single render path)

`web/src/components/UserAvatar.tsx`: takes the `avatar` string + a size
variant, splits on the last `:`, renders `EntityIcon` centered on the
rounded colored square (rounded-square look preserved from the current
`<img>` styling). Color slug → background class via a literal map
(`web/src/lib/avatars.ts`); unknown color falls back to `fuchsia`, missing
glyph name falls back to EntityIcon's own `question_mark` default.

Replaces every `<img src={avatar}>` site: `UserCard` (sidebar + profile),
`ApplicationLayout` (rail mode), `ConnectionsPage`, `ShareAccessDialog`,
`PreviewConnectionDialog`, `DeclineAccessDialog`, `AccessLevelDialog`, and
`OnboardingPage`. The `?s=100` Gravatar size params disappear with the imgs.

### Picker

`AvatarPickerDialog` (`web/src/components/AvatarPickerDialog.tsx`, follows
the `CurrencyPickerDialog` / `ResponsiveDialog` pattern): opened by clicking
the avatar on `/settings/profile` (button semantics: focusable,
`aria-label`). Contents: a live preview, the existing `IconPicker` (all
`avatarIcons`, a curated 36-name single page), a 7-swatch color row, save +
cancel. Save calls a new
`updateAvatar(icon, color)` API client fn + TanStack `useUpdateAvatar`
mutation that updates the user-data query (same pattern as `useUpdateName`)
and tracks a `USER_UPDATE_AVATAR` metric event.

The onboarding page's avatar step drops its gravatar.com links/copy and
opens the same dialog.

Frontend constants (`web/src/lib/avatars.ts`): the color slug list + class
map (mirroring the Go list), split/join helpers.

## Testing

- **Backend unit:** avatar helpers (join/validate/random-picker output
  always valid), DTO `Validate()` cases (blank / bad format / bad color /
  valid), register + CLI create use the picker seam, update-avatar use case
  happy path + persistence round-trip, admin change-email no longer touches
  the avatar.
- **apiparity:** new `update-avatar` calls in the user catalogue (happy +
  validation-error cases); regenerate goldens with `UPDATE_GOLDEN=1` —
  goldens embedding an `avatar` value change to the new format; inspect the
  diff, expect no other changes. The route-coverage guard forces the new
  route to have a scenario; raise `minRoutes` 84 → 85.
- **enginecompare / pgsql rerun:** covered automatically by the existing
  suites once queries are regenerated (`make test`).
- **Frontend vitest:** `UserAvatar` (parsing, color mapping, fallbacks),
  `AvatarPickerDialog` (select icon + color, save calls the mutation),
  ProfilePage opens the dialog, fixtures move from URL values to
  `"face:fuchsia"`-style values.
- **Sync guard:** a vitest reads `internal/user/avatar.go` from the repo
  (node `fs`) and asserts (1) the backend random-subset icons all appear in
  `availableIcons`, (2) the backend color list equals the frontend color
  list exactly (names and order).

## Documentation

- Update CLAUDE.md: the wire-contract section documents `avatar` as
  `"<icon>:<color>"`; remove Gravatar references.
- `.env.example` / README untouched (no config surface added).

## Out of scope

- Custom image upload.
- Server-side icon allowlisting (icons follow the category-icon precedent:
  frontend-owned list, format-only backend validation).
- Any deprecation window for third-party clients (the bundled SPA is the
  only supported client).
