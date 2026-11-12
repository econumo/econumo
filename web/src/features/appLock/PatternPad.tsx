import { useEffect, useRef, useState, type PointerEvent as ReactPointerEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { encodePattern, extendPattern, PATTERN_MIN_NODES, PATTERN_NODES } from './pattern'

// The red pattern of a rejected or too-short try stays this long, then clears.
export const PATTERN_FLASH_MS = 600
// Within this distance of a dot's centre (in cells) the line picks it up.
const HIT_RADIUS = 0.42
// A press that travels less than this (in cells) is a tap on one dot.
const TAP_SLOP = 0.25
const SAMPLE_STEP = 0.15

export interface PatternPadProps {
  /** Receives the encoded pattern ("p:0-4-8-5") once 4+ dots are drawn; resolve false to reject it: it flashes red and clears. */
  onSubmit: (pattern: string) => boolean | Promise<boolean>
  disabled?: boolean
  /** Shown above the grid in the error colour and announced; wins over the hint. */
  error?: string
  hint?: string
  className?: string
}

interface Point {
  x: number
  y: number
}

type Phase = 'idle' | 'drawing' | 'busy' | 'error'

function centre(node: number): Point {
  return { x: (node % 3) + 0.5, y: Math.floor(node / 3) + 0.5 }
}

function hit(point: Point): number | null {
  const col = Math.floor(point.x)
  const row = Math.floor(point.y)
  if (col < 0 || col > 2 || row < 0 || row > 2) {
    return null
  }
  const node = row * 3 + col
  const c = centre(node)
  return Math.hypot(point.x - c.x, point.y - c.y) <= HIT_RADIUS ? node : null
}

export function PatternPad({ onSubmit, disabled = false, error, hint, className }: PatternPadProps) {
  const { t } = useTranslation()
  const [nodes, setNodesState] = useState<number[]>([])
  const [phase, setPhaseState] = useState<Phase>('idle')
  const [pointer, setPointer] = useState<Point | null>(null)
  // Dots picked one at a time (keyboard, screen reader, single taps) wait for Done.
  const [stepping, setStepping] = useState(false)
  const [tooShort, setTooShort] = useState(false)
  const nodesRef = useRef<number[]>([])
  const phaseRef = useRef<Phase>('idle')
  const steppingRef = useRef(false)
  const grid = useRef<HTMLDivElement>(null)
  const press = useRef<{ start: Point; last: Point; base: number[]; travel: number } | null>(null)
  const mounted = useRef(true)
  const flashTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
      if (flashTimer.current !== null) {
        clearTimeout(flashTimer.current)
      }
    }
  }, [])

  const setNodes = (value: number[]) => {
    nodesRef.current = value
    setNodesState(value)
  }
  const setPhase = (value: Phase) => {
    phaseRef.current = value
    setPhaseState(value)
  }
  const setStep = (value: boolean) => {
    steppingRef.current = value
    setStepping(value)
  }

  const cancelFlash = () => {
    if (flashTimer.current !== null) {
      clearTimeout(flashTimer.current)
      flashTimer.current = null
    }
  }

  const reset = () => {
    cancelFlash()
    setNodes([])
    setPointer(null)
    setStep(false)
    setPhase('idle')
  }

  const flashThenClear = () => {
    setPhase('error')
    setPointer(null)
    cancelFlash()
    flashTimer.current = setTimeout(() => {
      flashTimer.current = null
      if (mounted.current) {
        reset()
      }
    }, PATTERN_FLASH_MS)
  }

  const submit = (value: number[]) => {
    setStep(false)
    setPhase('busy')
    setPointer(null)
    void Promise.resolve()
      .then(() => onSubmit(encodePattern(value)))
      .catch(() => false)
      .then((accepted) => {
        if (!mounted.current) {
          return
        }
        if (accepted) {
          reset()
        } else {
          flashThenClear()
        }
      })
  }

  const blocked = () => disabled || phaseRef.current === 'busy'

  const toGrid = (event: ReactPointerEvent): Point | null => {
    const rect = grid.current?.getBoundingClientRect()
    if (!rect || rect.width === 0 || rect.height === 0) {
      return null
    }
    return { x: ((event.clientX - rect.left) / rect.width) * 3, y: ((event.clientY - rect.top) / rect.height) * 3 }
  }

  const pickUp = (point: Point) => {
    const node = hit(point)
    if (node !== null && !nodesRef.current.includes(node)) {
      setNodes(extendPattern(nodesRef.current, node))
    }
  }

  // The drawer around the pad must neither scroll nor follow the finger.
  const onPointerDown = (event: ReactPointerEvent<HTMLDivElement>) => {
    event.stopPropagation()
    if (blocked() || (event.pointerType === 'mouse' && event.button !== 0)) {
      return
    }
    event.preventDefault()
    try {
      event.currentTarget.setPointerCapture?.(event.pointerId)
    } catch {
      // a synthetic pointer has nothing to capture
    }
    cancelFlash()
    setTooShort(false)
    const point = toGrid(event)
    const base = steppingRef.current ? nodesRef.current : []
    press.current = { start: point ?? { x: 0, y: 0 }, last: point ?? { x: 0, y: 0 }, base, travel: 0 }
    setNodes([])
    setPhase('drawing')
    if (point) {
      pickUp(point)
      setPointer(point)
    }
  }

  const onPointerMove = (event: ReactPointerEvent<HTMLDivElement>) => {
    event.stopPropagation()
    const current = press.current
    if (current === null || phaseRef.current !== 'drawing') {
      return
    }
    const point = toGrid(event)
    if (!point) {
      return
    }
    const dx = point.x - current.last.x
    const dy = point.y - current.last.y
    const distance = Math.hypot(dx, dy)
    // A fast swipe can jump past a dot between two events.
    const steps = Math.max(1, Math.ceil(distance / SAMPLE_STEP))
    for (let i = 1; i <= steps; i++) {
      pickUp({ x: current.last.x + (dx * i) / steps, y: current.last.y + (dy * i) / steps })
    }
    current.travel = Math.max(current.travel, Math.hypot(point.x - current.start.x, point.y - current.start.y))
    current.last = point
    setPointer(point)
  }

  const onPointerUp = (event: ReactPointerEvent<HTMLDivElement>) => {
    event.stopPropagation()
    const current = press.current
    press.current = null
    if (current === null || phaseRef.current !== 'drawing') {
      return
    }
    setPointer(null)
    const drawn = nodesRef.current
    if (drawn.length <= 1 && current.travel < TAP_SLOP) {
      // A tap picks one dot and keeps the ones picked before it.
      setNodes(drawn.length === 1 ? extendPattern(current.base, drawn[0]) : current.base)
      setStep(drawn.length === 1 || current.base.length > 0)
      setPhase('idle')
      return
    }
    setStep(false)
    if (drawn.length < PATTERN_MIN_NODES) {
      setTooShort(true)
      flashThenClear()
      return
    }
    submit(drawn)
  }

  const onPointerCancel = (event: ReactPointerEvent<HTMLDivElement>) => {
    event.stopPropagation()
    if (press.current !== null) {
      press.current = null
      reset()
    }
  }

  // A pointer's click arrives with a detail count and was handled as a tap;
  // the keyboard's (Enter, Space) has none.
  const onDotClick = (node: number, detail: number) => {
    if (detail !== 0 || blocked()) {
      return
    }
    cancelFlash()
    setTooShort(false)
    const base = steppingRef.current ? nodesRef.current : []
    setNodes(extendPattern(base, node))
    setStep(true)
    setPhase('idle')
  }

  const onDone = () => {
    if (blocked()) {
      return
    }
    if (nodesRef.current.length < PATTERN_MIN_NODES) {
      setTooShort(true)
      return
    }
    submit(nodesRef.current)
  }

  const message = error || (tooShort ? t('appLock.pattern_pad.too_short') : hint)
  const failed = phase === 'error'
  const last = nodes.at(-1)
  const points = nodes.map((n) => {
    const c = centre(n)
    return `${c.x},${c.y}`
  })
  const tail = phase === 'drawing' && pointer !== null && last !== undefined ? { from: centre(last), to: pointer } : null

  return (
    <div className={cn('flex flex-col items-center', className)}>
      <p
        aria-live="assertive"
        className={cn('min-h-5 max-w-64 text-center text-sm', error || tooShort ? 'text-destructive' : 'text-muted-foreground')}
      >
        {message}
      </p>
      <div
        ref={grid}
        role="group"
        aria-label={t('appLock.pattern_pad.grid')}
        data-testid="pattern-grid"
        data-nodes={nodes.join('-')}
        data-vaul-no-drag=""
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
        onPointerCancel={onPointerCancel}
        onMouseDown={(event) => event.preventDefault()}
        className={cn(
          'relative mt-4 grid aspect-square w-[15.75rem] touch-none grid-cols-3 select-none [-webkit-tap-highlight-color:transparent]',
          disabled && 'pointer-events-none opacity-40',
          failed && 'pin-shake',
        )}
      >
        <svg
          aria-hidden="true"
          viewBox="0 0 3 3"
          className={cn(
            'pointer-events-none absolute inset-0 size-full fill-none',
            failed ? 'stroke-destructive/70' : 'stroke-primary/55',
          )}
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          {points.length > 1 && (
            <polyline points={points.join(' ')} strokeWidth={6} vectorEffect="non-scaling-stroke" />
          )}
          {tail && (
            <line x1={tail.from.x} y1={tail.from.y} x2={tail.to.x} y2={tail.to.y} strokeWidth={6} vectorEffect="non-scaling-stroke" />
          )}
        </svg>
        {Array.from({ length: PATTERN_NODES }, (_, node) => {
          const on = nodes.includes(node)
          return (
            <button
              key={node}
              type="button"
              aria-label={t('appLock.pattern_pad.dot', { n: node + 1 })}
              aria-pressed={on}
              disabled={disabled || phase === 'busy'}
              onClick={(event) => onDotClick(node, event.detail)}
              className="group relative flex items-center justify-center outline-none"
            >
              <span
                className={cn(
                  'absolute size-14 rounded-full transition-colors duration-150 group-focus-visible:ring-3 group-focus-visible:ring-ring/60',
                  on && (failed ? 'bg-destructive/12' : 'bg-primary/12'),
                )}
              />
              <span
                className={cn(
                  'relative size-3.5 rounded-full transition-[transform,background-color] duration-150',
                  on ? (failed ? 'scale-125 bg-destructive' : 'pin-dot-fill scale-125 bg-primary') : 'bg-foreground/30',
                )}
              />
            </button>
          )
        })}
      </div>
      {stepping && nodes.length > 0 && (
        <div className="mt-2 flex gap-2">
          <Button type="button" variant="ghost" className="h-9 normal-case! tracking-normal!" onClick={reset}>
            {t('appLock.pattern_pad.clear')}
          </Button>
          <Button
            type="button"
            variant="ghost"
            className="h-9 text-econumo-link normal-case! tracking-normal!"
            disabled={nodes.length < PATTERN_MIN_NODES}
            onClick={onDone}
          >
            {t('appLock.pattern_pad.done')}
          </Button>
        </div>
      )}
    </div>
  )
}
