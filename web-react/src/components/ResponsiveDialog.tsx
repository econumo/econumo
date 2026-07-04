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
}

export function ResponsiveDialog({ open, onOpenChange, title, description, children, dismissible = true, caps = false }: ResponsiveDialogProps) {
  const isMobile = useIsMobile()
  const titleClass = caps ? 'uppercase tracking-wide' : undefined
  const handleOpenChange = (next: boolean) => {
    if (!next && !dismissible) {
      return
    }
    onOpenChange(next)
  }
  if (isMobile) {
    return (
      <Drawer open={open} onOpenChange={handleOpenChange} dismissible={dismissible}>
        <DrawerContent>
          <DrawerHeader>
            <DrawerTitle className={titleClass}>{title}</DrawerTitle>
            {description ? <DrawerDescription>{description}</DrawerDescription> : null}
          </DrawerHeader>
          <div className="overflow-y-auto px-4 pb-4">{children}</div>
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
        <DialogHeader>
          <DialogTitle className={titleClass}>{title}</DialogTitle>
          {description ? <DialogDescription>{description}</DialogDescription> : null}
        </DialogHeader>
        {children}
      </DialogContent>
    </Dialog>
  )
}
