import { useEffect, useRef, useState } from 'react'
import { suggestIcon, type SuggestIconParams } from '@/api/onboarding'
import { useOnline } from '@/hooks/useOnline'
import { isAiEnabled } from '@/lib/config'
import { METRICS, trackEvent } from '@/lib/metrics'
import { getSessionUserId } from '@/lib/storage'
import { answers } from './answers'

export type IconSuggestionOptions = SuggestIconParams & {
  // false once the user picked an icon, or while the name is not theirs to rename
  enabled: boolean
}

const DEBOUNCE_MS = 600
const NAME_MIN = 2
const NAME_MAX = 64

// The icon the server's AI suggests for a name being typed, once the typing has paused; null
// while there is none. Without an AI provider (or offline) it asks nothing at all.
export function useIconSuggestion({ kind, name, type, currency, enabled }: IconSuggestionOptions): string | null {
  const online = useOnline()
  const trimmed = name.trim()
  const length = [...trimmed].length
  const key =
    enabled && online && isAiEnabled() && length >= NAME_MIN && length <= NAME_MAX
      ? [getSessionUserId() ?? '', kind, type ?? '', currency ?? '', trimmed.toLowerCase()].join('\0')
      : null
  const [settled, setSettled] = useState<string | null>(null)

  useEffect(() => {
    if (!key) return
    const controller = new AbortController()
    const timer = setTimeout(() => {
      if (answers.has(key)) {
        setSettled(key)
        return
      }
      suggestIcon({ kind, name: trimmed, ...(type ? { type } : {}), ...(currency ? { currency } : {}) }, controller.signal)
        .then((icon) => {
          // "" may be a passing refusal (the limit, a slow provider): asked again next time,
          // which the server answers from its own cache when it was final
          if (icon) answers.set(key, icon)
          if (!controller.signal.aborted) setSettled(key)
        })
        // a failed request is no suggestion; typing never shows an error
        .catch(() => undefined)
    }, DEBOUNCE_MS)
    return () => {
      clearTimeout(timer)
      controller.abort()
    }
  }, [key, kind, trimmed, type, currency])

  return key !== null && settled === key ? answers.get(key) || null : null
}

// where a suggestion was applied, for the metric
export type IconSuggestionSource = 'dialog' | 'onboarding' | 'inline'

// Applies a suggestion through onApply while the hook is enabled; the metric fires the first
// time a suggestion changes the item's icon. `current` is the icon shown now. An icon put there
// for an earlier name gives way to the item's starting icon as soon as the name changes, until
// the new name has a suggestion of its own; an icon set any other way is never touched.
export function useApplyIconSuggestion(
  options: IconSuggestionOptions,
  current: string,
  onApply: (icon: string) => void,
  source: IconSuggestionSource,
): void {
  const suggestion = useIconSuggestion(options)
  const apply = useRef(onApply)
  const reported = useRef(false)
  // the icon this hook put there (while it is still shown) and the one it replaced
  const applied = useRef<string | null>(null)
  const start = useRef<string | null>(null)
  const subject = [options.kind, options.type ?? '', options.currency ?? '', options.name.trim().toLowerCase()].join('\0')
  useEffect(() => {
    apply.current = onApply
  })
  useEffect(() => {
    if (!options.enabled) {
      reported.current = false
      applied.current = null
      start.current = null
      return
    }
    if (applied.current !== null && current !== applied.current) {
      // changed by other means since: no longer ours to change back
      applied.current = null
      start.current = null
    }
    if (suggestion) {
      if (suggestion === current) return
      if (applied.current === null) start.current = current
      applied.current = suggestion
      apply.current(suggestion)
      if (!reported.current) {
        reported.current = true
        trackEvent(METRICS.ICON_SUGGESTION_APPLIED, { kind: options.kind, source })
      }
    } else if (applied.current !== null && start.current !== null) {
      apply.current(start.current)
      applied.current = null
      start.current = null
    }
    // runs when the name or the suggestion moves, not when the icon does because of it
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [subject, suggestion, options.enabled])
}

// useApplyIconSuggestion for an onboarding row, rendered by a callback where a hook cannot be called.
export function IconSuggestion({
  current,
  onApply,
  ...options
}: IconSuggestionOptions & { current: string; onApply: (icon: string) => void }) {
  useApplyIconSuggestion(options, current, onApply, 'onboarding')
  return null
}
