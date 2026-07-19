import { EntityIcon, SortableList, Switch } from 'web'
import { GripVertical } from 'lucide-react'

// dnd-kit vertical reorder list (ClassificationList, account/category
// settings). Static resting state only — drag-in-motion can't render.

const accounts = [
  { id: 'a1', name: 'Main account', icon: 'account_balance', balance: '$2,450.10', negative: false },
  { id: 'a2', name: 'Cash', icon: 'wallet', balance: '$180.00', negative: false },
  { id: 'a3', name: 'Savings', icon: 'savings', balance: '$12,300.00', negative: false },
  { id: 'a4', name: 'Credit card', icon: 'credit_card', balance: '−$385.20', negative: true },
]

export const AccountRows = () => (
  <div className="w-96 rounded-lg border bg-card p-2">
    <SortableList
      items={accounts}
      onReorder={() => {}}
      renderItem={(item, handle) => (
        <div className="flex items-center gap-2 rounded-md px-1 py-1.5">
          <button
            type="button"
            aria-label={`drag ${item.name}`}
            className="cursor-grab touch-none text-muted-foreground"
            {...handle.attributes}
            {...(handle.listeners ?? {})}
          >
            <GripVertical className="size-4" />
          </button>
          <EntityIcon name={item.icon} className="text-base text-muted-foreground" />
          <span className="min-w-0 flex-1 truncate text-sm">{item.name}</span>
          <span className={`text-sm tabular-nums ${item.negative ? 'text-expense' : 'text-muted-foreground'}`}>
            {item.balance}
          </span>
        </div>
      )}
    />
  </div>
)

const categories = [
  { id: 'c1', name: 'Groceries', icon: 'shopping_cart', archived: false },
  { id: 'c2', name: 'Restaurants', icon: 'restaurant', archived: false },
  { id: 'c3', name: 'Transport', icon: 'directions_car', archived: false },
  { id: 'c4', name: 'Subscriptions', icon: 'subscriptions', archived: true },
]

export const CategorySettingsRows = () => (
  <div className="w-96 rounded-lg border bg-card p-2">
    <SortableList
      items={categories}
      onReorder={() => {}}
      renderItem={(item, handle) => (
        <div className="flex items-center gap-2 rounded-md px-1 py-1.5">
          <button
            type="button"
            aria-label={`drag ${item.name}`}
            className="cursor-grab touch-none text-muted-foreground"
            {...handle.attributes}
            {...(handle.listeners ?? {})}
          >
            <GripVertical className="size-4" />
          </button>
          <EntityIcon name={item.icon} className="text-base text-muted-foreground" />
          <span className="flex min-w-0 flex-1 flex-col">
            <span className={`truncate text-sm ${item.archived ? 'text-muted-foreground' : ''}`}>{item.name}</span>
            {item.archived ? <span className="text-xs text-muted-foreground">Archived</span> : null}
          </span>
          <Switch aria-label={`archive ${item.name}`} checked={!item.archived} />
        </div>
      )}
    />
  </div>
)
