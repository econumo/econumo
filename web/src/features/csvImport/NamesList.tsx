import { useId, useState } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { pluralPick } from '@/lib/plural'

export const NAMES_SHOWN = 8

export interface NamesListItem {
  key: string
  name: string
  /** transactions that name it; left out when not known yet */
  count?: number
  status: ReactNode
  action?: ReactNode
  below?: ReactNode
}

/** The names a column of the file holds and what the import does with each. */
export function NamesList({ heading, items, limit }: { heading: ReactNode; items: NamesListItem[]; limit?: number }) {
  const { t, i18n } = useTranslation()
  const headingId = useId()
  const [expanded, setExpanded] = useState(false)
  if (items.length === 0) return null
  const cut = limit !== undefined && !expanded && items.length > limit
  const shown = cut ? items.slice(0, limit) : items
  return (
    <div className="flex flex-col gap-2">
      <div id={headingId} className="text-xs text-muted-foreground">
        {heading}
      </div>
      <ul aria-labelledby={headingId} className="divide-y divide-border rounded-xl border border-border">
        {shown.map((item) => (
          <li key={item.key} className="flex flex-col gap-2 px-4 py-3">
            <div className="flex flex-col gap-1 sm:flex-row sm:items-start sm:gap-3">
              <p className="min-w-0 flex-1 text-sm break-words">
                {item.name}
                {item.count !== undefined ? (
                  <span className="text-muted-foreground">
                    {` · ${pluralPick(t('transactions.import_wizard.account.transactions'), item.count, i18n.language)}`}
                  </span>
                ) : null}
              </p>
              <div className="flex items-center gap-3 text-sm sm:justify-end">
                <span>
                  {' '}
                  {item.status}
                </span>
                {item.action}
              </div>
            </div>
            {item.below}
          </li>
        ))}
      </ul>
      {cut ? (
        <button
          type="button"
          className="self-start rounded-md text-xs text-muted-foreground hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none"
          onClick={() => setExpanded(true)}
        >
          {t('transactions.import_wizard.names_more', { count: items.length - (limit ?? 0) })}
        </button>
      ) : null}
    </div>
  )
}
