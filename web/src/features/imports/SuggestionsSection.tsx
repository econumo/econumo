import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import type { ImportSuggestionDto } from '@/api/dto/imports'
import { useUiStore } from '@/app/uiStore'
import { Button } from '@/components/ui/button'
import { useOnline } from '@/hooks/useOnline'
import { useCategories, usePayees } from '@/features/classifications/queries'
import { apiErrorMessage } from '@/lib/apiError'
import { pluralPick } from '@/lib/plural'
import { useDismissImportSuggestion } from './queries'

export function SuggestionsSection({ suggestions }: { suggestions: ImportSuggestionDto[] }) {
  const { t, i18n } = useTranslation()
  const online = useOnline()
  const setRulePrompt = useUiStore((s) => s.setRulePrompt)
  const { data: categories = [] } = useCategories()
  const { data: payees = [] } = usePayees()
  const dismiss = useDismissImportSuggestion()

  const setsText = (s: ImportSuggestionDto) => {
    const category = categories.find((c) => c.id === s.categoryId)?.name
    const payee = payees.find((p) => p.id === s.payeeId)?.name
    if (category && payee) return t('imports.suggestions.set_both', { category, payee })
    if (category) return t('imports.suggestions.set_category', { category })
    if (payee) return t('imports.suggestions.set_payee', { payee })
    return ''
  }

  return (
    <div className="flex flex-col gap-2">
      <p className="px-1 text-sm text-muted-foreground">{t('imports.suggestions.hint')}</p>
      {suggestions.map((s) => {
        const sets = setsText(s)
        return (
          <div key={s.id} className="flex items-center gap-3 rounded-lg bg-econumo-card px-4 py-3 text-sm">
            <button type="button" className="min-w-0 flex-1 text-left" onClick={() => setRulePrompt({ kind: 'suggestion', suggestion: s })}>
              <span className="block truncate">
                {t(`imports.rules.field.${s.matchField}`)} {t('imports.rules.type.contains')} &quot;{s.matchText}&quot;
              </span>
              {sets ? <span className="block truncate text-muted-foreground">{sets}</span> : null}
              <span className="block text-xs text-muted-foreground">
                {pluralPick(t('imports.suggestions.transactions'), s.transactions.length, i18n.language)}
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
