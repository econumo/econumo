import { useState } from 'react'
import { Plus, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { CommandEmpty, CommandItem } from '@/components/ui/command'
import { PickerSheet } from '@/components/PickerSheet'
import { fullCurrencyLabel } from '@/components/CurrencySelect'
import { rankByName } from '@/lib/search'
import type { CurrencyListItemDto } from '@/api/dto/currency'
import type { Id } from '@/api/types'

interface CurrencyListSheetProps {
  open: boolean
  title: string
  options: CurrencyListItemDto[]
  value?: Id | null
  onClose: () => void
  onPick: (id: Id) => void
}

export function CurrencyListSheet({ open, title, options, value, onClose, onPick }: CurrencyListSheetProps) {
  const { t } = useTranslation()
  return (
    <PickerSheet open={open} onClose={onClose} title={title} searchLabel={t('accounts.page.toolbar.search')} currentValue={value ?? undefined}>
      {(search) => (
        <>
          <CommandEmpty />
          {rankByName(options, (c) => [c.name, c.code, c.symbol], search).map((c) => (
            <CommandItem
              key={c.id}
              value={c.id}
              data-checked={c.id === value}
              onSelect={() => {
                onPick(c.id)
                onClose()
              }}
            >
              <span className="flex-1">{fullCurrencyLabel(c)}</span>
            </CommandItem>
          ))}
        </>
      )}
    </PickerSheet>
  )
}

interface CurrenciesBlockProps {
  currencies: CurrencyListItemDto[]
  selectableOptions: CurrencyListItemDto[]
  usedIds: Id[]
  extraIds: Id[]
  mainId: Id | null
  missingRateIds: Id[]
  onAddExtra: (id: Id) => void
  onRemoveExtra: (id: Id) => void
  onMainChange: (id: Id) => void
}

export function CurrenciesBlock({
  currencies,
  selectableOptions,
  usedIds,
  extraIds,
  mainId,
  missingRateIds,
  onAddExtra,
  onRemoveExtra,
  onMainChange,
}: CurrenciesBlockProps) {
  const { t } = useTranslation()
  const [adding, setAdding] = useState(false)
  const [pickingMain, setPickingMain] = useState(false)
  const byId = new Map(currencies.map((c) => [c.id, c]))
  const code = (id: Id) => byId.get(id)?.code ?? ''
  const selected = [...usedIds, ...extraIds.filter((id) => !usedIds.includes(id))]
  const addable = selectableOptions.filter((c) => !selected.includes(c.id))
  const missingCodes = missingRateIds.map(code).filter(Boolean).join(', ')

  return (
    <section className="space-y-3 rounded-xl border bg-card p-4">
      <h2 className="text-lg text-muted-foreground">{t('onboarding.currencies.title')}</h2>
      <div className="flex flex-wrap items-center gap-2">
        {usedIds.map((id) => (
          <span key={id} className="rounded-full bg-muted px-3 py-1 text-sm">
            {code(id)}
          </span>
        ))}
        {extraIds
          .filter((id) => !usedIds.includes(id))
          .map((id) => (
            <span key={id} className="inline-flex items-center gap-1 rounded-full bg-muted py-1 pr-1 pl-3 text-sm">
              {code(id)}
              <button
                type="button"
                aria-label={t('onboarding.currencies.remove', { code: code(id) })}
                className="rounded-full p-1 text-muted-foreground hover:bg-background"
                onClick={() => onRemoveExtra(id)}
              >
                <X className="size-3" />
              </button>
            </span>
          ))}
        <Button type="button" variant="ghost" size="sm" onClick={() => setAdding(true)}>
          <Plus className="size-4" />
          {t('onboarding.currencies.add')}
        </Button>
      </div>
      {mainId && (
        <button type="button" className="text-left text-sm text-muted-foreground underline-offset-4 hover:underline" onClick={() => setPickingMain(true)}>
          {t('onboarding.currencies.main', { code: code(mainId) })}
        </button>
      )}
      <p className="text-sm text-muted-foreground">{t('onboarding.currencies.note')}</p>
      {missingCodes && (
        <p role="note" className="text-sm text-amber-600 dark:text-amber-400">
          {t('onboarding.currencies.no_rate', { codes: missingCodes })}
        </p>
      )}
      <CurrencyListSheet open={adding} title={t('onboarding.currencies.add')} options={addable} onClose={() => setAdding(false)} onPick={onAddExtra} />
      <CurrencyListSheet
        open={pickingMain}
        title={t('onboarding.currencies.main_title')}
        options={selected.map((id) => byId.get(id)).filter((c): c is CurrencyListItemDto => !!c)}
        value={mainId}
        onClose={() => setPickingMain(false)}
        onPick={onMainChange}
      />
    </section>
  )
}
