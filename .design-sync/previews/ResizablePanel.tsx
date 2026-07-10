import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from 'web'

export const TwoPaneSplit = () => (
  <div className="h-48 w-[28rem] overflow-hidden rounded-lg border">
    <ResizablePanelGroup>
      <ResizablePanel defaultSize={38} minSize={20}>
        <div className="flex h-full flex-col gap-1 p-4 text-sm">
          <span className="font-medium">Accounts</span>
          <span className="flex justify-between text-muted-foreground">
            <span>Main account</span>
            <span>$2,450.80</span>
          </span>
          <span className="flex justify-between text-muted-foreground">
            <span>Savings</span>
            <span>€8,120.00</span>
          </span>
          <span className="flex justify-between text-muted-foreground">
            <span>Cash</span>
            <span>$180.00</span>
          </span>
        </div>
      </ResizablePanel>
      <ResizableHandle withHandle />
      <ResizablePanel defaultSize={62} minSize={30}>
        <div className="flex h-full flex-col gap-1 p-4 text-sm">
          <span className="font-medium">Transactions — Main account</span>
          <span className="flex justify-between text-muted-foreground">
            <span>Groceries · Whole Foods</span>
            <span className="text-expense">−$85.40</span>
          </span>
          <span className="flex justify-between text-muted-foreground">
            <span>Salary · June</span>
            <span className="text-income">+$4,200.00</span>
          </span>
          <span className="flex justify-between text-muted-foreground">
            <span>Transport · Metro pass</span>
            <span className="text-expense">−$42.50</span>
          </span>
        </div>
      </ResizablePanel>
    </ResizablePanelGroup>
  </div>
)

export const VerticalSplit = () => (
  <div className="h-64 w-80 overflow-hidden rounded-lg border">
    <ResizablePanelGroup orientation="vertical">
      <ResizablePanel defaultSize={45} minSize={25}>
        <div className="flex h-full flex-col justify-center gap-1 p-4 text-sm">
          <span className="font-medium">June budget</span>
          <span className="text-muted-foreground">
            $1,850.00 of $2,400.00 spent
          </span>
        </div>
      </ResizablePanel>
      <ResizableHandle withHandle />
      <ResizablePanel defaultSize={55} minSize={25}>
        <div className="flex h-full flex-col gap-1 p-4 text-sm">
          <span className="font-medium">Envelopes</span>
          <span className="flex justify-between text-muted-foreground">
            <span>Groceries</span>
            <span>$385.20 / $450.00</span>
          </span>
          <span className="flex justify-between text-muted-foreground">
            <span>Restaurants</span>
            <span>$142.75 / $200.00</span>
          </span>
        </div>
      </ResizablePanel>
    </ResizablePanelGroup>
  </div>
)

export const ThreePaneSplit = () => (
  <div className="h-40 w-[30rem] overflow-hidden rounded-lg border">
    <ResizablePanelGroup>
      <ResizablePanel defaultSize={28} minSize={15}>
        <div className="flex h-full items-center justify-center p-3 text-sm font-medium">
          Accounts
        </div>
      </ResizablePanel>
      <ResizableHandle withHandle />
      <ResizablePanel defaultSize={44} minSize={20}>
        <div className="flex h-full items-center justify-center p-3 text-sm font-medium">
          Transactions
        </div>
      </ResizablePanel>
      <ResizableHandle withHandle />
      <ResizablePanel defaultSize={28} minSize={15}>
        <div className="flex h-full items-center justify-center p-3 text-sm font-medium">
          Details
        </div>
      </ResizablePanel>
    </ResizablePanelGroup>
  </div>
)
