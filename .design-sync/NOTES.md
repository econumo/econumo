# design-sync notes — Econumo (web/)

Repo-specific gotchas for future syncs. The config lives in
`.design-sync/config.json`; durable sync inputs in `.design-sync/`.

## Source shape

- `web/` is a Vite **app**, not a library: no dist entry, no shipped `.d.ts`.
  The sync builds both itself (see `buildCmd`):
  - `web/tsconfig.dts.json` emits a declaration tree to `web/dist/types/`;
    `.design-sync/gen-dts-barrel.mjs` writes the `index.d.ts` barrel;
    `web/package.json` `"types"` points at it (harmless for the app — private
    package, nothing consumes the field at runtime).
  - `cfg.entry` (`web/dist/index.es.js`) deliberately does NOT exist — that
    routes the converter to PKG_DIR=web/ and synth-entry mode (bundles all 76
    src files under `src/components/`). The `[NO_DIST]` warns it prints are
    expected, not a failure.
- `.design-sync/overrides/dts.mjs` fork: shadcn flat exports carry no
  compound signal; the fork folds same-source-file PascalCase-prefix exports
  (CardHeader→Card) into compounds. 333 exports → 75 root cards. On re-sync,
  diff the fork against the bundled lib/dts.mjs and merge upstream changes.

## CSS / Tailwind 4

- The app CSS can't ship as-built: Tailwind emits only classes the app uses,
  so preview/agent layout glue (w-80 etc.) silently missed. buildCmd compiles
  `dist/econumo.css` from `.design-sync/ds.css` = app `index.css` + extra
  `@source`s: `.design-sync/previews/` and the generated utility corpus
  (`gen-utility-corpus.mjs` → `.cache/utility-corpus.txt`, ~1400 candidates).
  A utility missing from a preview render usually means it's not in the
  corpus — extend `gen-utility-corpus.mjs`.
- `@tailwindcss/cli` was added to web devDependencies for this (the app build
  itself still uses the Vite plugin). `pnpm-workspace.yaml` pins
  `allowBuilds: '@parcel/watcher': false` (CLI dep; its build script is only
  needed for watch mode — pnpm 11 blocks unapproved builds otherwise).

## Fonts

- Roboto ships via @fontsource imports in index.css; `cfg.extraFonts` lists
  the four @fontsource css files so the woff2s copy into fonts/ and the
  bundle css urls rewrite.
- Material Icons: the app serves the font from `public/fonts/` at an
  absolute `/fonts/` URL the converter can't resolve —
  `.design-sync/material-icons.css` is a twin @font-face with an on-disk
  relative URL, listed in extraFonts. Entity icons (account/category) are
  Material icon **ligature names** rendered via the `.material-icon` class.
  **The twin's url() must track the app's font FILENAME**: the 2026-07-11
  rename to `material-symbols-rounded.woff2` left the twin pointing at a
  gone file — the dead @font-face was silently dropped and every icon
  captured as tofu until `[FONT_DANGLING]` flagged it (2026-07-16 sync).

## Brand facts (don't "fix" these in previews)

- Action buttons render UPPERCASE + letter-spacing (Quasar parity rule in
  index.css); value-bearing triggers (selects, date pickers) stay sentence case.
- Primary = econumo magenta (#BD51CF family); `text-income` green,
  `text-expense` red; font Roboto; `--radius: 0.625rem`.

## Components

- Grouping comes from `.design-sync/docs/<Name>.md` frontmatter categories
  (`cfg.docsDir`) — 75 stubs, one per root; their body line feeds .prompt.md.
- `componentSrcMap` nulls: ChartStyle/ChartLegend(+Content)/ChartTooltip(+Content)
  (helpers documented under ChartContainer), DirectionProvider, ScrollBar
  (belongs to ScrollArea), ResizableHandle (composed in ResizablePanel).
  All remain importable bundle exports — they just have no standalone card.
- i18n: CoinLoader, CurrencyPickerDialog, FailDialog, SortDialog call
  useTranslation; previews import '@/app/i18n' (safe standalone — locale()
  falls back to navigator.language). No react-query/router use in
  components/ — no other providers needed.
- Overlay components carry cardMode single + viewport in cfg.overrides.

## Verification environment

- playwright 1.61.0 in .ds-sync/node_modules matches the machine's cached
  chromium-1228 (~/.cache/ms-playwright). typescript + @types/react also
  installed there (d.ts parse check + prop extraction).

## Preview-authoring facts (folded from wave learnings)

- Subcomponents are FLAT named exports of 'web' (RadioGroupItem, ButtonGroupSeparator,
  InputGroupAddon…) even where a `.d.ts` models them as dot-properties — use the flat form.
- A component that throws during mount renders a BLANK cell, not the ⚠ fallback
  (React 19 surfaces render errors asynchronously past the mount try/catch).
- `cfg.provider` = `EconumoPreviewProvider` (from `.design-sync/ds-extras.tsx`, bundled
  via extraEntries): a seeded QueryClient for data-dependent components
  (CurrencySelect, CurrencyPickerDialog; since 2026-07-16 also `queryKeys.user`
  seeded with a CurrentUserDto for AvatarPickerDialog/useUserData consumers). react-query lives INSIDE the bundle, so the
  provider must be a bundle export — a preview-side QueryClientProvider from
  node_modules is a different module instance and never reaches the bundle's hooks.
- Slider: always pass explicit `defaultValue` array (omitting it renders two thumbs
  at [min,max]). Progress: h-1 — needs w-80 wrapper + label row to read as non-blank.
- Radix Select default `position="item-aligned"` overlays the popup ON the trigger in
  open shots — use `position="popper"` for below-trigger. base-ui Combobox statics:
  `items` on Root + function-child List; `defaultOpen` renders statically.
- Label disabled styling needs a `group` + `data-disabled="true"` wrapper; Field error
  needs BOTH `data-invalid` on Field and `aria-invalid` on the control.
- InputGroupButton has its own size scale (`icon-xs`), not Button's.
- Badge with a width cap center-clips both sides — put `<span className="truncate">`
  inside for ellipsis truncation.
- Calendar previews: fixed `selected`/`defaultMonth` dates + `weekStartsOn={1}`
  (app parity, deterministic capture). InputOTP controlled value needs onChange no-op.
- Checkbox indeterminate renders a check glyph on an unfilled box (component uses
  CheckIcon unconditionally) — actual behavior, graded good; candidate upstream fix.

- **Module-instance wall (the recurring theme)**: anything the bundle inlines
  (react-query, react-i18next, sonner) is a separate module instance from
  node_modules — preview-side providers/inits/emitters can NEVER reach the
  bundle's copy. All fixes are bundle-side in `.design-sync/ds-extras.tsx`:
  QueryClientProvider (seeded), `import '../web/src/app/i18n'` (initializes the
  bundle's default i18n instance), `export { toast } from 'sonner'`.
- `@/` path aliases do NOT compile in preview .tsx files (the paths plugin
  chokes on tsconfig comment-stripping for the `"@/*"` key) — previews import
  only from 'web' / node_modules; app-side setup goes through ds-extras.
- Arbitrary-value utilities (`h-[460px]`) aren't in the corpus-compiled CSS —
  use inline style for exact sizes in previews.
- Radix open-autofocus creates selection/focus-ring artifacts in captures:
  `onOpenAutoFocus={(e) => e.preventDefault()}` on Content. ContextMenu has no
  open prop — dispatch a synthetic `contextmenu` MouseEvent + `modal={false}`.
  Menubar opens statically via root `value=`; NavigationMenu via
  `defaultValue` + `viewport={false}`.
- Sidebar previews: `collapsible="none"` in a fixed-height wrapper (default
  offcanvas mode is position:fixed h-svh and escapes the cell).
- ResponsiveDialog needs viewport ≥640px wide for the desktop Dialog branch
  (below that renders the vaul Drawer branch — also true behavior).
- AlertDialog action/cancel and outline menu triggers are sentence case by
  component design (the uppercase rule targets action buttons only).

- **Dual-recharts trap**: the bundle inlines recharts; chart primitives
  (BarChart, XAxis, …) are re-exported through ds-extras so previews AND the
  design agent use the bundle's instance. In chart previews: explicit
  width/height + `isAnimationActive={false}` (ResponsiveContainer measures 0
  in cells). `--chart-1..5` are genuinely grayscale in this brand.
- Capture browser has no color-emoji font (tofu) — use lucide icons.
- react-resizable-panels v4: prop is `orientation`, group needs a sized wrapper.
- ScrollArea needs `type="always"` for a scrollbar in static captures.
- UserCard appends `?s=N` to avatar URLs — end data-URI avatars with `#`.

- Avatars (2026-07): `avatar` values are `"<icon>:<color>"` (7-slug backend
  allowlist; utilities `bg-avatar-<slug>-tint` / `text-avatar-<slug>` come from
  index.css theme vars). UserCard renders UserAvatar — preview avatars are
  these strings now, never image URLs (the old data-URI trick is dead).

## Known render warns

(triaged legitimate warn lines; a warn not listed here is new)

- none — the 2026-07-16 sync ended with zero `!` warn lines: `[FONT_DANGLING]`
  was fixed (twin css filename) and all 18 `[GRID_OVERFLOW]` flags (a check new
  to the 2.1.211 scripts) were resolved via cardMode overrides in config
  (column for wide data-display/layout cards; Toaster/Menubar single+primaryStory).

## Re-sync risks

- A rename of the Material font file under `web/public/fonts/` silently kills
  the `.design-sync/material-icons.css` twin (dead @font-face dropped, icons
  become tofu) — `[FONT_DANGLING]` is the tell; update the twin's url().
- The seeded provider user in ds-extras.tsx hand-copies the CurrentUserDto
  shape — a DTO field rename won't fail the build, components just read
  undefined.

- `web/dist/types/` and `dist/econumo.css` are build artifacts — always
  re-run cfg.buildCmd before the converter; stale types silently shrink the
  component list.
- The dts.mjs fork can drift from the upstream lib — diff on every re-sync.
- The utility corpus is a curated approximation; new preview vocabulary needs
  corpus extension (silent miss otherwise).
- Grouping doc stubs enumerate components — a NEW component lands in
  "general" until a stub is added (visible, not silent).
- pnpm 11 build-approval state lives in web/pnpm-workspace.yaml — a fresh
  clone without it fails `pnpm exec tailwindcss` with ERR_PNPM_IGNORED_BUILDS.
