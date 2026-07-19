import { ToggleGroup, ToggleGroupItem } from 'web'
import { ArrowLeftRight, TrendingDown, TrendingUp } from 'lucide-react'

export const SingleSelect = () => (
  <ToggleGroup type="single" defaultValue="month" aria-label="Report period">
    <ToggleGroupItem value="day">Day</ToggleGroupItem>
    <ToggleGroupItem value="week">Week</ToggleGroupItem>
    <ToggleGroupItem value="month">Month</ToggleGroupItem>
    <ToggleGroupItem value="year">Year</ToggleGroupItem>
  </ToggleGroup>
)

export const OutlineAttached = () => (
  <ToggleGroup
    type="single"
    variant="outline"
    spacing={0}
    defaultValue="expense"
    aria-label="Transaction type"
  >
    <ToggleGroupItem value="expense">
      <TrendingDown />
      Expense
    </ToggleGroupItem>
    <ToggleGroupItem value="income">
      <TrendingUp />
      Income
    </ToggleGroupItem>
    <ToggleGroupItem value="transfer">
      <ArrowLeftRight />
      Transfer
    </ToggleGroupItem>
  </ToggleGroup>
)

export const MultipleSelect = () => (
  <ToggleGroup
    type="multiple"
    variant="outline"
    defaultValue={['groceries', 'transport']}
    aria-label="Filter categories"
  >
    <ToggleGroupItem value="groceries">Groceries</ToggleGroupItem>
    <ToggleGroupItem value="restaurants">Restaurants</ToggleGroupItem>
    <ToggleGroupItem value="transport">Transport</ToggleGroupItem>
    <ToggleGroupItem value="salary">Salary</ToggleGroupItem>
  </ToggleGroup>
)

export const SmallAndDisabled = () => (
  <div className="flex flex-col items-start gap-4">
    <ToggleGroup type="single" size="sm" variant="outline" spacing={0} defaultValue="usd">
      <ToggleGroupItem value="usd">USD</ToggleGroupItem>
      <ToggleGroupItem value="eur">EUR</ToggleGroupItem>
    </ToggleGroup>
    <ToggleGroup type="single" variant="outline" spacing={0} defaultValue="week" disabled>
      <ToggleGroupItem value="week">Week</ToggleGroupItem>
      <ToggleGroupItem value="month">Month</ToggleGroupItem>
    </ToggleGroup>
  </div>
)
