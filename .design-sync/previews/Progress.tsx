import { Progress } from 'web'

export const BudgetSpend = () => (
  <div className="flex w-80 flex-col gap-4">
    <div className="flex flex-col gap-1.5">
      <div className="flex items-baseline justify-between text-sm">
        <span>Groceries</span>
        <span className="text-muted-foreground">$385.20 of $600.00</span>
      </div>
      <Progress value={64} aria-label="Groceries budget" />
    </div>
    <div className="flex flex-col gap-1.5">
      <div className="flex items-baseline justify-between text-sm">
        <span>Restaurants</span>
        <span className="text-muted-foreground">$91.40 of $200.00</span>
      </div>
      <Progress value={46} aria-label="Restaurants budget" />
    </div>
    <div className="flex flex-col gap-1.5">
      <div className="flex items-baseline justify-between text-sm">
        <span>Transport</span>
        <span className="text-muted-foreground">$135.00 of $150.00</span>
      </div>
      <Progress value={90} aria-label="Transport budget" />
    </div>
  </div>
)

export const Overspent = () => (
  <div className="flex w-80 flex-col gap-1.5">
    <div className="flex items-baseline justify-between text-sm">
      <span>Entertainment</span>
      <span className="text-expense">$742.10 of $600.00</span>
    </div>
    <Progress value={100} aria-label="Entertainment budget" className="[&>*]:bg-red-600" />
  </div>
)

export const ImportProgress = () => (
  <div className="flex w-80 flex-col gap-1.5">
    <span className="text-sm text-muted-foreground">Importing transactions… 124 of 200</span>
    <Progress value={62} aria-label="import progress" />
  </div>
)
