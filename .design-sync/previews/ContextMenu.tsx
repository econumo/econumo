import * as React from 'react'
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuShortcut,
  ContextMenuTrigger,
} from 'web'
import { Copy, CopyPlus, Pencil, Trash2 } from 'lucide-react'

// Radix ContextMenu has no open/defaultOpen prop; dispatch a real
// contextmenu event on mount so the static capture shows the open menu.
function useOpenOnMount() {
  const ref = React.useRef<HTMLDivElement>(null)
  React.useEffect(() => {
    const el = ref.current
    if (!el) return
    const rect = el.getBoundingClientRect()
    el.dispatchEvent(
      new MouseEvent('contextmenu', {
        bubbles: true,
        clientX: rect.left + 32,
        clientY: rect.bottom - 4,
      })
    )
  }, [])
  return ref
}

export const TransactionRowMenu = () => {
  const ref = useOpenOnMount()
  return (
    <ContextMenu modal={false}>
      <ContextMenuTrigger asChild>
        <div
          ref={ref}
          className="flex w-80 items-center justify-between rounded-lg border bg-card px-3 py-2 text-sm"
        >
          <div>
            <div className="font-medium">Groceries</div>
            <div className="text-xs text-muted-foreground">
              REWE Supermarket · Main account
            </div>
          </div>
          <span className="text-expense">−$42.50</span>
        </div>
      </ContextMenuTrigger>
      <ContextMenuContent className="w-52">
        <ContextMenuItem>
          <Pencil />
          Edit transaction
        </ContextMenuItem>
        <ContextMenuItem>
          <CopyPlus />
          Duplicate
        </ContextMenuItem>
        <ContextMenuItem>
          <Copy />
          Copy amount
          <ContextMenuShortcut>⌘C</ContextMenuShortcut>
        </ContextMenuItem>
        <ContextMenuSeparator />
        <ContextMenuItem variant="destructive">
          <Trash2 />
          Delete transaction
        </ContextMenuItem>
      </ContextMenuContent>
    </ContextMenu>
  )
}
