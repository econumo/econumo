import { Checkbox, Input, Label } from 'web'

export const InputLabel = () => (
  <div className="flex w-64 flex-col gap-2">
    <Label htmlFor="account-name">Account name</Label>
    <Input id="account-name" defaultValue="Main account" />
  </div>
)

export const CheckboxLabel = () => (
  <Label>
    <Checkbox defaultChecked />
    Include archived accounts
  </Label>
)

export const DisabledLabel = () => (
  <div className="group flex w-64 flex-col gap-2" data-disabled="true">
    <Label htmlFor="base-currency">Base currency</Label>
    <Input id="base-currency" defaultValue="USD" disabled />
  </div>
)
