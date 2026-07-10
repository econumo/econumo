import {
  Button,
  Kbd,
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from 'web'
import { Plus } from 'lucide-react'

export const BalanceTooltip = () => (
  <TooltipProvider>
    <div className="flex h-48 w-full items-end justify-center pb-4">
      <Tooltip open>
        <TooltipTrigger asChild>
          <Button variant="outline">Savings</Button>
        </TooltipTrigger>
        <TooltipContent side="top">
          Balance as of today: $4,200.00
        </TooltipContent>
      </Tooltip>
    </div>
  </TooltipProvider>
)

export const ShortcutTooltip = () => (
  <TooltipProvider>
    <div className="flex h-48 w-full items-end justify-center pb-4">
      <Tooltip open>
        <TooltipTrigger asChild>
          <Button size="icon" variant="outline" aria-label="New transaction">
            <Plus />
          </Button>
        </TooltipTrigger>
        <TooltipContent side="top">
          New transaction <Kbd>N</Kbd>
        </TooltipContent>
      </Tooltip>
    </div>
  </TooltipProvider>
)
