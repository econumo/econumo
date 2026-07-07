import { useRef } from 'react'
import type { ReactNode } from 'react'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Drawer, DrawerContent, DrawerDescription, DrawerHeader, DrawerTitle } from '@/components/ui/drawer'
import { useIsMobile } from '@/hooks/useIsMobile'

const STACKED_CONTENT = '[data-slot="dialog-content"], [data-slot="drawer-content"], [data-slot="alert-dialog-content"]'
const STACKED_OVERLAY = '[data-slot="dialog-overlay"], [data-slot="drawer-overlay"], [data-slot="alert-dialog-overlay"]'

/** the standard dialog action row (Cancel | Confirm). Action buttons are h-11
    (the Add-transaction bar height) on EVERY screen size — desktop is not a
    smaller-target platform, heights stay identical to mobile by design */
export const dialogActionsClass = 'grid grid-cols-2 gap-3 [&_button]:h-11'

interface ResponsiveDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description?: string
  children: ReactNode
  dismissible?: boolean
  /** Quasar-parity headers (ADD TRANSACTION, …); confirmation questions stay sentence case */
  caps?: boolean
  /** visually hide the header (title stays for screen readers) — pure-indicator dialogs */
  hideHeader?: boolean
  /** desktop: keep the corner X even with hideHeader (hero-style dialogs) */
  showClose?: boolean
  /** mobile only: stretch the sheet to the full viewport (long forms; avoids a scrolling half-sheet) */
  fullScreen?: boolean
  /** action row rendered outside the scroll area — pinned to the sheet bottom on mobile */
  footer?: ReactNode
}

export function ResponsiveDialog({ open, onOpenChange, title, description, children, dismissible = true, caps = false, hideHeader = false, showClose = false, fullScreen = false, footer }: ResponsiveDialogProps) {
  const isMobile = useIsMobile()
  const titleClass = caps ? 'uppercase tracking-wide' : undefined
  const showCloseButton = dismissible && (!hideHeader || showClose)
  // keep the (possibly long, title-less-confirm) heading clear of the corner X
  const headerClass = hideHeader ? 'sr-only' : showCloseButton ? 'pr-8' : undefined
  const contentRef = useRef<HTMLDivElement>(null)
  // edge-to-edge viewport: content that reaches the screen bottom must clear
  // the home indicator itself; a pinned footer carries its own inset instead
  const bodyClass = `flex-1 overflow-y-auto px-4 pt-1 ${footer ? 'pb-4' : 'pb-[max(env(safe-area-inset-bottom),1rem)]'}`

  // An interaction that BEGAN inside a dialog stacked on top of this one must
  // never dismiss this one. Radix defers its outside-check to the click phase,
  // by which time the upper dialog may have closed and unmounted — a detached
  // target therefore also counts as stacked. Own-overlay clicks still dismiss.
  const onInteractOutside = (e: { preventDefault: () => void; target: EventTarget | null; detail?: { originalEvent?: Event } }) => {
    if (!dismissible) {
      e.preventDefault()
      return
    }
    const target = e.detail?.originalEvent?.target ?? e.target
    if (!(target instanceof Element)) {
      return
    }
    if (target.closest(STACKED_CONTENT)) {
      e.preventDefault()
      return
    }
    const overlay = target.closest(STACKED_OVERLAY)
    const own = contentRef.current
    if (overlay && (!overlay.isConnected || (own && (own.compareDocumentPosition(overlay) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0))) {
      e.preventDefault()
    }
  }
  // dismissible=false shields against ACCIDENTAL dismissal (outside click /
  // swipe) — those are prevented at the source (onInteractOutside, vaul's
  // dismissible), so any close reaching here is deliberate (Escape) and passes
  if (isMobile && fullScreen) {
    // full-viewport forms read as a PAGE, not a sheet — a plain fade beats the
    // whole screen sliding up from the bottom
    return (
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent
          ref={contentRef}
          className="top-0 left-0 flex h-dvh max-h-dvh w-screen max-w-none translate-x-0 translate-y-0 flex-col gap-0 rounded-none p-0 ring-0 data-open:zoom-in-100 data-closed:zoom-out-100 [&_[data-slot=dialog-close]]:top-[max(env(safe-area-inset-top),0.5rem)]"
          onInteractOutside={onInteractOutside}
          showCloseButton={showCloseButton}
        >
          {/* the full-viewport page sits under the status bar — keep the header (and the corner X above) clear of it */}
          <DialogHeader className={`${headerClass ?? ''} px-4 pt-[max(env(safe-area-inset-top),1rem)]`}>
            <DialogTitle className={titleClass}>{title}</DialogTitle>
            {description ? <DialogDescription>{description}</DialogDescription> : null}
          </DialogHeader>
          {/* pt-1 keeps the first field's focus ring from being clipped by the scroll container */}
          <div className={bodyClass}>{children}</div>
          {footer ? (
            <div className="border-t px-4 pt-3 pb-[max(env(safe-area-inset-bottom),0.75rem)] [&_button]:h-11">{footer}</div>
          ) : null}
        </DialogContent>
      </Dialog>
    )
  }
  if (isMobile) {
    return (
      <Drawer open={open} onOpenChange={onOpenChange} dismissible={dismissible}>
        <DrawerContent ref={contentRef} onInteractOutside={onInteractOutside}>
          {/* the drawer has no corner X, so it never needs the pr-8 clearance */}
          <DrawerHeader className={hideHeader ? 'sr-only' : undefined}>
            <DrawerTitle className={titleClass}>{title}</DrawerTitle>
            {description ? <DrawerDescription>{description}</DrawerDescription> : null}
          </DrawerHeader>
          {/* pt-1 keeps the first field's focus ring from being clipped by the scroll container */}
          <div className={bodyClass}>{children}</div>
          {footer ? (
            <div className="border-t px-4 pt-3 pb-[max(env(safe-area-inset-bottom),0.75rem)] [&_button]:h-11">{footer}</div>
          ) : null}
        </DrawerContent>
      </Drawer>
    )
  }
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        ref={contentRef}
        onInteractOutside={onInteractOutside}
        // a floating X with no header row to anchor it looks stray — unless asked for
        showCloseButton={showCloseButton}
      >
        <DialogHeader className={headerClass}>
          <DialogTitle className={titleClass}>{title}</DialogTitle>
          {description ? <DialogDescription>{description}</DialogDescription> : null}
        </DialogHeader>
        {children}
        {footer}
      </DialogContent>
    </Dialog>
  )
}
