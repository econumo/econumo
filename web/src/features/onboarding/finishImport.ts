import type { Query, QueryClient } from '@tanstack/react-query'
import { isAxiosError } from 'axios'
import { v7 as uuidv7 } from 'uuid'
import { finishOnboardingImport } from '@/api/onboarding'
import type { CurrentUserDto } from '@/api/dto/user'
import { queryKeys } from '@/app/queryKeys'
import { isAiEnabled } from '@/lib/config'
import { userAiOn } from '@/features/user/queries'
import { METRICS, trackEvent } from '@/lib/metrics'
import { emptyDraft, loadDraft, saveDraft } from './draft'
import { onboardingProgress, storedStepState } from './onboardingProgress'

// when, after a CSV import or finish-import, the lists are refetched to show the AI's icons and savings (a job may take up to 90 s)
export const AI_REFRESH_DELAYS_MS = [10_000, 30_000, 90_000]

export const notUser = (q: Query) => q.queryKey[0] !== queryKeys.user[0]

// A 400 on `id`: the operation id was claimed by another write.
export const claimedElsewhere = (err: unknown) =>
  isAxiosError(err) && err.response?.status === 400 && Boolean((err.response.data as { errors?: Record<string, unknown> } | undefined)?.errors?.id)

let aiRefresh: ReturnType<typeof setTimeout>[] = []

export function cancelAiRefresh(): void {
  aiRefresh.forEach(clearTimeout)
  aiRefresh = []
}

// Module-level, not tied to a component: the import may come from the Settings data page, and
// finishing onboarding within 90 s must not stop the icons from showing up. Without AI (or with
// the user's AI option off) there is nothing to wait for; a logout or another user signing in (a
// different cached user) ends it.
export function scheduleAiRefresh(queryClient: QueryClient): void {
  cancelAiRefresh()
  const user = queryClient.getQueryData<CurrentUserDto>(queryKeys.user)
  if (!isAiEnabled() || !user || !userAiOn(user)) return
  const userId = user.id
  aiRefresh = AI_REFRESH_DELAYS_MS.map((delay) =>
    setTimeout(() => {
      if (queryClient.getQueryData<CurrentUserDto>(queryKeys.user)?.id !== userId) {
        cancelAiRefresh()
        return
      }
      for (const queryKey of [queryKeys.categories, queryKeys.accounts, queryKeys.budgets, queryKeys.budget, queryKeys.budgetPlan]) {
        void queryClient.invalidateQueries({ queryKey })
      }
    }, delay),
  )
}

let inFlight: Promise<boolean> | null = null

// The one path that settles the import step while onboarding is in progress: the server marks
// `import`, creates the budget when there is none and starts the AI savings job. The client never
// writes `import` through update-onboarding. Resolves false when there was nothing to settle
// (onboarding over, or the step already done); concurrent callers share one request, and the
// operation id lives in the Day 0 draft so a retry after a lost answer replays it.
export function finishImportStep(queryClient: QueryClient): Promise<boolean> {
  inFlight ??= run(queryClient).finally(() => {
    inFlight = null
  })
  return inFlight
}

async function run(queryClient: QueryClient): Promise<boolean> {
  const user = queryClient.getQueryData<CurrentUserDto>(queryKeys.user)
  const option = onboardingProgress(user)
  if (!user || option === null || storedStepState(option, 'import') === 'done') return false
  const draft = loadDraft(user.id) ?? emptyDraft()
  const opId = (draft.ops.import ??= uuidv7())
  saveDraft(user.id, draft)
  let result
  try {
    result = await finishOnboardingImport(opId)
  } catch (err) {
    // the step may have committed with its answer lost: the refetched user shows it done
    void queryClient.invalidateQueries()
    if (claimedElsewhere(err)) {
      const d = loadDraft(user.id) ?? emptyDraft()
      delete d.ops.import
      saveDraft(user.id, d)
    }
    throw err
  }
  await queryClient.cancelQueries({ queryKey: queryKeys.user })
  queryClient.setQueryData(queryKeys.user, result.user)
  const done = loadDraft(user.id) ?? emptyDraft()
  done.budgetId = result.budgetId || done.budgetId
  delete done.ops.import
  saveDraft(user.id, done)
  void queryClient.invalidateQueries({ predicate: notUser })
  trackEvent(METRICS.ONBOARDING_IMPORT_FINISHED, { budget: result.budgetId !== '' })
  scheduleAiRefresh(queryClient)
  return true
}
