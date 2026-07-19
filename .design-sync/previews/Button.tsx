import { Button } from 'web'

export const Variants = () => (
  <div className="flex flex-wrap items-center gap-3">
    <Button>Add transaction</Button>
    <Button variant="secondary">Save changes</Button>
    <Button variant="outline">Cancel</Button>
    <Button variant="ghost">Skip</Button>
    <Button variant="destructive">Delete account</Button>
    <Button variant="link">View all transactions</Button>
  </div>
)

export const Sizes = () => (
  <div className="flex flex-wrap items-center gap-3">
    <Button size="xs">Filter</Button>
    <Button size="sm">Apply</Button>
    <Button size="default">Add transaction</Button>
    <Button size="lg">Get started</Button>
  </div>
)

export const States = () => (
  <div className="flex flex-wrap items-center gap-3">
    <Button disabled>Saving…</Button>
    <Button variant="outline" disabled>
      Cancel
    </Button>
  </div>
)
