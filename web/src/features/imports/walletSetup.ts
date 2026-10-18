import { useCallback, useEffect, useState } from 'react'
import type { ImportSourceDto } from '@/api/dto/imports'
import { METRICS, trackEvent } from '@/lib/metrics'
import { getItem, removeItem, setItem } from '@/lib/storage'
import { useDiscardImportEvent, useImportQueue, useImportSources } from './queries'

export const STEPS = ['install', 'configure', 'automate', 'payment', 'immediate'] as const
export type StepId = (typeof STEPS)[number]

// What the server answers to the shortcut's setup check-in (and to a run by
// hand): the body has no account. That failure IS the proof the shortcut
// reached the server, and it is cleared from the queue once seen.
const NO_INPUT_ERROR = 'account is required'
const POLL_MS = 4000

// Hand-ticked steps live per source in localStorage: the server knows nothing
// about the phone-side steps, and a reconnect starts the list over.
const ticksKey = (sourceId: string) => `appleWalletSetup:${sourceId}`

function readTicks(sourceId: string): StepId[] {
  const stored = getItem(ticksKey(sourceId))
  if (!Array.isArray(stored)) {
    return []
  }
  const ids = new Set(stored.filter((s): s is string => typeof s === 'string'))
  // the two install steps of the first guide are one step now, and the
  // run-once step became part of connecting
  if (ids.has('install_wallet') && ids.has('install_setup')) {
    ids.add('install')
  }
  if (ids.has('test')) {
    ids.add('configure')
  }
  // Old guides allowed a manual payment tick; only a received tap proves it.
  return STEPS.filter((s) => s !== 'payment' && ids.has(s))
}

export function forgetSetupTicks(sourceId: string): void {
  removeItem(ticksKey(sourceId))
}

export interface WalletSetup {
  done: (id: StepId) => boolean
  /** ticked by hand, so it can be unticked; server-confirmed steps cannot */
  handTicked: (id: StepId) => boolean
  /** the first step not done yet, null once every step is */
  current: StepId | null
  doneCount: number
  tick: (id: StepId, on: boolean) => void
  /** the Shortcuts link carrying the token minted on this page */
  configureLink: string | null
  setConfigureLink: (link: string | null) => void
}

export function useWalletSetup(source: ImportSourceDto | null): WalletSetup {
  const queue = useImportQueue()
  const sources = useImportSources()
  const discardEvent = useDiscardImportEvent()
  const sourceId = source?.id ?? null
  const paymentArrived = source?.cards.some((card) => card.tapCount > 0) ?? false
  // Ignore an old Run Immediately tick saved before a payment was received.
  const [ticks, setTicks] = useState<StepId[]>(() => (sourceId ? readTicks(sourceId).filter((id) => id !== 'immediate' || paymentArrived) : []))
  const [configureLink, setConfigureLink] = useState<string | null>(null)

  useEffect(() => {
    if (sourceId && !paymentArrived && readTicks(sourceId).includes('immediate')) {
      // Remove the invalid saved tick so a later reload cannot restore it.
      setItem(ticksKey(sourceId), ticks.filter((id) => id !== 'immediate'))
    }
  }, [sourceId, paymentArrived, ticks])

  const tick = useCallback(
    (id: StepId, on: boolean) => {
      if (!sourceId || id === 'payment' || (id === 'immediate' && on && !paymentArrived)) {
        return
      }
      setTicks((prev) => {
        const next = on ? STEPS.filter((s) => s === id || prev.includes(s)) : prev.filter((s) => s !== id)
        setItem(ticksKey(sourceId), next)
        return next
      })
    },
    [sourceId, paymentArrived],
  )

  // A parsed payment creates a ledger row counted in tapCount; failed events
  // and card mappings without payments do not count. This proves every step
  // up to the first payment worked. Switching
  // the automation to Run Immediately happens after that payment and leaves
  // no trace on the server, so it stays a hand tick.
  const auto = (id: StepId) => paymentArrived && id !== 'immediate'
  const done = (id: StepId) => (id !== 'immediate' || paymentArrived) && (auto(id) || ticks.includes(id))
  const current = STEPS.find((id) => !done(id)) ?? null

  // Connecting (the shortcut's check-in) and the first payment are confirmed
  // by the server, so while either is the open step the page keeps asking —
  // and asks again the moment the user switches back from the Shortcuts app.
  const { refetch: refetchQueue } = queue
  const { refetch: refetchSources } = sources
  const { mutate: discard } = discardEvent
  useEffect(() => {
    if (!sourceId || (current !== 'configure' && current !== 'payment')) {
      return
    }
    let stopped = false
    const check = async () => {
      if (document.visibilityState !== 'visible') {
        return
      }
      const [q, s] = await Promise.all([refetchQueue(), refetchSources()])
      if (stopped) {
        return
      }
      const tapped = (s.data ?? []).some((src) => src.id === sourceId && src.cards.some((card) => card.tapCount > 0))
      if (current === 'configure') {
        const failed = (q.data?.failed ?? []).filter((e) => e.sourceId === sourceId)
        const queued = [...(q.data?.queued ?? []), ...(q.data?.skipped ?? [])].some((e) => e.sourceId === sourceId)
        for (const e of failed.filter((f) => f.error === NO_INPUT_ERROR)) {
          discard(e.eventId)
        }
        if (failed.length > 0 || queued || tapped) {
          stopped = true
          trackEvent(METRICS.IMPORT_SHORTCUT_CHECK, { step: 'configure', ok: true })
          tick('configure', true)
        }
      } else if (tapped) {
        stopped = true
        trackEvent(METRICS.IMPORT_SHORTCUT_CHECK, { step: 'payment', ok: true })
      }
    }
    void check()
    const timer = window.setInterval(() => void check(), POLL_MS)
    const onVisible = () => void check()
    document.addEventListener('visibilitychange', onVisible)
    return () => {
      stopped = true
      window.clearInterval(timer)
      document.removeEventListener('visibilitychange', onVisible)
    }
  }, [sourceId, current, refetchQueue, refetchSources, discard, tick])

  return {
    done,
    handTicked: (id) => done(id) && ticks.includes(id) && !auto(id),
    current,
    doneCount: STEPS.filter(done).length,
    tick,
    configureLink,
    setConfigureLink,
  }
}
