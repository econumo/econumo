import { useRef } from 'react'
import type { ReactNode } from 'react'
import { CardField } from '@/components/CardField'

// strips the EntitySelect chrome so the CardField alone carries it. The selector
// must match EntitySelect's own data-slot — targeting a bare button (or a
// role="combobox" trigger) silently styles nothing, since EntitySelect renders
// an input inside a slotted div.
const cardSelectClass =
  '[&_[data-slot=entity-select]]:h-auto [&_[data-slot=entity-select]]:border-0 [&_[data-slot=entity-select]]:px-0 [&_[data-slot=entity-select]]:ring-0 [&_[data-slot=entity-select]]:bg-transparent dark:[&_[data-slot=entity-select]]:bg-transparent'

/**
 * CardField around an EntitySelect where the WHOLE card is the tap target —
 * clicks on the label/padding forward to the field inside. Shared by the
 * transaction and recurring-template dialogs so their pickers look identical.
 */
export function SelectCard({ label, error, children }: { label: string; error?: string | null; children: ReactNode }) {
  // if the picker was open at pointerdown, that press already dismissed it —
  // forwarding the click would immediately reopen
  const wasOpen = useRef(false)
  const trigger = (root: HTMLElement) => root.querySelector<HTMLInputElement>('[data-slot=entity-select] input')
  return (
    <div
      className="cursor-pointer"
      onPointerDownCapture={(e) => {
        wasOpen.current = trigger(e.currentTarget)?.getAttribute('aria-expanded') === 'true'
      }}
      onClick={(e) => {
        if (wasOpen.current || (e.target as HTMLElement).closest('input, button')) {
          return
        }
        trigger(e.currentTarget)?.click()
      }}
    >
      <CardField label={label} error={error}>
        <div className={cardSelectClass}>{children}</div>
      </CardField>
    </div>
  )
}
