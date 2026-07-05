import type { ReactNode } from 'react'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Drawer, DrawerContent, DrawerDescription, DrawerHeader, DrawerTitle } from '@/components/ui/drawer'
import { useIsMobile } from '@/hooks/useIsMobile'

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
  /** mobile only: stretch the sheet to the full viewport (long forms; avoids a scrolling half-sheet) */
  fullScreen?: boolean
  /** action row rendered outside the scroll area — pinned to the sheet bottom on mobile */
  footer?: ReactNode
}

export function ResponsiveDialog({ open, onOpenChange, title, description, children, dismissible = true, caps = false, hideHeader = false, fullScreen = false, footer }: ResponsiveDialogProps) {
  const isMobile = useIsMobile()
  const titleClass = caps ? 'uppercase tracking-wide' : undefined
  const headerClass = hideHeader ? 'sr-only' : undefined
  // dismissible=false shields against ACCIDENTAL dismissal (outside click /
  // swipe) — those are prevented at the source (onInteractOutside, vaul's
  // dismissible), so any close reaching here is deliberate (Escape) and passes
  if (isMobile && fullScreen) {
    // full-viewport forms read as a PAGE, not a sheet — a plain fade beats the
    // whole screen sliding up from the bottom
    return (
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent
          className="top-0 left-0 flex h-dvh max-h-dvh w-screen max-w-none translate-x-0 translate-y-0 flex-col gap-0 rounded-none p-0 ring-0 data-open:zoom-in-100 data-closed:zoom-out-100"
          onInteractOutside={dismissible ? undefined : (e) => e.preventDefault()}
          showCloseButton={dismissible && !hideHeader}
        >
          <DialogHeader className={`${headerClass ?? ''} px-4 pt-4`}>
            <DialogTitle className={titleClass}>{title}</DialogTitle>
            {description ? <DialogDescription>{description}</DialogDescription> : null}
          </DialogHeader>
          {/* pt-1 keeps the first field's focus ring from being clipped by the scroll container */}
          <div className="flex-1 overflow-y-auto px-4 pt-1 pb-4">{children}</div>
          {footer ? (
            <div className="border-t px-4 pt-3 pb-[max(env(safe-area-inset-bottom),0.75rem)]">{footer}</div>
          ) : null}
        </DialogContent>
      </Dialog>
    )
  }
  if (isMobile) {
    return (
      <Drawer open={open} onOpenChange={onOpenChange} dismissible={dismissible}>
        <DrawerContent>
          <DrawerHeader className={headerClass}>
            <DrawerTitle className={titleClass}>{title}</DrawerTitle>
            {description ? <DrawerDescription>{description}</DrawerDescription> : null}
          </DrawerHeader>
          {/* pt-1 keeps the first field's focus ring from being clipped by the scroll container */}
          <div className="flex-1 overflow-y-auto px-4 pt-1 pb-4">{children}</div>
          {footer ? (
            <div className="border-t px-4 pt-3 pb-[max(env(safe-area-inset-bottom),0.75rem)]">{footer}</div>
          ) : null}
        </DrawerContent>
      </Drawer>
    )
  }
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        onInteractOutside={dismissible ? undefined : (e) => e.preventDefault()}
        // a floating X with no header row to anchor it looks stray
        showCloseButton={dismissible && !hideHeader}
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
