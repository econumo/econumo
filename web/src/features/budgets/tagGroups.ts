import { BudgetElementType } from '@/api/dto/budget'

interface Groupable {
  type: BudgetElementType
  name: string
  isArchived: 0 | 1
}

/** A run of a folder's rows: one row, or the same-name tags gathered under a group
 *  line. Partners in a shared budget each keep their own tag ("Travel" and
 *  "Travel"), and the group line is where their combined amounts read. */
export type RowSegment<T> = { kind: 'row'; item: T } | { kind: 'group'; key: string; name: string; items: T[] }

const nameKey = (name: string): string => name.trim().toLocaleLowerCase()

/** the group's fold key: per folder, so the same name in two folders folds apart */
export const tagGroupKey = (scope: string | null, name: string): string => `taggroup:${scope ?? ''}:${nameKey(name)}`

/** Tags sharing a name (trimmed, case-insensitive) form a group at the place of the
 *  first of them; everything else keeps its own place. Archived tags group only with
 *  archived ones, live with live. */
export function segmentTagGroups<T>(items: T[], of: (item: T) => Groupable, scope: string | null): RowSegment<T>[] {
  const groupable = (item: T) => of(item).type === BudgetElementType.TAG
  const memberKey = (item: T) => `${of(item).isArchived}:${nameKey(of(item).name)}`
  const byName = new Map<string, T[]>()
  for (const item of items) {
    if (groupable(item)) {
      const key = memberKey(item)
      byName.set(key, [...(byName.get(key) ?? []), item])
    }
  }
  const segments: RowSegment<T>[] = []
  const placed = new Set<string>()
  for (const item of items) {
    const members = groupable(item) ? byName.get(memberKey(item))! : null
    if (!members || members.length < 2) {
      segments.push({ kind: 'row', item })
      continue
    }
    if (!placed.has(memberKey(item))) {
      placed.add(memberKey(item))
      const key = tagGroupKey(scope, of(item).name)
      segments.push({ kind: 'group', key, name: of(members[0]).name.trim(), items: members })
    }
  }
  return segments
}

/** the rows in the order the groups show them: a group's tags next to each other.
 *  Bucketing applies it, so drag positions and keyboard order follow the screen. */
export function withTagGroupsAdjacent<T>(items: T[], of: (item: T) => Groupable): T[] {
  return segmentTagGroups(items, of, null).flatMap((s) => (s.kind === 'row' ? [s.item] : s.items))
}
