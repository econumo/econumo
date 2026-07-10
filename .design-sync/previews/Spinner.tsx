import { Button, Spinner } from 'web'

export const Sizes = () => (
  <div className="flex items-center gap-6">
    <Spinner className="size-4" />
    <Spinner className="size-6" />
    <Spinner className="size-8" />
  </div>
)

export const InButton = () => (
  <div className="flex items-center gap-3">
    <Button disabled>
      <Spinner />
      Saving…
    </Button>
    <Button variant="outline" disabled>
      <Spinner />
      Importing CSV…
    </Button>
  </div>
)

export const LoadingRow = () => (
  <div className="flex items-center gap-2 text-sm text-muted-foreground">
    <Spinner className="text-primary" />
    Loading transactions…
  </div>
)
