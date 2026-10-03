import { cloneElement, useCallback, useEffect, useRef, useState } from 'react'
import type { KeyboardEvent as ReactKeyboardEvent, PointerEvent as ReactPointerEvent, ReactElement, Ref, RefObject } from 'react'
import { useTranslation } from 'react-i18next'
import { Popover, PopoverAnchor, PopoverContent } from '@/components/ui/popover'
import { useIsCompact } from '@/hooks/useIsCompact'
import type { BudgetCommentDto } from '@/api/dto/budget'
import { commentAnchorOf } from './cellDom'
import { formatCommentTime, sortByCreatedAt } from './CommentThread'

type PointerHandler = (e: ReactPointerEvent<HTMLElement>) => void

interface CellProps {
  ref?: Ref<HTMLElement>
  onPointerDown?: PointerHandler
  onPointerEnter?: PointerHandler
  onPointerLeave?: PointerHandler
  onKeyDown?: (e: ReactKeyboardEvent<HTMLElement>) => void
}

interface CellShellProps {
  /** the cell element: a single DOM element; it receives the shell's handlers and ref directly, with no wrapper */
  children: ReactElement
  comments: BudgetCommentDto[]
  /** no hover preview while a thread is open (touch viewports never preview) */
  previewDisabled?: boolean
  /** no Shift+F2: edit-structure mode */
  shortcutDisabled?: boolean
  /** omitted for the uncategorized row */
  onOpenComments?: (anchor: HTMLElement) => void
}

const PREVIEW_COUNT = 2
const PREVIEW_OPEN_MS = 300
const PREVIEW_CLOSE_MS = 100
const OPEN_LIMIT_EDITOR = '[data-limit-trigger][data-state="open"]'

function assignRef<T>(ref: Ref<T> | undefined, value: T | null) {
  if (typeof ref === 'function') {
    ref(value)
  } else if (ref) {
    ref.current = value
  }
}

// A grid re-renders every cell on each selection move, so the shell stays a bare
// set of handlers on the cell itself: the Radix preview mounts as a sibling only
// while open. Wrapping the cell in its trigger instead cost several times the
// grid's own render and one document listener per cell.
export function CellShell({ children, comments, previewDisabled = false, shortcutDisabled = false, onOpenComments }: CellShellProps) {
  const isTouch = useIsCompact()
  const cellRef = useRef<HTMLElement | null>(null)
  const [previewOpen, setPreviewOpen] = useState(false)
  // a press means the user is acting on the cell (amount editor, selection): the
  // preview stays shut until the pointer leaves, or its pending open timer would
  // pop it over the editor the press just opened
  const pressed = useRef(false)
  const previewTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

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

  // Unmounting mid-hover (the row's data reloads, the column scrolls out) must
  // not fire the preview after the fact.
  useEffect(
    () => () => {
      if (previewTimer.current) {
        clearTimeout(previewTimer.current)
      }
    },
    [],
  )

  const previewable = !isTouch && !previewDisabled && comments.length > 0

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
      if (previewOpen || previewTimer.current) {
        schedulePreview(false)
      }
    },
    onPointerDown: (e: ReactPointerEvent<HTMLElement>) => {
      childProps.onPointerDown?.(e)
      pressed.current = true
      closePreview()
    },
    onKeyDown: (e: ReactKeyboardEvent<HTMLElement>) => {
      childProps.onKeyDown?.(e)
      // Shift+F2 starts or opens the thread from any focused control in the cell:
      // the add-comment corner shows only under a hovering mouse and never takes
      // focus. Handled here so an enclosing grid does not open its own selected
      // cell's thread as well.
      if (
        e.key === 'F2' &&
        e.shiftKey &&
        onOpenComments &&
        !shortcutDisabled &&
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
    </>
  )
}

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
