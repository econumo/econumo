import { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react'
import type { FocusEvent, ReactNode, RefObject } from 'react'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Drawer, DrawerContent, DrawerDescription, DrawerHeader, DrawerTitle } from '@/components/ui/drawer'
import { useIsPhone } from '@/hooks/useIsPhone'
import { useKeyboardInset } from '@/hooks/useKeyboardInset'

const STACKED_CONTENT = '[data-slot="dialog-content"], [data-slot="drawer-content"], [data-slot="alert-dialog-content"]'
const STACKED_OVERLAY = '[data-slot="dialog-overlay"], [data-slot="drawer-overlay"], [data-slot="alert-dialog-overlay"]'

// px kept free above a sheet squeezed by the on-screen keyboard
const SHEET_TOP_GAP = 24

// a full-screen page counts the sheets open over it (a picker's), and holds
// still while any is: the keyboard those sheets raise is theirs to follow
// px left between a field brought into view and the form's edge
const FIELD_REVEAL_GAP = 8
const FIELD_REVEAL_DELAY_MS = 100

const CoverPage = createContext<((delta: 1 | -1) => void) | null>(null)

/** the standard dialog action row (Cancel | Confirm). Action buttons are h-11
    (the Add-transaction bar height) on EVERY screen size — desktop is not a
    smaller-target platform, heights stay identical to mobile by design */
export const dialogActionsClass = 'grid grid-cols-2 gap-3 [&_button]:h-11'

interface ResponsiveDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  /** shown before the title, e.g. the entity's icon */
  titleIcon?: ReactNode
  /** a compact icon button at the header's end (beside the desktop corner X) */
  headerAction?: ReactNode
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
  /** desktop width: `wide` suits list-heavy dialogs (the global search palette) */
  size?: 'default' | 'wide'
  /** mobile sheet: the field that takes focus on open (the drawer focuses itself otherwise) */
  initialFocus?: RefObject<HTMLElement | null>
  /** runs as the dialog hands focus back on close; preventDefault() keeps focus
   *  where an action started from the dialog put it */
  onCloseAutoFocus?: (e: Event) => void
}

export function ResponsiveDialog({ open, onOpenChange, title, titleIcon, headerAction, description, children, dismissible = true, caps = false, hideHeader = false, showClose = false, fullScreen = false, footer, onCloseAutoFocus, initialFocus, size = 'default' }: ResponsiveDialogProps) {
  // full-screen / bottom-sheet layouts kick in only below sm (640px); at 640px
  // and up dialogs stay centred like on desktop
  const isMobile = useIsPhone()
  const titleClass = caps ? 'uppercase tracking-wide' : undefined
  const showCloseButton = dismissible && (!hideHeader || showClose)
  // keep the (possibly long, title-less-confirm) heading clear of the corner X
  const headerClass = hideHeader ? 'sr-only' : headerAction ? (showCloseButton ? 'pr-16' : 'pr-8') : showCloseButton ? 'pr-8' : undefined
  const titleContent = titleIcon ? (
    <>
      {titleIcon}
      <span className="min-w-0">{title}</span>
    </>
  ) : (
    title
  )
  const titleRowClass = titleIcon ? 'flex items-center gap-1.5' : ''
  // sits left of the corner X (size icon-sm, at right-2) or takes its place
  const cornerActionClass = showCloseButton ? 'right-10' : 'right-2'
  const contentRef = useRef<HTMLDivElement>(null)
  // the sheet sits on the keyboard's top edge and never grows past the visible
  // area; vaul's own repositionInputs is off because it froze a stale height
  const liveKeyboard = useKeyboardInset(open && isMobile)
  const [covered, setCovered] = useState(0)
  // a closed sheet's keyboard is still going away: until it is down, or a
  // field of the page takes focus, its geometry is not the page's
  const [sheetKeyboard, setSheetKeyboard] = useState(false)
  // held only while a sheet's own keyboard is up (it may pan the screen for
  // its search); with no keyboard the full-height page is already right, so
  // it follows, and a sheet closing finds nothing left to change
  const held = (covered > 0 || sheetKeyboard) && liveKeyboard.height > 0
  const heldKeyboard = useRef(liveKeyboard)
  useEffect(() => {
    if (covered === 0 && !sheetKeyboard) {
      heldKeyboard.current = liveKeyboard
    }
  }, [covered, sheetKeyboard, liveKeyboard])
  // also when the sheet closed with its keyboard already down: the viewport
  // then never changes again to clear it
  useEffect(() => {
    if (sheetKeyboard && liveKeyboard.height === 0) {
      setSheetKeyboard(false)
    }
  }, [sheetKeyboard, liveKeyboard])
  const keyboard = held ? heldKeyboard.current : liveKeyboard
  const cover = useCallback((delta: 1 | -1) => {
    setCovered((n) => n + delta)
    if (delta < 0) {
      setSheetKeyboard(true)
    }
  }, [])
  // React focus events also bubble up from the sheets portaled out of the page
  const onPageFocus = (e: FocusEvent) => {
    if (contentRef.current?.contains(e.target as Node)) {
      setSheetKeyboard(false)
    }
  }
  // a field tapped on the full-height page was in view, so iOS revealed
  // nothing; the page then shrinks above the keyboard and can leave it under
  // the footer. Bring it back into the form's visible part
  const fitted = isMobile && fullScreen && keyboard.height > 0
  const bodyRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const body = bodyRef.current
    const field = document.activeElement
    if (!fitted || !body || !(field instanceof HTMLElement) || !body.contains(field)) {
      return
    }
    // once the fitted layout has settled: on the frame of the change the form
    // still measured at its old height
    const timer = setTimeout(() => {
      const box = body.getBoundingClientRect()
      const rect = field.getBoundingClientRect()
      if (rect.bottom > box.bottom) {
        body.scrollTop += rect.bottom - box.bottom + FIELD_REVEAL_GAP
      } else if (rect.top < box.top) {
        body.scrollTop -= box.top - rect.top + FIELD_REVEAL_GAP
      }
    }, FIELD_REVEAL_DELAY_MS)
    return () => clearTimeout(timer)
  }, [fitted, keyboard.height, keyboard.top])
  const coverParent = useContext(CoverPage)
  useEffect(() => {
    if (!open || !coverParent) {
      return
    }
    coverParent(1)
    return () => coverParent(-1)
  }, [open, coverParent])
  const sheetStyle = keyboard.height > 0 ? { bottom: keyboard.bottom, maxHeight: keyboard.height - SHEET_TOP_GAP } : undefined
  // a full-screen page shrinks to the visible area instead: left at h-dvh, iOS
  // pans it so the header and the fields above the focused one go off-screen
  // and the rest hides behind the keyboard; fitted, the header and footer stay
  // put and the form scrolls between them
  const pageStyle = {
    // Radix turns off pointer events on <body> as the page opens and only turns
    // them back on for the page a render later; iOS works out where its ^ / v
    // keys go when the autofocused field takes focus, and in that gap every
    // other field counts as covered. A stacked sheet brings its own overlay
    pointerEvents: 'auto' as const,
    ...(keyboard.height > 0 ? { top: keyboard.top, height: keyboard.height, maxHeight: keyboard.height } : {}),
  }
  // edge-to-edge viewport: content that reaches the screen bottom must clear
  // the home indicator itself; a pinned footer carries its own inset instead
  // the keyboard covers the home indicator, so its inset is dead space then
  const keyboardUp = keyboard.height > 0
  const bodyClass = `flex-1 overflow-y-auto px-4 pt-1 ${footer || keyboardUp ? 'pb-4' : 'pb-[max(env(safe-area-inset-bottom),1rem)]'}`
  const footerClass = `border-t px-4 pt-3 ${keyboardUp ? 'pb-3' : 'pb-[max(env(safe-area-inset-bottom),0.75rem)]'} [&_button]:h-11`

  // An open combobox popup (Base UI, not a Radix layer, so Radix's document-
  // capture Escape handler doesn't know about it) owns Escape: it closes the
  // picker, and the dialog must stay put
  const onEscapeKeyDown = (e: KeyboardEvent) => {
    if (document.querySelector('[data-slot="combobox-content"]')) {
      e.preventDefault()
    }
  }
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
        {/* the ::after carries the page's background below its bottom edge, so a
            keyboard sliding away under a fitted page never shows the screen behind */}
        <DialogContent
          ref={contentRef}
          style={pageStyle}
          onFocusCapture={onPageFocus}
          className="top-0 left-0 flex h-dvh max-h-dvh w-screen after:absolute after:inset-x-0 after:top-full after:h-dvh after:bg-background after:content-[''] max-w-none translate-x-0 translate-y-0 flex-col gap-0 rounded-none p-0 ring-0 data-open:zoom-in-100 data-closed:zoom-out-100 [&_[data-slot=dialog-close]]:top-[max(env(safe-area-inset-top),0.5rem)]"
          onInteractOutside={onInteractOutside}
          onEscapeKeyDown={onEscapeKeyDown}
          onCloseAutoFocus={onCloseAutoFocus}
          showCloseButton={showCloseButton}
        >
          {/* the full-viewport page sits under the status bar — keep the header (and the corner X above) clear of it */}
          <DialogHeader className={`${headerClass ?? ''} px-4 pt-[max(env(safe-area-inset-top),1rem)]`}>
            <DialogTitle className={`${titleClass ?? ''} ${titleRowClass}`}>{titleContent}</DialogTitle>
            {description ? <DialogDescription>{description}</DialogDescription> : null}
          </DialogHeader>
          {headerAction && !hideHeader ? <div className={`absolute top-[max(env(safe-area-inset-top),0.5rem)] ${cornerActionClass}`}>{headerAction}</div> : null}
          {/* pt-1 keeps the first field's focus ring from being clipped by the scroll container */}
          <CoverPage.Provider value={cover}>
            {/* above the header and footer in stacking order, though clipped to
                its own box on screen: iOS's ^ / v keys skip a field whose centre
                another element covers, and a field scrolled under the footer
                would otherwise be unreachable */}
            <div ref={bodyRef} data-slot="dialog-body" className={`${bodyClass} relative z-10`}>{children}</div>
            {footer ? (
              <div className={footerClass}>{footer}</div>
            ) : null}
          </CoverPage.Provider>
        </DialogContent>
      </Dialog>
    )
  }
  if (isMobile) {
    return (
      <Drawer open={open} onOpenChange={onOpenChange} dismissible={dismissible} repositionInputs={false}>
        <DrawerContent
          ref={contentRef}
          style={sheetStyle}
          onInteractOutside={onInteractOutside}
          onEscapeKeyDown={onEscapeKeyDown}
          onCloseAutoFocus={onCloseAutoFocus}
          onOpenAutoFocus={(e) => {
            const field = initialFocus?.current
            if (field) {
              e.preventDefault()
              // iOS scrolls the whole screen to reveal a field it sees near the
              // bottom (the sheet rests on the keyboard); placed far above for
              // the focus, it has nothing to reveal (vaul does the same on tap)
              field.style.transform = 'translateY(-2000px)'
              field.focus()
              requestAnimationFrame(() => {
                field.style.transform = ''
              })
            }
          }}
        >
          {/* the drawer has no corner X, so it never needs the pr-8 clearance */}
          {/* a header action is pinned to the end; the matching start inset keeps the title centred */}
          <DrawerHeader className={hideHeader ? 'sr-only' : headerAction ? 'relative px-12' : undefined}>
            <DrawerTitle className={`${titleClass ?? ''} ${titleIcon ? `${titleRowClass} justify-center` : ''}`}>{titleContent}</DrawerTitle>
            {description ? <DrawerDescription>{description}</DrawerDescription> : null}
            {headerAction && !hideHeader ? <div className="absolute top-1/2 right-3 -translate-y-1/2">{headerAction}</div> : null}
          </DrawerHeader>
          {/* pt-1 keeps the first field's focus ring from being clipped by the scroll container */}
          <div className={bodyClass}>{children}</div>
          {footer ? (
            <div className={footerClass}>{footer}</div>
          ) : null}
        </DrawerContent>
      </Drawer>
    )
  }
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        ref={contentRef}
        // one column that may shrink below its content's width: a long line (a
        // list of account names) truncates inside the dialog instead of widening
        // every field past its edge
        className={`grid-cols-[minmax(0,1fr)] ${size === 'wide' ? 'sm:max-w-2xl' : ''}`}
        onInteractOutside={onInteractOutside}
        onEscapeKeyDown={onEscapeKeyDown}
        onCloseAutoFocus={onCloseAutoFocus}
        // a floating X with no header row to anchor it looks stray — unless asked for
        showCloseButton={showCloseButton}
      >
        <DialogHeader className={headerClass}>
          <DialogTitle className={`${titleClass ?? ''} ${titleRowClass}`}>{titleContent}</DialogTitle>
          {description ? <DialogDescription>{description}</DialogDescription> : null}
        </DialogHeader>
        {headerAction && !hideHeader ? <div className={`absolute top-2 ${cornerActionClass}`}>{headerAction}</div> : null}
        {children}
        {footer}
      </DialogContent>
    </Dialog>
  )
}
