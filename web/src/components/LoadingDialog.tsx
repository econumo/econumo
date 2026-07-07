import { CoinLoader } from '@/components/CoinLoader'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'

export function LoadingDialog({ open, label }: { open: boolean; label: string }) {
  return (
    <ResponsiveDialog open={open} onOpenChange={() => {}} title={label} dismissible={false} hideHeader>
      <div className="pt-2 pb-8">
        <CoinLoader label={label} />
      </div>
    </ResponsiveDialog>
  )
}
