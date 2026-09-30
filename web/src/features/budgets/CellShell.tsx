import { cloneElement, useCallback, useEffect, useRef, useState } from 'react'
import type {
  KeyboardEvent as ReactKeyboardEvent,
  MouseEvent as ReactMouseEvent,
  PointerEvent as ReactPointerEvent,
  ReactElement,
  Ref,
  RefObject,
} from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { Popover, PopoverAnchor, PopoverContent } from '@/components/ui/popover'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Button } from '@/components/ui/button'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { useIsCompact } from '@/hooks/useIsCompact'
import type { BudgetCommentDto } from '@/api/dto/budget'
import { commentAnchorOf } from './cellDom'
import { formatCommentTime, sortByCreatedAt } from './CommentThread'

type PointerHandler = (e: ReactPointerEvent<HTMLElement>) => void

interface CellProps {
  ref?: Ref<HTMLElement>
  onPointerDown?: PointerHandler
  onPointerMove?: PointerHandler
  onPointerUp?: PointerHandler
  onPointerCancel?: PointerHandler
  onPointerEnter?: PointerHandler
  onPointerLeave?: PointerHandler
  onClickCapture?: (e: ReactMouseEvent<HTMLElement>) => void
  onContextMenu?: (e: ReactMouseEvent<HTMLElement>) => void
  onKeyDown?: (e: ReactKeyboardEvent<HTMLElement>) => void
}

interface CellShellProps {
  /** the cell element: a single DOM element; it receives the shell's handlers and ref directly, with no wrapper */
  children: ReactElement
  /** heading of the touch actions modal: the element's display name */
  title: string
  comments: BudgetCommentDto[]
  /** no hover preview while a thread is open (touch viewports never preview) */
  previewDisabled?: boolean
  /** no menu / actions modal / Shift+F2: phones (stage 2 gives them the item sheet) and edit-structure mode */
  menuDisabled?: boolean
  onSetBudget?: (anchor: HTMLElement) => void
  /** omitted for the uncategorized row */
  onOpenComments?: (anchor: HTMLElement) => void
  onShowTransactions?: () => void
}

type ActionRun = (anchor: HTMLElement) => void

interface CellAction {
  key: string
  label: string
  run: ActionRun
}

const PREVIEW_COUNT = 2
const PREVIEW_OPEN_MS = 300
const PREVIEW_CLOSE_MS = 100
const LONG_PRESS_MS = 500
// a finger drifting further than this is a scroll, not a press
const LONG_PRESS_SLOP_PX = 10
const OPEN_LIMIT_EDITOR = '[data-limit-trigger][data-state="open"]'

function assignRef<T>(ref: Ref<T> | undefined, value: T | null) {
  if (typeof ref === 'function') {
    ref(value)
  } else if (ref) {
    ref.current = value
  }
}

// A grid re-renders every cell on each selection move, so the shell stays a bare
// set of handlers on the cell itself: the Radix preview, menu and modal (and
// their translated labels) mount as siblings only while open. Wrapping the cell
// in their triggers instead cost several times the grid's own render and one
// document listener per cell.
export function CellShell({
  children,
  title,
  comments,
  previewDisabled = false,
  menuDisabled = false,
  onSetBudget,
  onOpenComments,
  onShowTransactions,
}: CellShellProps) {
  const isTouch = useIsCompact()
  const cellRef = useRef<HTMLElement | null>(null)
  const [previewOpen, setPreviewOpen] = useState(false)
  // the desktop menu's pointer position; kept after close until the menu has handed focus back
  const [menu, setMenu] = useState<{ x: number; y: number; open: boolean } | null>(null)
  const [actionsModal, setActionsModal] = useState<'open' | 'closing' | null>(null)
  // a press means the user is acting on the cell (amount editor, selection): the
  // preview stays shut until the pointer leaves, or its pending open timer would
  // pop it over the editor the press just opened
  const pressed = useRef(false)
  const previewTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  // The menu/modal runs its action only after it has closed and handed focus back,
  // or the returning focus would steal it from the popover/dialog the action opens.
  const pending = useRef<ActionRun | null>(null)
  // what had focus when the menu opened; a menu dismissed without an action returns it there
  const menuOpener = useRef<Element | null>(null)
  const pressTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const pressStart = useRef<{ x: number; y: number } | null>(null)
  // the finger's release after a long-press fires a click on the cell underneath;
  // swallow that one click so the tap action (set-limit dialog, selection) does not
  // run under the modal. Reset by the next press, so a browser that never sends
  // that click cannot make a later, genuine tap disappear.
  const swallowClick = useRef(false)

  const child = children as ReactElement<CellProps>
  const childProps = child.props
  const childRef = childProps.ref
  const setCellRef = useCallback(
    (node: HTMLElement | null) => {
      cellRef.current = node
      assignRef(childRef, node)
    },
    [childRef],
  )

  // Unmounting mid-press or mid-hover (the row's data reloads, the column scrolls
  // out) must not fire the modal or the preview after the fact.
  useEffect(
    () => () => {
      if (pressTimer.current) {
        clearTimeout(pressTimer.current)
      }
      if (previewTimer.current) {
        clearTimeout(previewTimer.current)
      }
    },
    [],
  )

  const hasActions = !menuDisabled && !!(onSetBudget || onOpenComments || onShowTransactions)
  const previewable = !isTouch && !previewDisabled && comments.length > 0
  const touchActions = isTouch && hasActions
  const desktopMenu = !isTouch && hasActions

  const buildActions = (t: TFunction): CellAction[] => {
    const list: CellAction[] = []
    if (onSetBudget) {
      list.push({ key: 'set-budget', label: t('budgets.modal.set_limit_form.header'), run: onSetBudget })
    }
    if (onOpenComments) {
      list.push({
        key: 'comments',
        label: comments.length > 0 ? t('budgets.page.plan.comments.disclosure', { count: comments.length }) : t('budgets.page.plan.comments.add'),
        run: onOpenComments,
      })
    }
    if (onShowTransactions) {
      list.push({ key: 'transactions', label: t('budgets.page.budget.structure.element.action.show_transactions'), run: () => onShowTransactions() })
    }
    return list
  }

  const clearPreviewTimer = () => {
    if (previewTimer.current) {
      clearTimeout(previewTimer.current)
      previewTimer.current = null
    }
  }
  const schedulePreview = (open: boolean) => {
    clearPreviewTimer()
    previewTimer.current = setTimeout(
      () => {
        previewTimer.current = null
        // an amount editor opened from this cell owns the space the card would cover
        const editorOpen = !!cellRef.current?.querySelector(OPEN_LIMIT_EDITOR)
        setPreviewOpen(open && !pressed.current && !editorOpen)
      },
      open ? PREVIEW_OPEN_MS : PREVIEW_CLOSE_MS,
    )
  }
  const closePreview = () => {
    clearPreviewTimer()
    setPreviewOpen(false)
  }

  const runPending = (e: Event) => {
    const action = pending.current
    pending.current = null
    if (action && cellRef.current) {
      e.preventDefault()
      action(commentAnchorOf(cellRef.current))
    }
  }
  const onMenuCloseAutoFocus = (e: Event) => {
    const opener = menuOpener.current
    menuOpener.current = null
    setMenu(null)
    runPending(e)
    if (e.defaultPrevented) {
      return
    }
    // Radix would focus the menu's trigger, an invisible stand-in at the pointer;
    // a dismissed menu hands focus back to whatever had it before instead
    e.preventDefault()
    if (opener instanceof HTMLElement && opener !== document.body && opener.isConnected) {
      opener.focus()
    }
  }
  const cancelLongPress = () => {
    if (pressTimer.current) {
      clearTimeout(pressTimer.current)
      pressTimer.current = null
    }
    pressStart.current = null
  }

  const cell = cloneElement(child, {
    ref: setCellRef,
    onPointerEnter: (e: ReactPointerEvent<HTMLElement>) => {
      childProps.onPointerEnter?.(e)
      // mouse only: touch has no hover, and keyboard focus never previews
      if (e.pointerType === 'mouse' && previewable) {
        schedulePreview(true)
      }
    },
    onPointerLeave: (e: ReactPointerEvent<HTMLElement>) => {
      childProps.onPointerLeave?.(e)
      pressed.current = false
      cancelLongPress()
      if (previewOpen || previewTimer.current) {
        schedulePreview(false)
      }
    },
    onPointerDown: (e: ReactPointerEvent<HTMLElement>) => {
      childProps.onPointerDown?.(e)
      pressed.current = true
      swallowClick.current = false
      closePreview()
      if (touchActions && e.pointerType !== 'mouse') {
        cancelLongPress()
        pressStart.current = { x: e.clientX, y: e.clientY }
        pressTimer.current = setTimeout(() => {
          pressTimer.current = null
          swallowClick.current = true
          setActionsModal('open')
        }, LONG_PRESS_MS)
      }
    },
    onPointerMove: (e: ReactPointerEvent<HTMLElement>) => {
      childProps.onPointerMove?.(e)
      const start = pressStart.current
      if (start && Math.hypot(e.clientX - start.x, e.clientY - start.y) > LONG_PRESS_SLOP_PX) {
        cancelLongPress()
      }
    },
    onPointerUp: (e: ReactPointerEvent<HTMLElement>) => {
      childProps.onPointerUp?.(e)
      cancelLongPress()
    },
    onPointerCancel: (e: ReactPointerEvent<HTMLElement>) => {
      childProps.onPointerCancel?.(e)
      cancelLongPress()
    },
    onClickCapture: (e: ReactMouseEvent<HTMLElement>) => {
      if (swallowClick.current) {
        swallowClick.current = false
        e.preventDefault()
        e.stopPropagation()
        return
      }
      childProps.onClickCapture?.(e)
    },
    onContextMenu: (e: ReactMouseEvent<HTMLElement>) => {
      childProps.onContextMenu?.(e)
      if (touchActions) {
        // the long-press is ours: no native callout / selection menu on touch
        e.preventDefault()
        return
      }
      if (!desktopMenu) {
        return
      }
      e.preventDefault()
      closePreview()
      let { clientX: x, clientY: y } = e
      // the keyboard's context-menu key reports no pointer position
      if (x === 0 && y === 0) {
        const rect = e.currentTarget.getBoundingClientRect()
        x = rect.left
        y = rect.bottom
      }
      menuOpener.current = document.activeElement
      setMenu({ x, y, open: true })
    },
    onKeyDown: (e: ReactKeyboardEvent<HTMLElement>) => {
      childProps.onKeyDown?.(e)
      // Shift+F2 starts or opens the thread from any focused control in the cell:
      // the marker exists only once a thread does, and not every keyboard has a
      // context-menu key. Handled here so an enclosing grid does not open its own
      // selected cell's thread as well.
      if (
        e.key === 'F2' &&
        e.shiftKey &&
        onOpenComments &&
        !menuDisabled &&
        !e.defaultPrevented &&
        e.currentTarget.contains(e.target as Node)
      ) {
        e.preventDefault()
        e.stopPropagation()
        onOpenComments(commentAnchorOf(e.currentTarget))
      }
    },
  })

  return (
    <>
      {cell}
      {previewable && previewOpen ? (
        <CommentPreview
          anchorRef={cellRef}
          comments={comments}
          onClose={closePreview}
          onPointerEnter={clearPreviewTimer}
          onPointerLeave={() => schedulePreview(false)}
        />
      ) : null}
      {desktopMenu && menu ? (
        <CellMenu
          at={menu}
          buildActions={buildActions}
          onPick={(run) => {
            pending.current = run
          }}
          onDismiss={() => setMenu((m) => (m ? { ...m, open: false } : m))}
          onCloseAutoFocus={onMenuCloseAutoFocus}
        />
      ) : null}
      {touchActions && actionsModal ? (
        <CellActionsModal
          open={actionsModal === 'open'}
          title={title}
          buildActions={buildActions}
          onPick={(run) => {
            pending.current = run
            setActionsModal('closing')
          }}
          onDismiss={() => setActionsModal('closing')}
          onCloseAutoFocus={(e) => {
            setActionsModal(null)
            runPending(e)
          }}
        />
      ) : null}
    </>
  )
}

type BuildActions = (t: TFunction) => CellAction[]

function CommentPreview({
  anchorRef,
  comments,
  onClose,
  onPointerEnter,
  onPointerLeave,
}: {
  anchorRef: RefObject<HTMLElement | null>
  comments: BudgetCommentDto[]
  onClose: () => void
  onPointerEnter: () => void
  onPointerLeave: () => void
}) {
  const { t, i18n } = useTranslation()
  const latest = sortByCreatedAt(comments).slice(-PREVIEW_COUNT)
  const rest = comments.length - latest.length
  return (
    <Popover open onOpenChange={(o) => !o && onClose()}>
      <PopoverAnchor virtualRef={anchorRef as RefObject<HTMLElement>} />
      <PopoverContent
        align="end"
        className="w-72"
        role="tooltip"
        data-testid="comment-preview"
        // a read-only card: it never takes focus from the cell or hands it anywhere on close
        onOpenAutoFocus={(e) => e.preventDefault()}
        onCloseAutoFocus={(e) => e.preventDefault()}
        onPointerEnter={onPointerEnter}
        onPointerLeave={onPointerLeave}
      >
        <div className="flex flex-col gap-2">
          {latest.map((c) => (
            <div key={c.id} className="flex flex-col gap-0.5">
              <div className="flex items-baseline gap-1.5">
                <span className="truncate text-xs font-medium">{c.author.name}</span>
                <span className="text-[11px] text-muted-foreground">{formatCommentTime(c.createdAt, i18n.language)}</span>
              </div>
              <p className="line-clamp-3 whitespace-pre-wrap text-sm">{c.comment}</p>
            </div>
          ))}
          {rest > 0 ? <p className="text-xs text-muted-foreground">{t('budgets.page.plan.comments.more', { count: rest })}</p> : null}
        </div>
      </PopoverContent>
    </Popover>
  )
}

function CellMenu({
  at,
  buildActions,
  onPick,
  onDismiss,
  onCloseAutoFocus,
}: {
  at: { x: number; y: number; open: boolean }
  buildActions: BuildActions
  onPick: (run: ActionRun) => void
  onDismiss: () => void
  onCloseAutoFocus: (e: Event) => void
}) {
  const { t } = useTranslation()
  return (
    <DropdownMenu open={at.open} onOpenChange={(o) => !o && onDismiss()}>
      {/* the menu positions against its trigger: a zero-size stand-in at the pointer,
          portalled so it never lands inside a table row or a grid track */}
      {createPortal(
        <DropdownMenuTrigger asChild>
          <span aria-hidden className="pointer-events-none fixed size-0" style={{ left: at.x, top: at.y }} />
        </DropdownMenuTrigger>,
        document.body,
      )}
      <DropdownMenuContent className="w-52" onCloseAutoFocus={onCloseAutoFocus}>
        {buildActions(t).map((a) => (
          <DropdownMenuItem key={a.key} onSelect={() => onPick(a.run)}>
            {a.label}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function CellActionsModal({
  open,
  title,
  buildActions,
  onPick,
  onDismiss,
  onCloseAutoFocus,
}: {
  open: boolean
  title: string
  buildActions: BuildActions
  onPick: (run: ActionRun) => void
  onDismiss: () => void
  onCloseAutoFocus: (e: Event) => void
}) {
  const { t } = useTranslation()
  return (
    <ResponsiveDialog open={open} onOpenChange={(o) => !o && onDismiss()} title={title} onCloseAutoFocus={onCloseAutoFocus}>
      <div className="flex flex-col gap-2" data-testid="cell-actions">
        {buildActions(t).map((a) => (
          <Button key={a.key} type="button" variant="secondary" className="h-11 justify-start" onClick={() => onPick(a.run)}>
            {a.label}
          </Button>
        ))}
      </div>
    </ResponsiveDialog>
  )
}
