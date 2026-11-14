import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import type { ImportSuggestionDto } from '@/api/dto/imports'
import { useUiStore } from '@/app/uiStore'
import { Button } from '@/components/ui/button'
import { useOnline } from '@/hooks/useOnline'
import { useCategories, usePayees } from '@/features/classifications/queries'
import { apiErrorMessage } from '@/lib/apiError'
import { formatDate, parseDateTime } from '@/lib/datetime'
import { moneyFormat } from '@/lib/money'
import { pluralPick } from '@/lib/plural'
import { useDismissImportSuggestion } from './queries'

export function SuggestionsSection({ suggestions }: { suggestions: ImportSuggestionDto[] }) {
  const { t, i18n } = useTranslation()
  const online = useOnline()
  const setRulePrompt = useUiStore((s) => s.setRulePrompt)
  const { data: categories = [] } = useCategories()
  const { data: payees = [] } = usePayees()
  const dismiss = useDismissImportSuggestion()

  const target = (s: ImportSuggestionDto) =>
    [categories.find((c) => c.id === s.categoryId)?.name, payees.find((p) => p.id === s.payeeId)?.name].filter(Boolean).join(' · ')

  return (
    <div className="flex flex-col gap-2">
      {suggestions.map((s) => {
        const names = target(s)
        const newest = s.transactions[0]
        return (
          <div key={s.id} className="flex items-center gap-3 rounded-lg bg-econumo-card px-4 py-3 text-sm">
            <button type="button" className="min-w-0 flex-1 text-left" onClick={() => setRulePrompt({ kind: 'suggestion', suggestion: s })}>
              <span className="block truncate">{s.matchText}</span>
              {names ? <span className="block truncate text-muted-foreground">→ {names}</span> : null}
              <span className="block text-xs text-muted-foreground">
                {pluralPick(t('imports.suggestions.transactions'), s.transactions.length, i18n.language)}
                {newest
                  ? ` · ${moneyFormat(newest.amount, null, { showCurrency: false, useNativePrecision: false })} ${newest.currency} · ${formatDate(parseDateTime(newest.date))}`
                  : ''}
              </span>
            </button>
            <Button
              type="button"
              size="sm"
              variant="ghost"
              aria-label={`${t('imports.suggestions.dismiss')} ${s.matchText}`}
              disabled={!online || (dismiss.isPending && dismiss.variables === s.id)}
              onClick={() => dismiss.mutate(s.id, { onError: (err) => toast.error(apiErrorMessage(err)) })}
            >
              {t('imports.suggestions.dismiss')}
            </Button>
          </div>
        )
      })}
    </div>
  )
}
