import { useTranslation } from 'react-i18next'
import type { ColumnProfile } from '../columns'

interface PlainColumnPickerProps {
  profiles: ColumnProfile[]
  value: number | null
  onChange: (column: number | null) => void
  label: string
}

const NONE = ''

export function PlainColumnPicker({ profiles, value, onChange, label }: PlainColumnPickerProps) {
  const { t } = useTranslation()
  return (
    <select
      aria-label={label}
      className="h-9 w-full rounded-md border bg-transparent px-2 text-sm"
      value={value === null ? NONE : String(value)}
      onChange={(e) => onChange(e.target.value === NONE ? null : Number(e.target.value))}
    >
      <option value={NONE}>{t('transactions.import_wizard.none')}</option>
      {profiles.map((p) => (
        <option key={p.index} value={String(p.index)}>
          {p.samples.length > 0 ? `${p.name}: ${p.samples.join(', ')}` : p.name}
        </option>
      ))}
    </select>
  )
}
