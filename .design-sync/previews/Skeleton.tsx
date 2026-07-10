import { Skeleton } from 'web'

export const TransactionRows = () => (
  <div className="flex w-80 flex-col gap-4">
    {[0, 1, 2].map((i) => (
      <div key={i} className="flex items-center gap-3">
        <Skeleton className="size-9 rounded-full" />
        <div className="flex flex-1 flex-col gap-1.5">
          <Skeleton className="h-4 w-32" />
          <Skeleton className="h-3 w-20" />
        </div>
        <Skeleton className="h-4 w-16" />
      </div>
    ))}
  </div>
)

export const AccountCard = () => (
  <div className="flex w-80 flex-col gap-3 rounded-lg border p-4">
    <Skeleton className="h-4 w-28" />
    <Skeleton className="h-8 w-40" />
    <Skeleton className="h-3 w-48" />
  </div>
)

export const TextBlock = () => (
  <div className="flex w-80 flex-col gap-2">
    <Skeleton className="h-5 w-3/5" />
    <Skeleton className="h-4 w-full" />
    <Skeleton className="h-4 w-4/5" />
  </div>
)
