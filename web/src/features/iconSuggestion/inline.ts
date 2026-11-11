import { onlineManager, type QueryClient } from '@tanstack/react-query'
import type { CategoryDto } from '@/api/dto/category'
import { applySuggestedIcon, suggestIcon } from '@/api/onboarding'
import { queryKeys } from '@/app/queryKeys'
import { isAiEnabled } from '@/lib/config'
import { METRICS, trackEvent } from '@/lib/metrics'

// A category created inline from a picker ("Create <name>") is saved at once on the default
// icon; afterwards, in the background, it takes the icon the AI suggests for its name, but only
// while it still shows the icon it was created with (a change made meanwhile wins; the server
// checks). Nothing is shown either way, a failure included.
export async function suggestIconForCreatedCategory(queryClient: QueryClient, item: CategoryDto): Promise<void> {
  if (!isAiEnabled() || !onlineManager.isOnline()) return
  try {
    const icon = await suggestIcon({ kind: 'category', name: item.name, type: item.type })
    if (!icon || icon === item.icon) return
    // the server writes only while the category still shows its first icon, so a change made
    // meanwhile (on any device) wins, and the name is never sent back
    const applied = await applySuggestedIcon({ categoryId: item.id, icon, expectedIcon: item.icon })
    if (applied) {
      queryClient.setQueryData<CategoryDto[]>(queryKeys.categories, (prev) => (prev ?? []).map((c) => (c.id === item.id ? { ...c, icon } : c)))
    }
    void queryClient.invalidateQueries({ queryKey: queryKeys.categories })
    if (!applied) return
    // the budget views and debts show the category's icon
    void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
    void queryClient.invalidateQueries({ queryKey: queryKeys.budgetPlan })
    void queryClient.invalidateQueries({ queryKey: queryKeys.debts })
    trackEvent(METRICS.ICON_SUGGESTION_APPLIED, { kind: 'category', source: 'inline' })
  } catch {
    // the category keeps its icon
  }
}
