# Avatar icons (replacing Gravatar) — design

**Date:** 2026-07-09
**Status:** Approved design, pre-implementation

## Goal

Replace Gravatar-based user avatars with a selectable icon + background color.
The user clicks their avatar on `/settings/profile`, a popup shows the
available icons and colors, they pick, and the avatar updates everywhere.
Gravatar is removed entirely — no external requests, works offline.

## Decisions (from brainstorming)

- **Icon set:** curated lucide-react glyphs rendered on a colored background
  (library already bundled; no new assets).
- **Gravatar:** dropped completely. No fallback to it anywhere.
- **Wire shape:** the existing `avatar` JSON field is **repurposed** — same
  name, same position in every embed, but the value changes from a Gravatar
  URL to `"<icon>:<color>"` (e.g. `"cat:emerald"`). The bundled SPA is the
  only supported client; CLAUDE.md's frozen-contract notes are updated.
- **Color:** the user picks a background color along with the icon.
- **Existing users:** all backfilled to the single standard value
  `user:fuchsia` (the lucide `user` glyph on the brand purple/magenta —
  brand primary is `#BD51CF`, fuchsia is the nearest palette hue).
- **New users:** registration assigns a **random** icon and a **random**
  color from the allowlists.
- **Frontend:** exactly one component (`UserAvatar`) renders avatars
  everywhere; a future look change touches one file.

## Value format

`<icon>:<color>` — two lowercase slugs joined by a single colon, stored and
transported as one string. Always valid after migration: every DB row and
every API response carries a value from the allowlists.

### Icon allowlist (32 lucide slugs)

`user`, `cat`, `dog`, `bird`, `fish`, `rabbit`, `squirrel`, `turtle`,
`snail`, `bug`, `ghost`, `skull`, `crown`, `star`, `heart`, `zap`,
`flame`, `leaf`, `flower`, `sun`, `moon`, `cloud`, `umbrella`, `anchor`,
`bike`, `car`, `plane`, `rocket`, `gamepad-2`, `music`, `camera`, `coffee`

Slugs are the kebab-case lucide component names (`gamepad-2` → `Gamepad2`).

### Color allowlist (16 Tailwind hue slugs)

`red`, `orange`, `amber`, `yellow`, `lime`, `green`, `emerald`, `teal`,
`cyan`, `sky`, `blue`, `indigo`, `violet`, `purple`, `fuchsia`, `pink`

Rendered by the frontend as the Tailwind 500-level tone of each hue;
`fuchsia` may be tuned toward the brand primary `#BD51CF`.

The Go allowlists are the canonical source; the frontend keeps mirrored
constants guarded by a sync test (see Testing).

## Data model & migration

One new migration per engine (`internal/infra/storage/migrations/{sqlite,pgsql}`):

1. `ALTER TABLE users RENAME COLUMN avatar_url TO avatar;`
2. `UPDATE users SET avatar = 'user:fuchsia';`

Both statements are supported natively by SQLite and PostgreSQL and produce
identical results. The column stays `VARCHAR(255) NOT NULL`. After the
migration the column always holds a valid `<icon>:<color>` value, so **no
read path anywhere needs fallback or detection logic** — every feature that
embeds a user (account owners, budget participants, connections,
transactions) passes the stored value through unchanged.

sqlc queries referencing `avatar_url` (user, and any feature query selecting
user embeds) are renamed to `avatar` and regenerated for both engines.

## Backend

### Model

- `model.User.AvatarURL` → `model.User.Avatar`; `NewUser` takes the avatar
  value instead of a URL.
- `User.UpdateEmail` drops its `avatarURL` parameter — the avatar is no
  longer email-derived. `admin change-email` and the `data:remove-salt`
  migration stop touching it. The gravatar URL computation (`md5Hex` usage
  in `internal/user`) is removed where it fed avatars (the identifier
  derivation `md5(lower(email))` is unrelated and stays).
- New `User.UpdateAvatar(value string, now time.Time)` mutator.
- Allowlists live as exported constants in `internal/user` (e.g.
  `avatar.go`: `AvatarIcons`, `AvatarColors`) with helpers
  `IsValidAvatarIcon/Color` and `JoinAvatar(icon, color)`.

### Registration randomness (test-determinism seam)

Random defaults would make the golden-file and engine-comparison suites
nondeterministic (register-user returns the created user). Following the
existing `Clock` seam pattern:

- The user service takes a small avatar-picker seam (single-method
  interface or `func() string` returning a joined `<icon>:<color>` value).
- Production wiring (`server.BuildAPI` and the CLI container) supplies a
  `math/rand/v2`-based picker over the allowlists.
- Test harnesses supply a deterministic stub (fixed value, e.g.
  `user:fuchsia`), keeping goldens and enginecompare byte-stable.

`user:create` (CLI) uses the same picker as registration.

### API endpoint

`POST /api/v1/user/update-avatar` (auth required), registered in
`internal/user/api/routes.go`, handler `internal/user/api/avatar.go` via the
standard `endpoint.Handle` combinator.

Request DTO (`internal/model/user_dto.go`):

```json
{"icon": "cat", "color": "emerald"}
```

Validation (exact strings, standard 400 envelope):

- blank `icon` / blank `color` → field error
  `"This value should not be blank."`, code `IS_BLANK_ERROR`.
- unknown `icon` / unknown `color` (not in allowlist) → field error
  `"The value you selected is not a valid choice."`, code
  `NO_SUCH_CHOICE_ERROR`.

Success: stores `"<icon>:<color>"` via a repository update, returns the
standard success envelope with the updated user data payload (same shape as
`update-name`'s response — match the existing update-* convention).

## Frontend

### `UserAvatar` component (the single render path)

`web/src/components/UserAvatar.tsx`: takes the `avatar` string (+ size
variant), splits on `:`, maps the icon slug to its lucide component and the
color slug to background classes, renders the glyph centered on the rounded
colored square (matching the current rounded-square avatar look). Unknown
slug (defensive only) renders the `user` glyph on `fuchsia`.

Replaces every `<img src={avatar}>` site: `UserCard` (sidebar + profile),
`ApplicationLayout`, `ShareAccessDialog`, `PreviewConnectionDialog`,
`DeclineAccessDialog`, and any other embed render found during
implementation. The `?s=100` Gravatar size params disappear with the imgs.

### Picker

`AvatarPickerDialog` (follows the `CurrencyPickerDialog` dialog pattern):
opened by clicking the avatar on `/settings/profile` — the avatar gets
button semantics (focusable, `aria-label`). Contents: a grid of the 32
icons, a row/grid of the 16 color swatches, live preview, save + cancel.
Save calls a new `updateAvatar` API client fn + TanStack mutation that
invalidates the user-data query (same pattern as `useUpdateName`).
New i18n keys added to all locale files.

Mirrored frontend constants (`web/src/lib/avatars.ts`): icon slug →
lucide component map, color slug → class map, in allowlist order.

## Testing

- **Backend unit:** allowlist validation, `JoinAvatar`, DTO `Validate()`
  cases (blank / unknown / valid), registration uses the picker seam,
  update-avatar use case happy path + persistence round-trip.
- **apiparity:** one new scenario for `update-avatar` (guard counts grow by
  one); regenerate goldens with `UPDATE_GOLDEN=1` — every golden embedding
  an `avatar` value changes from a Gravatar URL to the stub value; inspect
  the diff, expect no other changes.
- **enginecompare / pgsql rerun:** covered automatically by the existing
  suites once queries are regenerated (`make test`).
- **Frontend vitest:** `UserAvatar` (parsing, icon/color mapping, fallback),
  `AvatarPickerDialog` (select + save flow, mutation called), fixtures move
  from URL values to `"cat:emerald"`-style values.
- **Sync guard:** a vitest reads the Go allowlist source
  (`internal/user/avatar.go`) from the repo and asserts the frontend
  constants match it exactly (names and order), so the two lists cannot
  drift silently.

## Documentation

- Update CLAUDE.md: the wire-contract section documents `avatar` as
  `"<icon>:<color>"`; remove Gravatar references (user feature notes).
- `.env.example` / README untouched (no config surface added).

## Out of scope

- Custom image upload.
- Per-user color theming beyond the avatar.
- Any deprecation window for third-party clients (the bundled SPA is the
  only supported client).
