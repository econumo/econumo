import { Collapsible, CollapsibleContent, CollapsibleTrigger } from 'web'
import { ChevronDown, ChevronRight } from 'lucide-react'

export const FolderOpen = () => (
  <Collapsible defaultOpen className="w-80">
    <CollapsibleTrigger className="flex w-full items-center justify-between rounded-md px-2 py-1.5 text-sm font-medium hover:bg-accent">
      <span className="flex items-center gap-2">
        <ChevronDown className="size-4 text-muted-foreground" />
        Accounts
      </span>
      <span className="text-muted-foreground">$10,870.80</span>
    </CollapsibleTrigger>
    <CollapsibleContent>
      <div className="space-y-1 pt-1 pl-8">
        <div className="flex justify-between rounded-md px-2 py-1.5 text-sm hover:bg-accent">
          <span>Main account</span>
          <span className="text-muted-foreground">$2,450.80</span>
        </div>
        <div className="flex justify-between rounded-md px-2 py-1.5 text-sm hover:bg-accent">
          <span>Savings</span>
          <span className="text-muted-foreground">$8,300.00</span>
        </div>
        <div className="flex justify-between rounded-md px-2 py-1.5 text-sm hover:bg-accent">
          <span>Cash</span>
          <span className="text-muted-foreground">$120.00</span>
        </div>
      </div>
    </CollapsibleContent>
  </Collapsible>
)

export const FolderClosed = () => (
  <Collapsible className="w-80">
    <CollapsibleTrigger className="flex w-full items-center justify-between rounded-md px-2 py-1.5 text-sm font-medium hover:bg-accent">
      <span className="flex items-center gap-2">
        <ChevronRight className="size-4 text-muted-foreground" />
        Shared with me
      </span>
      <span className="text-muted-foreground">€1,240.00</span>
    </CollapsibleTrigger>
    <CollapsibleContent>
      <div className="pt-1 pl-8 text-sm">Family budget · €1,240.00</div>
    </CollapsibleContent>
  </Collapsible>
)
