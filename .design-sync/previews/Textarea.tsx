import { Label, Textarea } from 'web'

export const Default = () => (
  <div className="flex w-80 flex-col gap-2">
    <Label htmlFor="tx-description">Description</Label>
    <Textarea id="tx-description" placeholder="Add a note about this transaction" />
  </div>
)

export const Filled = () => (
  <div className="flex w-80 flex-col gap-2">
    <Label htmlFor="tx-note">Description</Label>
    <Textarea
      id="tx-note"
      className="min-h-16 resize-none"
      defaultValue="Weekly groceries at Whole Foods — split with roommate, $42.50 each"
    />
  </div>
)

export const States = () => (
  <div className="flex w-80 flex-col gap-4">
    <div className="flex flex-col gap-2">
      <Label htmlFor="tx-disabled">Description (read-only budget)</Label>
      <Textarea id="tx-disabled" disabled defaultValue="Salary — March payout" />
    </div>
    <div className="flex flex-col gap-2">
      <Label htmlFor="tx-invalid">Description</Label>
      <Textarea
        id="tx-invalid"
        aria-invalid="true"
        defaultValue="This note is way too long for the 512-character limit…"
      />
      <span className="text-sm text-expense">Description must be 512 characters or fewer</span>
    </div>
  </div>
)
