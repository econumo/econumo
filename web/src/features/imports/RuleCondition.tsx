import type { ReactNode } from 'react'
import { ChevronDown } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { ImportRuleMatchField, ImportRuleMatchType } from '@/api/dto/imports'
import { cardFieldControlClass } from '@/components/CardField'
import { Input } from '@/components/ui/input'

interface RuleConditionProps {
  id: string
  matchField: ImportRuleMatchField
  matchType: ImportRuleMatchType
  matchValue: string
  /** the value input's accessible name */
  valueLabel: string
  onChange: (patch: { matchField?: ImportRuleMatchField; matchType?: ImportRuleMatchType; matchValue?: string }) => void
  /** offered only where the rule is edited in full */
  caseSensitive?: { value: boolean; onChange: (value: boolean) => void }
}

// The rule's condition reads as one sentence, "Payee contains STARBUCKS":
// the two choices sit as pills above the text they apply to.
export function RuleCondition({ id, matchField, matchType, matchValue, valueLabel, onChange, caseSensitive }: RuleConditionProps) {
  const { t } = useTranslation()
  return (
    <div className="flex flex-col gap-2 rounded-lg bg-econumo-card px-4 py-3 focus-within:ring-1 focus-within:ring-ring">
      <div className="flex flex-wrap items-center gap-2">
        <PillSelect aria-label={t('imports.rules.editor.field')} value={matchField} onChange={(v) => onChange({ matchField: v as ImportRuleMatchField })}>
          <option value="external_payee">{t('imports.rules.field.external_payee')}</option>
          <option value="description">{t('imports.rules.field.description')}</option>
        </PillSelect>
        <PillSelect aria-label={t('imports.rules.editor.type')} value={matchType} onChange={(v) => onChange({ matchType: v as ImportRuleMatchType })}>
          <option value="contains">{t('imports.rules.type.contains')}</option>
          <option value="prefix">{t('imports.rules.type.prefix')}</option>
          <option value="exact">{t('imports.rules.type.exact')}</option>
        </PillSelect>
      </div>
      <div className="flex items-center gap-2">
        <Input id={id} aria-label={valueLabel} placeholder={t('imports.rules.editor.value_placeholder')}
          className={`${cardFieldControlClass} text-base`} value={matchValue} maxLength={255} autoComplete="off"
          onChange={(e) => onChange({ matchValue: e.target.value })} />
        {caseSensitive ? (
          <button type="button" aria-pressed={caseSensitive.value} aria-label={t('imports.rules.editor.case_sensitive')} title={t('imports.rules.editor.case_sensitive')}
            className={`shrink-0 rounded-md px-2 py-1 text-xs transition-colors ${caseSensitive.value ? 'bg-econumo-magenta text-white' : 'text-muted-foreground hover:text-foreground'}`}
            onClick={() => caseSensitive.onChange(!caseSensitive.value)}>
            Aa
          </button>
        ) : null}
      </div>
    </div>
  )
}

function PillSelect({ value, onChange, children, 'aria-label': ariaLabel }: { value: string; onChange: (value: string) => void; children: ReactNode; 'aria-label': string }) {
  return (
    <span className="relative inline-flex">
      <select aria-label={ariaLabel} value={value} onChange={(e) => onChange(e.target.value)}
        className="cursor-pointer appearance-none rounded-md bg-background py-1 pr-7 pl-2.5 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring dark:bg-input/30">
        {children}
      </select>
      <ChevronDown className="pointer-events-none absolute top-1/2 right-2 size-3.5 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
    </span>
  )
}
