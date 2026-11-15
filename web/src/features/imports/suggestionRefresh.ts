import type { QueryClient } from '@tanstack/react-query'
import { queryKeys } from '@/app/queryKeys'
import { isAiEnabled } from '@/lib/config'

// when, after a sync, the suggestion list is refetched for the AI job's answer
export const SUGGESTION_REFRESH_DELAYS_MS = [15_000, 60_000]

let suggestionRefresh: ReturnType<typeof setTimeout>[] = []

export function cancelSuggestionRefresh(): void {
  suggestionRefresh.forEach(clearTimeout)
  suggestionRefresh = []
}

// Module-level, like finishImport's AI refetches: leaving the page that ran the sync must not
// stop the suggestions from showing up.
export function scheduleSuggestionRefresh(queryClient: QueryClient): void {
  cancelSuggestionRefresh()
  if (!isAiEnabled()) return
  suggestionRefresh = SUGGESTION_REFRESH_DELAYS_MS.map((delay) =>
    setTimeout(() => void queryClient.invalidateQueries({ queryKey: queryKeys.importSuggestions }), delay),
  )
}
