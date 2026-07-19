import { AspectRatio } from 'web'
import { TrendingUp, Wallet } from 'lucide-react'

export const SixteenByNine = () => (
  <div className="w-80">
    <AspectRatio ratio={16 / 9} className="rounded-md border bg-muted">
      <div className="flex h-full flex-col justify-between p-4">
        <div className="flex items-center gap-2 text-sm font-medium">
          <TrendingUp className="size-4 text-income" />
          Spending trend · June
        </div>
        <div className="flex items-end gap-2">
          <div className="w-8 rounded-t-sm bg-econumo-yellow" style={{ height: 28 }} />
          <div className="w-8 rounded-t-sm bg-econumo-yellow" style={{ height: 46 }} />
          <div className="w-8 rounded-t-sm bg-econumo-yellow" style={{ height: 34 }} />
          <div className="w-8 rounded-t-sm bg-econumo-yellow" style={{ height: 62 }} />
          <div className="w-8 rounded-t-sm bg-econumo-yellow" style={{ height: 50 }} />
        </div>
      </div>
    </AspectRatio>
    <p className="mt-2 text-sm text-muted-foreground">16 : 9 — report thumbnail</p>
  </div>
)

export const Square = () => (
  <div className="w-40">
    <AspectRatio ratio={1} className="rounded-md border bg-muted">
      <div className="flex h-full flex-col items-center justify-center gap-2">
        <Wallet className="size-8 text-muted-foreground" />
        <span className="text-sm font-medium">Cash</span>
        <span className="text-sm text-muted-foreground">$120.00</span>
      </div>
    </AspectRatio>
    <p className="mt-2 text-sm text-muted-foreground">1 : 1 — account tile</p>
  </div>
)
