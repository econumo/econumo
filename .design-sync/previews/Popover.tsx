import {
  Button,
  Input,
  Label,
  Popover,
  PopoverContent,
  PopoverDescription,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from 'web'

export const BudgetLimitPopover = () => (
  <div className="flex h-80 w-full items-start justify-center pt-2">
    <Popover defaultOpen>
      <PopoverTrigger asChild>
        <Button variant="outline" className="w-28 justify-end tabular-nums">
          385.20
        </Button>
      </PopoverTrigger>
      <PopoverContent
        className="w-64"
        align="center"
        onOpenAutoFocus={(e) => e.preventDefault()}
      >
        <PopoverHeader>
          <PopoverTitle>Groceries</PopoverTitle>
          <PopoverDescription>Monthly limit for July</PopoverDescription>
        </PopoverHeader>
        <div className="grid gap-2">
          <Label htmlFor="popover-limit">Limit</Label>
          <Input id="popover-limit" defaultValue="385.20" />
        </div>
        <Button size="sm">Save</Button>
      </PopoverContent>
    </Popover>
  </div>
)
