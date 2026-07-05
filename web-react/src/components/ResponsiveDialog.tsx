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
  const handleOpenChange = (next: boolean) => {
    if (!next && !dismissible) {
      return
    }
    onOpenChange(next)
  }
  if (isMobile) {
    return (
      <Drawer open={open} onOpenChange={handleOpenChange} dismissible={dismissible}>
        <DrawerContent
          className={
            fullScreen
              ? 'data-[vaul-drawer-direction=bottom]:mt-0 data-[vaul-drawer-direction=bottom]:h-dvh data-[vaul-drawer-direction=bottom]:max-h-dvh data-[vaul-drawer-direction=bottom]:rounded-none'
              : undefined
          }
        >
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
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent
        onInteractOutside={dismissible ? undefined : (e) => e.preventDefault()}
        showCloseButton={dismissible}
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
