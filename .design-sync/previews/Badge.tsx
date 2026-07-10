import { Badge } from 'web'
import { Repeat, Wallet } from 'lucide-react'

export const Variants = () => (
  <div className="flex flex-wrap items-center gap-3">
    <Badge>Income</Badge>
    <Badge variant="secondary">Groceries</Badge>
    <Badge variant="destructive">Overspent</Badge>
    <Badge variant="outline">USD</Badge>
    <Badge variant="ghost">Draft</Badge>
    <Badge variant="link">View report</Badge>
  </div>
)

export const TransactionTags = () => (
  <div className="flex flex-wrap items-center gap-2">
    <Badge variant="secondary">Vacation</Badge>
    <Badge variant="secondary">Family</Badge>
    <Badge variant="secondary">Subscriptions</Badge>
    <Badge variant="secondary" className="max-w-24">
      <span className="truncate">Reimbursable business travel</span>
    </Badge>
  </div>
)

export const WithIcon = () => (
  <div className="flex flex-wrap items-center gap-3">
    <Badge variant="secondary">
      <Wallet data-icon="inline-start" />
      Cash
    </Badge>
    <Badge variant="outline">
      <Repeat data-icon="inline-start" />
      Recurring
    </Badge>
  </div>
)
