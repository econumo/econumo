import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { CSSProperties, RefObject } from 'react'
import { layoutHeight } from '@/hooks/useKeyboardInset'
import type { KeyboardInset } from '@/hooks/useKeyboardInset'
import { ownsKeyboard } from '@/lib/ownsKeyboard'

// px kept free above the sheet
export const SHEET_TOP_GAP = 24
// px between the status bar and a sheet that fills the screen
const STATUS_BAR_GAP = 8
// ms a focused field may wait for a keyboard before the sheet stops leaving
// room for one (a hardware keyboard raises none)
const KEYBOARD_WAIT_MS = 800
// ms a shorter keyboard must stay before it replaces the remembered one: the
// number keyboard is still up for a moment while the text one replaces it
const KEYBOARD_SETTLE_MS = 600
// px a vaul sheet slides in from when a field takes focus as it opens: less
// than the gap under the search, so the field is never below the keyboard's
// edge while iOS decides whether to pan
const FOCUSED_OPEN_SLIDE = '10px'

/**
 * Where the sheet sits when it is closed: vaul's `--initial-transform`, which
 * its slide keyframes read at both ends. The default, 100% of the sheet's OWN
 * height, only carries a sheet off the screen when it rests on the bottom
 * edge. An anchored sheet does not: it rests on the keyboard, so its bottom
 * edge is well above the screen's, and translating it down by its own height
 * ended the slide with the sheet still covering the lower part of the screen,
 * where it sat until the animation's end unmounted it and it vanished.
 * `elementTop` is the sheet's own `top`, the pan included.
 */
export function closedTransform(layout: number, elementTop: number): string {
  // ceil, not round: a half pixel short still leaves a sliver on screen
  return String(Math.max(0, Math.ceil(layout - elementTop))) + 'px'
}
const KEYBOARD_STORE = 'econumo.keyboardVisible'

// The visible height the on-screen keyboard left the last time a sheet's field
// had it up, per layout height (portrait and landscape differ). iOS pans the
// screen for a field the keyboard would cover, animating the pan while
// reporting only where it ends, so a sheet cannot follow it. A sheet that is
// already this tall when its field takes focus keeps the field above the
// keyboard, and iOS pans nothing. Unknown until a keyboard was seen once.
function rememberedVisible(layout: number): number | undefined {
  try {
    const value = (JSON.parse(localStorage.getItem(KEYBOARD_STORE) ?? '{}') as Record<string, unknown>)[layout]
    return typeof value === 'number' && value > 0 && value < layout ? value : undefined
  } catch {
    return undefined
  }
}

function rememberVisible(layout: number, visible: number) {
  try {
    const all = JSON.parse(localStorage.getItem(KEYBOARD_STORE) ?? '{}') as Record<string, unknown>
    localStorage.setItem(KEYBOARD_STORE, JSON.stringify({ ...all, [layout]: visible }))
  } catch {
    // private mode: the sheet simply does not make room in advance
  }
}

export interface SheetAnchor {
  /** px from the top of the visible area, chosen as the sheet opens */
  top: number
  /** px the sheet's content takes */
  natural: number
  /** px the top never goes above: clear of the status bar */
  minTop: number
  /** px of the layout viewport it was measured on: a new size measures afresh */
  layout: number
}

let safeTop: { width: number; px: number } | null = null

// the status bar's height on an edge-to-edge screen (viewport-fit=cover)
function safeAreaTop(): number {
  if (safeTop?.width !== window.innerWidth) {
    const probe = document.createElement('div')
    probe.style.cssText = 'position:fixed;top:0;visibility:hidden;height:env(safe-area-inset-top, 0px)'
    document.body.append(probe)
    safeTop = { width: window.innerWidth, px: probe.offsetHeight }
    probe.remove()
  }
  return safeTop.px
}

/**
 * Where the sheet's top sits on screen for the visible area left by the
 * keyboard: high enough to show all its content above the keyboard, up to the
 * status bar. Keeping the top where it opened instead left a list that fits
 * the screen, but not the space above the keyboard, with two rows in view.
 */
export function anchoredTop(anchor: SheetAnchor, visibleHeight: number): number {
  return Math.max(anchor.minTop, Math.min(anchor.top, visibleHeight - anchor.natural))
}

/**
 * A sheet as tall as its content, up to the status bar. A list longer than
 * the screen keeps its top there whatever the keyboard does, and only its
 * bottom edge moves; a shorter one rises above the keyboard as far as its
 * content needs and comes back down when the keyboard goes. The content's
 * height only ever grows (data arriving), never shrinks with a filter, so
 * typing does not move the top. A closing sheet keeps its place while it
 * slides out.
 */
/** the height the sheet lays itself out in: what the keyboard leaves, or will leave */
export function sheetVisibleHeight(layout: number, keyboardVisible: number, remembered: number | undefined, expectsField: boolean, noKeyboard: boolean): number {
  const room = expectsField && remembered !== undefined
  if (keyboardVisible > 0) {
    // the number keyboard is still up while the taller text one replaces it
    return room ? Math.min(keyboardVisible, remembered) : keyboardVisible
  }
  return room && !noKeyboard ? remembered : layout
}

// The height the body's content takes, whatever height the body is drawn at:
// the body stretches to the sheet's fixed height, so its own scrollHeight
// never reports content shorter than that (a list shorter than last time)
function contentHeight(body: HTMLElement): number {
  const last = body.lastElementChild
  if (!last) {
    return body.scrollHeight
  }
  const paddingBottom = parseFloat(getComputedStyle(body).paddingBottom) || 0
  // rows that scroll inside a list of their own
  const list = body.querySelector<HTMLElement>('[cmdk-list]')
  const inner = list ? list.scrollHeight - list.clientHeight : 0
  // from the top of the scrolled content (its top padding included) to the
  // last element's end
  return Math.ceil(last.getBoundingClientRect().bottom - body.getBoundingClientRect().top + body.scrollTop + paddingBottom + inner)
}

export interface AnchoredSheet {
  style: CSSProperties
  /** px from the top of the visible area to the sheet's top */
  top: number
}

export function useAnchoredSheet(
  ref: RefObject<HTMLElement | null>,
  body: HTMLElement | null,
  enabled: boolean,
  open: boolean,
  keyboard: KeyboardInset,
): AnchoredSheet | undefined {
  const [anchor, setAnchor] = useState<SheetAnchor | null>(null)
  const measured = useRef(false)
  // state, so a resize or a rotation with no keyboard up re-renders the sheet
  const [layout, setLayout] = useState(layoutHeight)
  useEffect(() => {
    if (!enabled || !open) {
      return
    }
    const update = () => setLayout(layoutHeight())
    update()
    window.addEventListener('resize', update)
    return () => window.removeEventListener('resize', update)
  }, [enabled, open])
  // after every render, since the content can grow without the keyboard
  // changing; it only ever sets the anchor higher, so it settles
  // oxlint-disable-next-line react-hooks/exhaustive-deps
  useLayoutEffect(() => {
    if (!enabled) {
      measured.current = false
      if (anchor !== null) {
        setAnchor(null)
      }
      return
    }
    // a closing sheet keeps its place while it slides out; the next opening
    // measures afresh
    if (!open) {
      measured.current = false
      return
    }
    const el = ref.current
    if (!el || !body) {
      return
    }
    const chrome = el.offsetHeight - body.clientHeight
    const natural = chrome + contentHeight(body)
    const minTop = Math.max(SHEET_TOP_GAP, safeAreaTop() + STATUS_BAR_GAP)
    const top = Math.max(minTop, layout - natural)
    // the content grew (rows arriving, the query reset), or the screen changed
    if (!measured.current || anchor === null || anchor.layout !== layout || top < anchor.top - 1) {
      measured.current = true
      setAnchor({ top, natural, minTop, layout })
    }
  })

  const active = enabled && open
  // a field of the sheet is about to raise the keyboard, or has it up
  const [fieldTaken, setFieldTaken] = useState(false)
  const expectsField = active && fieldTaken
  // a field took focus as the sheet opened
  const [focusedOpen, setFocusedOpen] = useState(false)
  // it waited and none came
  const [noKeyboard, setNoKeyboard] = useState(false)
  const [remembered, setRemembered] = useState<number | undefined>(undefined)
  useLayoutEffect(() => {
    if (active) {
      setRemembered(rememberedVisible(layout))
    }
  }, [active, layout])
  // whether it could make room as it opened; a height learned later, with the
  // sheet already on screen, changes nothing about how it came in
  const [roomAtOpen, setRoomAtOpen] = useState(false)
  useLayoutEffect(() => {
    setRoomAtOpen(active && rememberedVisible(layoutHeight()) !== undefined)
  }, [active])
  // the keyboard was up for the field that has it now
  const sawKeyboard = useRef(false)
  useLayoutEffect(() => {
    const el = ref.current
    if (!active || !el || !body) {
      return
    }
    const focused = () => ownsKeyboard(document.activeElement) && el.contains(document.activeElement)
    setFieldTaken(focused())
    setFocusedOpen(focused())
    // Room is made when the field takes focus, not when it is touched: the
    // sheet resizing under a finger still down moves the field away, and the
    // tap then lands on something else (seen in Chromium)
    let touched = false
    const onPointerDown = () => {
      touched = true
    }
    const onFocusIn = (e: FocusEvent) => {
      if (ownsKeyboard(e.target)) {
        setFieldTaken(true)
        // focus nobody touched the sheet for is the sheet's own, as it opens
        if (!touched) {
          setFocusedOpen(true)
        }
      }
    }
    // a tap on a field that kept its focus while the keyboard was hidden asks
    // for the keyboard again
    const onClick = (e: MouseEvent) => {
      if (ownsKeyboard(e.target)) {
        sawKeyboard.current = false
        setNoKeyboard(false)
      }
    }
    const onFocusOut = (e: FocusEvent) => {
      if (!ownsKeyboard(e.relatedTarget)) {
        setFieldTaken(false)
      }
    }
    el.addEventListener('pointerdown', onPointerDown, true)
    el.addEventListener('focusin', onFocusIn)
    el.addEventListener('focusout', onFocusOut)
    el.addEventListener('click', onClick)
    return () => {
      el.removeEventListener('pointerdown', onPointerDown, true)
      el.removeEventListener('focusin', onFocusIn)
      el.removeEventListener('focusout', onFocusOut)
      el.removeEventListener('click', onClick)
      setFieldTaken(false)
      setFocusedOpen(false)
    }
  }, [active, body, ref])
  useEffect(() => {
    if (!expectsField || keyboard.height > 0) {
      sawKeyboard.current = expectsField
      setNoKeyboard(false)
      return
    }
    // hidden while the field keeps its focus (Android's back button)
    if (sawKeyboard.current) {
      setNoKeyboard(true)
      return
    }
    const timer = setTimeout(() => {
      setNoKeyboard(true)
    }, KEYBOARD_WAIT_MS)
    return () => clearTimeout(timer)
  }, [expectsField, keyboard.height])
  useEffect(() => {
    if (!expectsField || keyboard.height === 0 || keyboard.height === remembered) {
      return
    }
    const keep = () => {
      rememberVisible(layout, keyboard.height)
      setRemembered(keyboard.height)
    }
    // a taller keyboard needs its room at once
    if (remembered === undefined || keyboard.height < remembered) {
      keep()
      return
    }
    const timer = setTimeout(keep, KEYBOARD_SETTLE_MS)
    return () => clearTimeout(timer)
  }, [expectsField, keyboard.height, remembered, layout])

  // with next to no slide, the sheet fades in
  const shortSlide = open && focusedOpen && roomAtOpen
  useLayoutEffect(() => {
    if (shortSlide) {
      ref.current?.animate?.([{ opacity: 0 }, { opacity: 1 }], { duration: 150, easing: 'ease-out' })
    }
  }, [shortSlide, ref])

  // a closing sheet slides out as it stood, whatever its field and the
  // keyboard do meanwhile
  const closing = useRef<{ top: number; height: number; pan: number } | null>(null)
  if (!enabled || anchor === null) {
    return undefined
  }
  let top: number
  let height: number
  // the keyboard is no longer tracked for a closed sheet: it keeps the pan it had
  let pan = keyboard.top
  if (!open && closing.current) {
    ;({ top, height, pan } = closing.current)
  } else {
    const visibleHeight = sheetVisibleHeight(layout, keyboard.height, remembered, expectsField, noKeyboard)
    top = anchoredTop(anchor, visibleHeight)
    height = visibleHeight - top
    closing.current = { top, height, pan }
  }
  return {
    top,
    style: {
      // placed in the layout viewport, which iOS pans by keyboard.top
      top: pan + top,
      bottom: 'auto',
      // the drawer's mt-24 only kept a bottom-anchored sheet off the top
      marginTop: 0,
      height,
      maxHeight: 'none',
      // the entrance keeps the value it always had; only the exit is corrected,
      // so a sheet that slid in one way does not start sliding in another. The
      // property is always set, never dropped between renders, so the flip
      // happens with the state change rather than a render later
      '--initial-transform': shortSlide ? FOCUSED_OPEN_SLIDE : open ? '100%' : closedTransform(layout, pan + top),
    } as CSSProperties,
  }
}
