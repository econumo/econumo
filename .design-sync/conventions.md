# Econumo UI — build conventions

Econumo is a personal-finance/budgeting app. Components come from
`window.EconumoUI` (React 19, shadcn-style). Compose the exported parts —
compounds ship flat: `Card` + `CardHeader` + `CardTitle` + `CardContent`,
`Dialog` + `DialogContent` + `DialogFooter`, `Select` + `SelectTrigger` +
`SelectContent` + `SelectItem`, etc.

## Setup

No provider wrapper is required — components are self-contained. Dark mode is
a `dark` class on a root element (tokens flip automatically). Toasts: mount one
`<Toaster />` and fire with `toast('…')` / `toast.success('…')` — `toast` is
exported from the bundle (`window.EconumoUI.toast`); never import sonner
separately, it won't reach the mounted host.

Charts: use `ChartContainer` with the chart primitives exported from the
bundle — `BarChart`, `Bar`, `LineChart`, `Line`, `AreaChart`, `Area`,
`PieChart`, `Pie`, `Cell`, `XAxis`, `YAxis`, `CartesianGrid` (all on
`window.EconumoUI`; never import recharts separately — same module-instance
rule as toast). Give charts an explicit `width`/`height` and colors via the
`config` prop mapping series to `var(--chart-1)`…`var(--chart-5)`.

## Styling idiom — Tailwind 4 utilities over design tokens

Style with Tailwind utility classes; never hex colors — always the token-mapped
utilities so light/dark both work:

- Surfaces: `bg-background`, `bg-card`, `bg-popover`, `bg-muted`, `bg-accent`,
  `bg-secondary`, `bg-sidebar`
- Text: `text-foreground`, `text-muted-foreground`, `text-card-foreground`,
  `text-primary`, `text-destructive`
- Money semantics (used everywhere): `text-income` (green) for positive
  amounts, `text-expense` (red) for negative
- Brand accents: `bg-econumo-yellow` + `text-econumo-yellow-text`,
  `bg-econumo-magenta`, `bg-econumo-card`, `hover:bg-econumo-hover`
- Borders/radius: `border-border`, `rounded-lg` (radius scale derives from
  `--radius`), focus ring `ring-ring`
- Charts: `--chart-1` … `--chart-5` vars (via ChartContainer config)

Type is Roboto (`font-sans`, already the default). **Brand rule:** action
buttons render UPPERCASE automatically (a global stylesheet rule) — don't
uppercase text yourself and don't fight it; value-bearing triggers (Select,
date pickers) stay sentence case by the same rule.

Entity icons (accounts/categories) are Material Icons **ligature names**:
`<EntityIcon icon="shopping_cart" color="#4E8F26" />` or
`<span className="material-icon">wallet</span>`. Lucide icons are for UI
chrome only (chevrons, plus, trash).

## Where the truth lives

- `styles.css` → imports `_ds_bundle.css` (all tokens under `:root` /
  `.dark`, every utility, the uppercase-button rule) — read it before
  inventing a class; a class absent there does nothing.
- Per-component API: `components/<group>/<Name>/<Name>.d.ts`; usage examples
  in `<Name>.prompt.md`.

## Idiomatic example

```jsx
const { Card, CardHeader, CardTitle, CardDescription, CardContent, Button } =
  window.EconumoUI;

<Card className="w-80">
  <CardHeader>
    <CardTitle>Main account</CardTitle>
    <CardDescription>Personal · USD</CardDescription>
  </CardHeader>
  <CardContent className="space-y-1">
    <div className="flex justify-between text-sm">
      <span>Groceries</span><span className="text-expense">−$385.20</span>
    </div>
    <div className="flex justify-between text-sm">
      <span>Salary</span><span className="text-income">+$4,200.00</span>
    </div>
    <Button size="sm" className="mt-3">Add transaction</Button>
  </CardContent>
</Card>
```

Dialogs are the app's write pattern: `ResponsiveDialog` (or `Dialog` +
`DialogContent`) with a two-column footer `grid grid-cols-2 gap-3` —
outline Cancel on the left, primary/destructive action on the right.
