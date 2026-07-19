import { Separator } from 'web'

export const SectionDivider = () => (
  <div className="w-80">
    <div>
      <h4 className="text-sm font-medium">Main account</h4>
      <p className="text-sm text-muted-foreground">Personal · USD</p>
    </div>
    <Separator className="my-3" />
    <div className="space-y-1 text-sm">
      <div className="flex justify-between">
        <span>Groceries</span>
        <span className="text-expense">−$385.20</span>
      </div>
      <div className="flex justify-between">
        <span>Salary</span>
        <span className="text-income">+$4,200.00</span>
      </div>
    </div>
  </div>
)

export const VerticalDivider = () => (
  <div className="flex h-5 items-center gap-3 text-sm">
    <span>Income</span>
    <Separator orientation="vertical" />
    <span>Expenses</span>
    <Separator orientation="vertical" />
    <span>Transfers</span>
  </div>
)

export const ListDivider = () => (
  <div className="w-80 rounded-md border">
    <div className="flex justify-between p-3 text-sm">
      <span>Cash</span>
      <span className="text-muted-foreground">$120.00</span>
    </div>
    <Separator />
    <div className="flex justify-between p-3 text-sm">
      <span>Savings</span>
      <span className="text-muted-foreground">$8,300.00</span>
    </div>
    <Separator />
    <div className="flex justify-between p-3 text-sm">
      <span>Main account</span>
      <span className="text-muted-foreground">$2,450.80</span>
    </div>
  </div>
)
