import { NativeSelect, NativeSelectOptGroup, NativeSelectOption } from 'web'

export const AccountNativeSelect = () => (
  <NativeSelect defaultValue="main" className="w-56" aria-label="Account">
    <NativeSelectOption value="main">Main account</NativeSelectOption>
    <NativeSelectOption value="cash">Cash</NativeSelectOption>
    <NativeSelectOption value="savings">Savings</NativeSelectOption>
  </NativeSelect>
)

export const GroupedNativeSelect = () => (
  <NativeSelect defaultValue="groceries" className="w-56" aria-label="Category">
    <NativeSelectOptGroup label="Expenses">
      <NativeSelectOption value="groceries">Groceries</NativeSelectOption>
      <NativeSelectOption value="restaurants">Restaurants</NativeSelectOption>
      <NativeSelectOption value="transport">Transport</NativeSelectOption>
    </NativeSelectOptGroup>
    <NativeSelectOptGroup label="Income">
      <NativeSelectOption value="salary">Salary</NativeSelectOption>
    </NativeSelectOptGroup>
  </NativeSelect>
)

export const NativeSelectStates = () => (
  <div className="flex flex-col gap-3">
    <NativeSelect size="sm" defaultValue="usd" className="w-32" aria-label="Currency">
      <NativeSelectOption value="usd">USD</NativeSelectOption>
      <NativeSelectOption value="eur">EUR</NativeSelectOption>
    </NativeSelect>
    <NativeSelect defaultValue="cash" disabled className="w-56" aria-label="Account (disabled)">
      <NativeSelectOption value="cash">Cash</NativeSelectOption>
    </NativeSelect>
    <NativeSelect defaultValue="" aria-invalid className="w-56" aria-label="Payee">
      <NativeSelectOption value="">Payee is required</NativeSelectOption>
      <NativeSelectOption value="rewe">REWE Supermarket</NativeSelectOption>
    </NativeSelect>
  </div>
)
