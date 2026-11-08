import { useState } from 'react'
import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
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
  const extras = extraIds.filter((id) => !usedIds.includes(id))
  const selected = [...usedIds, ...extras]
  const addable = selectableOptions.filter((c) => !selected.includes(c.id))
  const missingCodes = missingRateIds.map(code).filter(Boolean).join(', ')
  const dot = <span aria-hidden>·</span>

  return (
    <section>
      <h3 className="mt-4 mb-1.5 text-xs tracking-wider text-muted-foreground uppercase">{t('onboarding.currencies.title')}</h3>
      <div className="flex flex-wrap items-center gap-x-1.5 gap-y-1 text-sm text-muted-foreground">
        {mainId && (
          <button
            type="button"
            aria-label={t('onboarding.currencies.main_title')}
            className="underline-offset-4 hover:underline"
            onClick={() => setPickingMain(true)}
          >
            {t('onboarding.currencies.main_short', { code: code(mainId) })}
          </button>
        )}
        {usedIds
          .filter((id) => id !== mainId)
          .map((id) => (
            <span key={id} className="contents">
              {dot}
              <span>{code(id)}</span>
            </span>
          ))}
        {extras
          .filter((id) => id !== mainId)
          .map((id) => (
            <span key={id} className="contents">
              {dot}
              <span className="inline-flex items-center">
                {code(id)}
                <button
                  type="button"
                  aria-label={t('onboarding.currencies.remove', { code: code(id) })}
                  className="ml-0.5 rounded-full p-0.5 hover:bg-accent"
                  onClick={() => onRemoveExtra(id)}
                >
                  <X className="size-3" />
                </button>
              </span>
            </span>
          ))}
        {dot}
        <button type="button" aria-label={t('onboarding.currencies.add')} className="text-primary hover:underline" onClick={() => setAdding(true)}>
          + {t('onboarding.currencies.add_short')}
        </button>
      </div>
      {missingCodes && (
        <p role="note" className="mt-2 text-sm text-amber-600 dark:text-amber-400">
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
