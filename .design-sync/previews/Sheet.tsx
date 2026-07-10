import {
  Button,
  Input,
  Label,
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from 'web'

export const EditAccountSheet = () => (
  <Sheet open>
    <SheetContent side="right" onOpenAutoFocus={(e) => e.preventDefault()}>
      <SheetHeader>
        <SheetTitle>Edit account</SheetTitle>
        <SheetDescription>
          Update the details of “Main account”.
        </SheetDescription>
      </SheetHeader>
      <div className="grid gap-4 px-4">
        <div className="grid gap-2">
          <Label htmlFor="sheet-account-name">Name</Label>
          <Input id="sheet-account-name" defaultValue="Main account" />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="sheet-account-balance">Starting balance</Label>
          <Input id="sheet-account-balance" defaultValue="4,200.00" />
        </div>
      </div>
      <SheetFooter>
        <Button>Save changes</Button>
        <Button variant="outline">Cancel</Button>
      </SheetFooter>
    </SheetContent>
  </Sheet>
)

export const AccountsSheetLeft = () => (
  <Sheet open>
    <SheetContent side="left" onOpenAutoFocus={(e) => e.preventDefault()}>
      <SheetHeader>
        <SheetTitle>Accounts</SheetTitle>
        <SheetDescription>Balances as of today</SheetDescription>
      </SheetHeader>
      <div className="flex flex-col gap-1 px-4 text-sm">
        <div className="flex items-center justify-between rounded-md px-2 py-2 hover:bg-muted">
          <span>Main account</span>
          <span className="font-medium">$4,200.00</span>
        </div>
        <div className="flex items-center justify-between rounded-md px-2 py-2 hover:bg-muted">
          <span>Cash</span>
          <span className="font-medium">$142.50</span>
        </div>
        <div className="flex items-center justify-between rounded-md px-2 py-2 hover:bg-muted">
          <span>Savings</span>
          <span className="font-medium">€8,050.00</span>
        </div>
      </div>
    </SheetContent>
  </Sheet>
)
