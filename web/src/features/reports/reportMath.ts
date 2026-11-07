import type { NetWorthAccountDto, NetWorthReportDto } from '@/api/dto/report'
import type { Id } from '@/api/types'
import { add } from '@/lib/decimal'

export function netWorthSeries(
  report: NetWorthReportDto,
  groupOf: (a: NetWorthAccountDto) => { id: string; name: string },
): { groups: { id: string; name: string; balances: string[] }[]; total: string[] } {
  const zeros = () => report.months.map(() => '0')
  const groups = new Map<string, { id: string; name: string; balances: string[] }>()
  const total = zeros()
  for (const acc of report.accounts) {
    const { id, name } = groupOf(acc)
    const g = groups.get(id) ?? { id, name, balances: zeros() }
    groups.set(id, g)
    report.months.forEach((_, i) => {
      const v = acc.balances[i] ?? '0'
      g.balances[i] = add(g.balances[i], v)
      total[i] = add(total[i], v)
    })
  }
  return { groups: [...groups.values()], total }
}

export interface NetWorthGrouping {
  me: Id
  /** my accounts' folder, from my account list */
  folderOf: Record<Id, Id | null>
  folders: { id: Id; name: string }[]
  /** budget participants' names by user id */
  owners: Record<Id, string>
  names: { noFolder: string; otherOwner: string }
}

// My accounts sit in my folders; anyone else's account is listed under its
// owner's name, since their folders are theirs and are never shown.
export function netWorthGroupOf(g: NetWorthGrouping) {
  const folderName = new Map(g.folders.map((f) => [f.id, f.name]))
  return (a: NetWorthAccountDto): { id: string; name: string } => {
    if (a.ownerUserId !== g.me) return { id: `owner:${a.ownerUserId}`, name: g.owners[a.ownerUserId] ?? g.names.otherOwner }
    const folderId = g.folderOf[a.id]
    const name = folderId ? folderName.get(folderId) : undefined
    return folderId && name !== undefined ? { id: `folder:${folderId}`, name } : { id: 'nofolder', name: g.names.noFolder }
  }
}
