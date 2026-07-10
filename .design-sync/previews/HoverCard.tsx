import { HoverCard, HoverCardContent, HoverCardTrigger } from 'web'
import { Wallet } from 'lucide-react'

export const AccountHoverCard = () => (
  <div className="flex justify-center pt-2">
    <HoverCard open>
      <HoverCardTrigger asChild>
        <a
          href="#main-account"
          className="text-sm font-medium underline decoration-dotted underline-offset-4"
        >
          Main account
        </a>
      </HoverCardTrigger>
      <HoverCardContent side="bottom" align="center">
        <div className="flex items-center gap-2">
          <Wallet className="size-4 text-muted-foreground" />
          <span className="font-medium">Main account</span>
          <span className="ml-auto text-xs text-muted-foreground">USD</span>
        </div>
        <div className="mt-2 text-2xl font-semibold">$1,240.50</div>
        <div className="mt-1 text-xs text-muted-foreground">
          34 transactions this month · shared with Anna
        </div>
      </HoverCardContent>
    </HoverCard>
  </div>
)

export const PayeeHoverCard = () => (
  <div className="flex justify-center pt-2">
    <HoverCard open>
      <HoverCardTrigger asChild>
        <a
          href="#rewe"
          className="text-sm font-medium underline decoration-dotted underline-offset-4"
        >
          REWE Supermarket
        </a>
      </HoverCardTrigger>
      <HoverCardContent side="bottom" align="center" className="w-72">
        <div className="font-medium">REWE Supermarket</div>
        <div className="mt-1 text-xs text-muted-foreground">
          Usually categorized as Groceries
        </div>
        <div className="mt-2 flex justify-between text-xs">
          <span className="text-muted-foreground">Last transaction</span>
          <span className="text-expense">−$42.50</span>
        </div>
        <div className="mt-1 flex justify-between text-xs">
          <span className="text-muted-foreground">This month</span>
          <span className="text-expense">−$385.20</span>
        </div>
      </HoverCardContent>
    </HoverCard>
  </div>
)
