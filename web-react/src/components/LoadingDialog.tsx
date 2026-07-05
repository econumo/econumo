import type { CSSProperties } from 'react'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'

const COIN_STAGGER_S = 0.16

export function LoadingDialog({ open, label }: { open: boolean; label: string }) {
  return (
    <ResponsiveDialog open={open} onOpenChange={() => {}} title={label} dismissible={false} hideHeader>
      <div className="flex justify-center pt-2 pb-8" role="status" aria-label={label}>
        <div className="coin-loader" aria-hidden="true">
          {[0, 1, 2].map((i) => (
            <span key={i} className="coin-loader-unit" style={{ '--coin-delay': `${i * COIN_STAGGER_S}s` } as CSSProperties}>
              <span className="coin-loader-coin">e</span>
              <span className="coin-loader-shadow" />
            </span>
          ))}
        </div>
      </div>
    </ResponsiveDialog>
  )
}
