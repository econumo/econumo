# Server-rendered error translations (REST + MCP)

Date: 2026-07-20 (revised same day: translate in place, no sibling fields)

## Problem

The i18n error contract predates the backend knowing the caller's locale: the
handled-error envelope shipped frozen English `message`/`errors` plus additive
machine codes (`messageCode`, `messageParams`, `errorCodes`), and the SPA
translated the codes client-side through the shared catalogue. The backend now
resolves the caller's language on every request, so the client-side
translation hop — and any parallel translated/untranslated field pairs — are
unnecessary indirection.

## Decision

Render `message` and the per-field `errors` strings in the caller's language
directly, on both edges. No code fields, no translated-sibling fields — the
envelope keeps its original four keys (`success`, `message`, `code`,
`errors`).

### Language resolution (both edges)

Explicit `Accept-Language` header (resolved by the `Language` middleware; the
SPA sends its selected locale on every request) → the authenticated user's
stored `users.language` → `en`. The stored fallback runs inside the auth
middleware via an optional capability interface
(`middleware.StoredLanguageResolver`) implemented by the server's
authenticator decorator (`StoredLanguage`,
`internal/server/glue_language.go`), so every authenticated route on REST and
`/mcp` gets it without per-feature wiring — including the read-only 402
written inside the middleware itself. Unauthenticated routes resolve
header-or-`en`.

### Rendering rule (`httpx.WriteError`, `mcp.MapErr`)

- An error carrying a catalogue code (`errors.*` keys in `locales/*.json`,
  registry `errs.AllCodes`) renders the catalogue text via `i18n.Lookup`
  into `message` (fieldless) or the field's `errors` entry.
- Fallback chain when things fail: a language missing the key falls back to
  the English catalogue (inside `i18n.Lookup`); a code absent from every
  catalogue keeps the error's literal English text — a dotted key never
  reaches the wire.
- An error with no code keeps its literal English text — nothing to look up;
  coverage grows as codes are added.
- The field-validation top-level label stays the literal
  `"Form validation error"` (no catalogue key); clients show the per-field
  text.
- The `en` catalogue text matches the historical frozen strings, so English
  callers see the same wire bytes as before. (Exception, deliberate: the
  legacy dotted literals such as `transaction.transaction.not_available`
  now render as readable text via their codes.)

### SPA (`web/src`)

- `apiErrorMessage`: first `errors` entry when present (the top-level message
  is then the generic form label), else `message`, else the generic
  `common.app.error`. No client-side catalogue lookup.
- `apiFieldErrors(err, field)`: the envelope's `errors[field]`, as rendered by
  the backend (used for inline form errors, e.g. ProfilePage).
- The `errors.*` catalogue keys stay in `locales/*.json` — backend-consumed on
  both edges; all i18ntest guards unchanged.

## Rejected alternatives

- **Additive `messageTranslated`/`errorsTranslated` sibling fields** (the
  first iteration of this design): kept the envelope frozen-English but
  duplicated every string on the wire and left two sources of truth for
  clients to prefer between.
- **Keep client-side code translation**: requires every client to embed the
  catalogue; the backend already knows the locale.

## Testing

- `apiparity`/`mcpparity` goldens regenerated (run in `en`): the diff is
  exactly the removal of the code fields; all `en` messages byte-identical to
  the historical strings.
- Unit tests cover: in-place `ru` rendering (fieldless + per-field), literal
  passthrough for code-less errors, the 402 translation, and the
  auth-middleware stored-language fallback (header wins / stored used / no
  stored / resolver-less authenticator).
- Full gates: `make go-test`, `pnpm test`, `pnpm exec tsc -b`, oxlint.
