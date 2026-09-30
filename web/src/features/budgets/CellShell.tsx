import { useEffect, useRef, useState } from 'react'
import type { PointerEvent as ReactPointerEvent, ReactElement } from 'react'
import { useTranslation } from 'react-i18next'
import { HoverCard, HoverCardContent, HoverCardTrigger } from '@/components/ui/hover-card'
import { ContextMenu, ContextMenuContent, ContextMenuItem, ContextMenuTrigger } from '@/components/ui/context-menu'
import { Button } from '@/components/ui/button'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { useIsCompact } from '@/hooks/useIsCompact'
import type { BudgetCommentDto } from '@/api/dto/budget'
import { commentAnchorOf } from './cellDom'
import { formatCommentTime, sortByCreatedAt } from './CommentThread'

interface CellShellProps {
  /** the cell element: must be a single DOM element that accepts a ref and props */
  children: ReactElement
  /** heading of the touch actions modal: the element's display name */
  title: string
  comments: BudgetCommentDto[]
  /** no hover preview while a thread is open (touch viewports never preview) */
  previewDisabled?: boolean
  /** no menu / actions modal: phones (stage 2 gives them the item sheet) and edit-structure mode */
  menuDisabled?: boolean
  onSetBudget?: (anchor: HTMLElement) => void
  /** omitted for the uncategorized row */
  onOpenComments?: (anchor: HTMLElement) => void
  onShowTransactions?: () => void
}

interface CellAction {
  key: string
  label: string
  run: (anchor: HTMLElement) => void
}

const PREVIEW_COUNT = 2
const LONG_PRESS_MS = 500
// a finger drifting further than this is a scroll, not a press
const LONG_PRESS_SLOP_PX = 10

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
  const { t, i18n } = useTranslation()
  const isTouch = useIsCompact()
  // Radix types HoverCardTrigger's ref as HTMLAnchorElement unconditionally (its
  // default rendered tag), even though `asChild` hands the ref to `children`
  // instead — there is no polymorphic ref inference for that composition. The
  // type is cosmetic here: commentAnchorOf only needs an Element, and the DOM
  // node is always whatever `children` actually renders.
  const cellRef = useRef<HTMLAnchorElement | null>(null)
  const [previewOpen, setPreviewOpen] = useState(false)
  const [actionsOpen, setActionsOpen] = useState(false)
  // a press means the user is acting on the cell (amount editor, selection): the
  // preview stays shut until the pointer leaves, or its pending open timer would
  // pop it over the editor the press just opened
  const pressed = useRef(false)
  // The menu/modal runs its action only after it has closed and handed focus back,
  // or the returning focus would steal it from the popover/dialog the action opens.
  const pending = useRef<((anchor: HTMLElement) => void) | null>(null)
  const pressTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const pressStart = useRef<{ x: number; y: number } | null>(null)
  // the finger's release after a long-press fires a click on the cell underneath;
  // swallow that one click so the tap action (set-limit dialog, selection) does not
  // run under the modal. Reset by the next press, so a browser that never sends
  // that click cannot make a later, genuine tap disappear.
  const swallowClick = useRef(false)

  // Unmounting mid-press (the row's data reloads, the column scrolls out) must
  // not fire the modal after the fact.
  useEffect(
    () => () => {
      if (pressTimer.current) {
        clearTimeout(pressTimer.current)
      }
    },
    [],
  )

  const actions: CellAction[] = []
  if (!menuDisabled) {
    if (onSetBudget) {
      actions.push({ key: 'set-budget', label: t('budgets.modal.set_limit_form.header'), run: onSetBudget })
    }
    if (onOpenComments) {
      actions.push({
        key: 'comments',
        label: comments.length > 0 ? t('budgets.page.plan.comments.disclosure', { count: comments.length }) : t('budgets.page.plan.comments.add'),
        run: onOpenComments,
      })
    }
    if (onShowTransactions) {
      actions.push({ key: 'transactions', label: t('budgets.page.budget.structure.element.action.show_transactions'), run: () => onShowTransactions() })
    }
  }
  const previewable = !isTouch && !previewDisabled && comments.length > 0
  const touchActions = isTouch && actions.length > 0
  const desktopMenu = !isTouch && actions.length > 0

  const runPending = (e: Event) => {
    const action = pending.current
    pending.current = null
    if (action && cellRef.current) {
      e.preventDefault()
      action(commentAnchorOf(cellRef.current))
    }
  }
  const cancelLongPress = () => {
    if (pressTimer.current) {
      clearTimeout(pressTimer.current)
      pressTimer.current = null
    }
    pressStart.current = null
  }

  const latest = sortByCreatedAt(comments).slice(-PREVIEW_COUNT)
  const rest = comments.length - latest.length

  return (
    <>
      <HoverCard
        open={previewable && previewOpen}
        onOpenChange={(open) => setPreviewOpen(open && !pressed.current)}
        openDelay={300}
        closeDelay={100}
      >
        <ContextMenu>
          <ContextMenuTrigger asChild disabled={!desktopMenu}>
            <HoverCardTrigger
              asChild
              ref={cellRef}
              onPointerDown={(e: ReactPointerEvent) => {
                pressed.current = true
                swallowClick.current = false
                setPreviewOpen(false)
                if (touchActions && e.pointerType !== 'mouse') {
                  cancelLongPress()
                  pressStart.current = { x: e.clientX, y: e.clientY }
                  pressTimer.current = setTimeout(() => {
                    pressTimer.current = null
                    swallowClick.current = true
                    setActionsOpen(true)
                  }, LONG_PRESS_MS)
                }
              }}
              onPointerMove={(e: ReactPointerEvent) => {
                const start = pressStart.current
                if (start && Math.hypot(e.clientX - start.x, e.clientY - start.y) > LONG_PRESS_SLOP_PX) {
                  cancelLongPress()
                }
              }}
              onPointerUp={cancelLongPress}
              onPointerCancel={cancelLongPress}
              onPointerLeave={() => {
                pressed.current = false
                cancelLongPress()
              }}
              onClickCapture={(e) => {
                if (swallowClick.current) {
                  swallowClick.current = false
                  e.preventDefault()
                  e.stopPropagation()
                }
              }}
              // the long-press is ours: no native callout / selection menu on touch
              onContextMenu={touchActions ? (e) => e.preventDefault() : undefined}
            >
              {children}
            </HoverCardTrigger>
          </ContextMenuTrigger>
          {desktopMenu ? (
            <ContextMenuContent className="w-52" onCloseAutoFocus={runPending}>
              {actions.map((a) => (
                <ContextMenuItem
                  key={a.key}
                  onSelect={() => {
                    pending.current = a.run
                  }}
                >
                  {a.label}
                </ContextMenuItem>
              ))}
            </ContextMenuContent>
          ) : null}
        </ContextMenu>
        {previewable ? (
          <HoverCardContent align="end" className="w-72" data-testid="comment-preview">
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
          </HoverCardContent>
        ) : null}
      </HoverCard>
      {touchActions ? (
        <ResponsiveDialog open={actionsOpen} onOpenChange={setActionsOpen} title={title} onCloseAutoFocus={runPending}>
          <div className="flex flex-col gap-2" data-testid="cell-actions">
            {actions.map((a) => (
              <Button
                key={a.key}
                type="button"
                variant="secondary"
                className="h-11 justify-start"
                onClick={() => {
                  pending.current = a.run
                  setActionsOpen(false)
                }}
              >
                {a.label}
              </Button>
            ))}
          </div>
        </ResponsiveDialog>
      ) : null}
    </>
  )
}
