import { useEffect, useRef, useState } from 'react'
import type { KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { limitAmountFromInput } from './limitAmount'

export type CellMove = 'down' | 'right' | 'left' | 'up' | 'none'

const MOVES: Record<string, CellMove> = { Enter: 'down', ArrowDown: 'down', ArrowUp: 'up', Tab: 'right' }

/** The Plan grid's in-cell editor: it sits in the cell and keeps the grid's keys. */
export function PlanCellInput({
  initial,
  label,
  onCommit,
  onCancel,
}: {
  initial: string
  label: string
  /** called only with a valid value; the grid decides whether it changed */
  onCommit: (raw: string, move: CellMove) => void
  onCancel: () => void
}) {
  const { t } = useTranslation()
  const [value, setValue] = useState(initial)
  const [invalid, setInvalid] = useState(false)
  const ref = useRef<HTMLInputElement | null>(null)
  // closing by key already committed or cancelled; the blur that follows must not commit again
  const closed = useRef(false)

  useEffect(() => {
    const el = ref.current
    if (el) {
      el.focus()
      el.setSelectionRange(el.value.length, el.value.length)
    }
  }, [])

  const tryCommit = (move: CellMove): boolean => {
    if (!limitAmountFromInput(value).ok) {
      setInvalid(true)
      return false
    }
    closed.current = true
    onCommit(value, move)
    return true
  }

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    // the grid's own keys (arrows, Enter, Delete, Shift+F2) must not act on the cell underneath
    e.stopPropagation()
    if (e.key === 'Escape') {
      e.preventDefault()
      closed.current = true
      onCancel()
      return
    }
    const move = e.key === 'Tab' && e.shiftKey ? 'left' : MOVES[e.key]
    if (move) {
      e.preventDefault()
      tryCommit(move)
    }
  }

  return (
    <span className="relative block w-full">
      <input
        ref={ref}
        type="text"
        inputMode="decimal"
        autoComplete="off"
        aria-label={label}
        aria-invalid={invalid}
        value={value}
        onChange={(e) => {
          setValue(e.target.value)
          setInvalid(false)
        }}
        onKeyDown={onKeyDown}
        onBlur={() => {
          // a click elsewhere commits; an invalid value there is dropped rather than
          // leaving an editor open in a cell the user has already left
          if (!closed.current && !tryCommit('none')) {
            closed.current = true
            onCancel()
          }
        }}
        className="w-full rounded-sm bg-background px-1 text-right text-[15px] tabular-nums outline-none ring-2 ring-ring"
      />
      {invalid ? (
        <span role="alert" className="absolute top-full right-0 z-30 mt-0.5 whitespace-nowrap rounded bg-background px-1 text-xs text-expense shadow">
          {t('common.validation.invalid_formula')}
        </span>
      ) : null}
    </span>
  )
}
