import { EntityIcon } from 'web'

// Material-icon ligature renderer used everywhere an account/category/tag
// shows its icon (BudgetTable, AccountPage, ClassificationList, ...).

const catalogue = [
  'account_balance',
  'credit_card',
  'savings',
  'wallet',
  'shopping_cart',
  'restaurant',
  'directions_car',
  'payments',
]

export const IconGrid = () => (
  <div className="grid w-fit grid-cols-4 gap-3">
    {catalogue.map((n) => (
      <div key={n} className="flex w-24 flex-col items-center gap-1.5 rounded-lg border bg-card px-2 py-3">
        <EntityIcon name={n} className="text-2xl text-muted-foreground" />
        <span className="max-w-full truncate text-[10px] text-muted-foreground">{n}</span>
      </div>
    ))}
  </div>
)

export const BrandColors = () => (
  <div className="flex items-center gap-6">
    <div className="flex items-center gap-2">
      <EntityIcon name="trending_up" className="text-2xl text-income" />
      <span className="text-sm">Salary</span>
    </div>
    <div className="flex items-center gap-2">
      <EntityIcon name="shopping_cart" className="text-2xl text-expense" />
      <span className="text-sm">Groceries</span>
    </div>
    <div className="flex items-center gap-2">
      <EntityIcon name="account_balance_wallet" className="text-2xl text-econumo-magenta" />
      <span className="text-sm">Main account</span>
    </div>
  </div>
)

export const Sizes = () => (
  <div className="flex items-end gap-6">
    <div className="flex flex-col items-center gap-1">
      <EntityIcon name="restaurant" className="text-base text-muted-foreground" />
      <span className="text-[10px] text-muted-foreground">text-base (list row)</span>
    </div>
    <div className="flex flex-col items-center gap-1">
      <EntityIcon name="restaurant" className="text-lg text-muted-foreground" />
      <span className="text-[10px] text-muted-foreground">text-lg (budget table)</span>
    </div>
    <div className="flex flex-col items-center gap-1">
      <EntityIcon name="restaurant" className="text-2xl text-muted-foreground" />
      <span className="text-[10px] text-muted-foreground">text-2xl (page header)</span>
    </div>
  </div>
)

export const Fallback = () => (
  <div className="flex items-center gap-2">
    <EntityIcon className="text-2xl text-muted-foreground" />
    <span className="text-sm text-muted-foreground">no icon set → question_mark</span>
  </div>
)
