---
name: add-language
description: Translate the Econumo app into a new language. Use whenever the user asks to translate the app, add a language ("add German", "translate to Spanish", "Polish support"), add a locale, or localize the UI — even if they never say "i18n" or "translation catalogue". Covers authoring the locales/<lang>.json catalogue, the backend/frontend wiring, and the guard tests that must pass.
---

# Adding a language to Econumo

Both stacks share the same translation catalogues: `locales/{en,ru,...}.json` is
embedded into the Go binary (`locales/embed.go`, `//go:embed *.json`) and imported
by the SPA via Vite JSON imports. Adding a language is therefore **one new
catalogue file plus a handful of small wiring edits** — everything else (the
`Accept-Language` middleware, the `update-language` endpoint's validation, the
i18ntest guard's language list) derives from `i18n.Supported` automatically.

Work through the steps in order; step 5 is the safety net that catches any slip
in steps 1–4.

## 1. Author `locales/<lang>.json`

`<lang>` is the lowercase two-letter ISO 639-1 tag (`de`, `es`, `pl`).

Copy the **exact key tree** of `en.json` and translate only the values. The
guard tests enforce key parity and `{placeholder}` parity per key, so:

- Keep every `{name}` placeholder verbatim — translate around it, never rename it.
- Add no keys, drop no keys, restructure nothing.

Translate *all* namespaces, including `errors.*` (the localized error catalogue
the SPA renders from error codes) and `emails.*` (backend-rendered mail). The
frozen English strings in the API envelope live in Go code and are a separate,
untouchable contract — they are not part of this task.

**Plurals** are authored as a single pipe-joined value (i18next plural suffixes
are NOT used). `web/src/lib/plural.ts` splits on `" | "` and picks:

- **2 variants** (`"{count} item | {count} items"`): first when `count === 1`,
  last otherwise. Use for languages that only distinguish one/other (German,
  Spanish, French, ...).
- **3+ variants**: `Intl.PluralRules` category — variant 0 for `one`, variant 1
  for `few`, the last for everything else. Use 3 variants for languages with a
  `few` category (Russian, Polish, ...), ordered `one | few | many`.
- Languages with richer plural systems than one/few/other (e.g. Arabic) must
  collapse into these three slots — pick the most natural collapse and say so
  in the PR description.

Find every plural key by grepping `en.json` for `" | "` and make sure your
translation has the right variant count for the target language — placeholder
parity won't catch a missing variant.

**Quality — translate from BOTH `en.json` and `ru.json`, not en alone.**
English UI strings are short and often ambiguous out of context: "Clear" could
mean *erase* or *transparent*, "Transfer" could be a noun or a verb, "Balance"
has several senses in a finance app. The Russian translation was written by
someone who knew the intended meaning, so reading the two side by side pins
down what each key actually says — when your translation of a key wouldn't
agree with *both* sources, you've probably misread the English. `ru.json` also
answers questions `en.json` can't:

- **Register**: Russian addresses the user formally («Вы уверены…», «Ваше
  имя»), so the app's established voice is formal — pick whatever register is
  conventional for software UI in the target language, but let ru's formality
  inform the call for languages where it's a real choice (German Sie, not du).
- **What stays untranslated**: the "Econumo" brand, currency codes, icon
  names, format strings — visible in what ru left alone.

Pick one term per app concept (budget, envelope, payee, tag, ...) and use it
consistently across all namespaces; inconsistency across the ~1200 lines is
the most common quality failure.

## 2. Backend wiring — one line

Append the tag to `Supported` in `internal/infra/i18n/i18n.go`:

```go
var Supported = []string{"en", "ru", "de"}
```

Index 0 is the English fallback — append, never reorder. This is the only
mandatory backend edit; the embed glob picks the new file up, and language
validation (`newLanguage` in `internal/user/usecase.go`), the middleware, and
the guard tests all read `i18n.Supported`.

Also add the tag to the hardcoded `[]string{"en", "ru"}` list in
`locales/embed_test.go` so the embed-and-parse test covers the new catalogue.

## 3. Frontend wiring — three files

- `web/src/app/i18n.ts`: import the JSON and register it in `resources`,
  mirroring the existing `en`/`ru` lines.
- `web/src/lib/config.ts` `getLocaleOptions()`: add
  `{ value: 'de', label: 'Deutsch', short: 'DE' }` — `label` is the language's
  own name *in that language*, `short` is two uppercase letters in the
  language's own script (compare `РУ` for Russian).
- `web/src/lib/calendarLocale.ts`: map the tag to its `react-day-picker/locale`
  export so calendar captions/weekdays follow the app language. **No guard test
  catches a miss here** — forget it and calendars silently stay English.

The language dialog/badge and the browser-language detection in `locale()` all
derive from `getLocaleOptions()`. Don't treat the list above as gospel,
though — new per-language wiring can appear after this skill was written. The
reliable check is to grep for how the existing non-English language is wired
and mirror every hit:

```bash
grep -rn "\bru\b" web/src/lib web/src/app web/src/components
```

(That grep is exactly how `calendarLocale.ts` was found after an earlier
version of this skill missed it.)

## 4. Check for hardcoded-tag collisions

Some tests use a specific tag as an example of an *unsupported* locale —
notably `web/src/lib/config.test.ts` uses `'de'` for "ignores an unsupported
stored locale". If your new language is used that way anywhere, the test's
premise is now false and it will fail. Run:

```bash
grep -rn "'<lang>'" web/src internal
```

and swap any "unsupported example" usage to a tag that is still unsupported
(don't weaken the test — keep its intent, change only the sample tag).

## 5. Verify

```bash
make go-test                                      # includes the i18ntest guards
cd web && pnpm test && pnpm lint && pnpm exec tsc -b
```

`make go-test` runs `internal/test/i18ntest`: key parity against `en.json`,
`{placeholder}` parity per key, two-way `errors.*` ↔ `errs.AllCodes` coverage,
and `emails.*` coverage — all for every language in `i18n.Supported`, so your
new catalogue is under guard the moment step 2 lands. `pnpm test`/`tsc -b`
catch the frontend wiring and any tag collision from step 4 (vitest and oxlint
do not type-check — `tsc -b` is a required part of the gate).
