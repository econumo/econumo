import { Input, Label } from 'web'

export const WithLabel = () => (
  <div className="flex w-80 flex-col gap-2">
    <Label htmlFor="account-name">Account name</Label>
    <Input id="account-name" placeholder="Main account" />
  </div>
)

export const Types = () => (
  <div className="flex w-80 flex-col gap-3">
    <Input type="email" placeholder="you@example.com" aria-label="Email" />
    <Input type="password" defaultValue="correct-horse" aria-label="Password" />
    <Input type="number" defaultValue="385.20" step="0.01" aria-label="Amount" />
    <Input type="date" defaultValue="2026-07-09" aria-label="Transaction date" />
  </div>
)

export const Invalid = () => (
  <div className="flex w-80 flex-col gap-2">
    <Label htmlFor="category-name">Category name</Label>
    <Input id="category-name" defaultValue="Gr" aria-invalid="true" />
    <p className="text-sm text-destructive">Category name must be 3-64 characters</p>
  </div>
)

export const Disabled = () => (
  <div className="flex w-80 flex-col gap-2">
    <Label htmlFor="base-currency">Base currency</Label>
    <Input id="base-currency" defaultValue="USD" disabled />
  </div>
)
