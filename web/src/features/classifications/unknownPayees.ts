import type { QueryClient } from '@tanstack/react-query'
import type { PayeeDto } from '@/api/dto/payee'
import type { Id } from '@/api/types'
import { queryKeys } from '@/app/queryKeys'

// Ids a refetch was already asked for: one the list still lacks afterwards (a payee deleted since,
// or one outside this user's list) must not refetch it on every sync.
const asked = new Set<Id>()

export function resetUnknownPayees(): void {
  asked.clear()
}

// The server creates payees this device never posted (the AI import job's new payees), so a row or
// a suggestion may name a payee the cached list lacks: the list is refetched then, once per id.
export function refetchPayeesIfUnknown(queryClient: QueryClient, ids: Iterable<Id | null | undefined>): void {
  const payees = queryClient.getQueryData<PayeeDto[]>(queryKeys.payees)
  if (!payees) return
  const known = new Set(payees.map((p) => p.id))
  let missing = false
  for (const id of ids) {
    if (id && !known.has(id) && !asked.has(id)) {
      asked.add(id)
      missing = true
    }
  }
  if (missing) void queryClient.invalidateQueries({ queryKey: queryKeys.payees })
}
