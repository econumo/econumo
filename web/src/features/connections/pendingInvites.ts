import { useQuery } from '@tanstack/react-query'
import * as accountApi from '@/api/account'
import * as budgetApi from '@/api/budget'
import type { Id } from '@/api/types'
import type { UserDto } from '@/api/dto/user'
import { queryKeys, TEN_MINUTES } from '@/app/queryKeys'
import { useUserData } from '@/features/user/queries'

export interface PendingInvite {
  kind: 'account' | 'budget'
  id: Id
  name: string
  owner: UserDto
  role: string
}

/** Pending share invites for the current user, derived from the account and
 *  budget list caches (raw, unfiltered queries on the same keys as
 *  useAccounts/useBudgets, whose `select` filters pending invites OUT). */
export function usePendingInvites(): { invites: PendingInvite[]; count: number } {
  const { data: user } = useUserData()
  const meId = user?.id
  const { data: accounts } = useQuery({
    queryKey: queryKeys.accounts,
    queryFn: accountApi.getAccountList,
    staleTime: TEN_MINUTES,
    enabled: !!meId,
  })
  const { data: budgets } = useQuery({
    queryKey: queryKeys.budgets,
    queryFn: budgetApi.getBudgetList,
    staleTime: TEN_MINUTES,
    enabled: !!meId,
  })

  const invites: PendingInvite[] = []
  for (const a of accounts ?? []) {
    if (a.owner.id === meId) continue
    const mine = a.sharedAccess.find((s) => s.user.id === meId && s.isAccepted === 0)
    if (mine) {
      invites.push({ kind: 'account', id: a.id, name: a.name, owner: a.owner, role: mine.role })
    }
  }
  for (const b of budgets ?? []) {
    if (b.ownerUserId === meId) continue
    const mine = b.access.find((s) => s.user.id === meId && s.isAccepted === 0)
    if (mine) {
      const owner = b.access.find((s) => s.user.id === b.ownerUserId)?.user
      invites.push({ kind: 'budget', id: b.id, name: b.name, owner: owner ?? { id: b.ownerUserId, name: '', avatar: '' }, role: mine.role })
    }
  }
  return { invites, count: invites.length }
}
