import { Loader2 } from 'lucide-react'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'

export function LoadingDialog({ open, label }: { open: boolean; label: string }) {
  return (
    <ResponsiveDialog open={open} onOpenChange={() => {}} title={label} dismissible={false}>
      <div className="flex justify-center py-4">
        <Loader2 className="size-8 animate-spin text-muted-foreground" aria-label={label} />
      </div>
    </ResponsiveDialog>
  )
}
