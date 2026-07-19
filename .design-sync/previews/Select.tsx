import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
} from 'web'

export const AccountSelect = () => (
  <Select defaultValue="main">
    <SelectTrigger className="w-56">
      <SelectValue placeholder="Select account" />
    </SelectTrigger>
    <SelectContent>
      <SelectItem value="main">Main account</SelectItem>
      <SelectItem value="cash">Cash</SelectItem>
      <SelectItem value="savings">Savings</SelectItem>
    </SelectContent>
  </Select>
)

export const CurrencySelectSmall = () => (
  <Select defaultValue="usd">
    <SelectTrigger size="sm" className="w-32">
      <SelectValue placeholder="Currency" />
    </SelectTrigger>
    <SelectContent>
      <SelectItem value="usd">USD</SelectItem>
      <SelectItem value="eur">EUR</SelectItem>
    </SelectContent>
  </Select>
)

export const SelectStates = () => (
  <div className="flex flex-col gap-3">
    <Select>
      <SelectTrigger className="w-56">
        <SelectValue placeholder="Select category" />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="groceries">Groceries</SelectItem>
        <SelectItem value="transport">Transport</SelectItem>
      </SelectContent>
    </Select>
    <Select defaultValue="cash" disabled>
      <SelectTrigger className="w-56">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="cash">Cash</SelectItem>
      </SelectContent>
    </Select>
    <Select>
      <SelectTrigger className="w-56" aria-invalid>
        <SelectValue placeholder="Payee is required" />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="rewe">REWE Supermarket</SelectItem>
      </SelectContent>
    </Select>
  </div>
)

export const OpenSelect = () => (
  <Select defaultValue="groceries" open>
    <SelectTrigger className="w-56">
      <SelectValue placeholder="Select category" />
    </SelectTrigger>
    <SelectContent>
      <SelectGroup>
        <SelectLabel>Expenses</SelectLabel>
        <SelectItem value="groceries">Groceries</SelectItem>
        <SelectItem value="restaurants">Restaurants</SelectItem>
        <SelectItem value="transport">Transport</SelectItem>
      </SelectGroup>
      <SelectSeparator />
      <SelectGroup>
        <SelectLabel>Income</SelectLabel>
        <SelectItem value="salary">Salary</SelectItem>
      </SelectGroup>
    </SelectContent>
  </Select>
)
