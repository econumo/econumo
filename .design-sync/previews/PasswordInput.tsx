import { Label, PasswordInput } from 'web'

export const Default = () => (
  <div className="w-80">
    <PasswordInput defaultValue="correct-horse-battery" />
  </div>
)

export const Empty = () => (
  <div className="w-80">
    <PasswordInput placeholder="Enter your password" />
  </div>
)

export const WithLabel = () => (
  <div className="flex w-80 flex-col gap-2">
    <Label htmlFor="password-input-preview">Password</Label>
    <PasswordInput id="password-input-preview" defaultValue="hunter2hunter2" />
  </div>
)

export const Disabled = () => (
  <div className="w-80">
    <PasswordInput defaultValue="not-editable" disabled />
  </div>
)
