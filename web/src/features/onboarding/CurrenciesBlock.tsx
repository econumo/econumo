import { useState } from 'react'
import { ChevronDown, Plus, X } from 'lucide-react'
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
  // a last item that opens the full list, for a list limited to some currencies
  onAddMore?: () => void
}

export function CurrencyListSheet({ open, title, options, value, onClose, onPick, onAddMore }: CurrencyListSheetProps) {
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
          {onAddMore && (
            <CommandItem
              value="__add"
              className="text-primary"
              onSelect={() => {
                onClose()
                onAddMore()
              }}
            >
              <Plus className="size-4" />
              <span className="flex-1">{t('onboarding.currencies.add')}</span>
            </CommandItem>
          )}
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
  onAddExtra,
  onRemoveExtra,
  onMainChange,
}: CurrenciesBlockProps) {
  const { t } = useTranslation()
  // 'extra' adds a currency to the list; 'main' also makes it the main one
  const [adding, setAdding] = useState<'extra' | 'main' | null>(null)
  const [pickingMain, setPickingMain] = useState(false)
  const byId = new Map(currencies.map((c) => [c.id, c]))
  const code = (id: Id) => byId.get(id)?.code ?? ''
  const extras = extraIds.filter((id) => !usedIds.includes(id))
  const selected = [...usedIds, ...extras]
  const addable = selectableOptions.filter((c) => !selected.includes(c.id))
  const main = mainId ? byId.get(mainId) : undefined
  const chipClass = 'inline-flex h-7 items-center gap-0.5 rounded-md border px-2 text-xs'

  return (
    <section>
      <h3 className="mt-4 mb-1 text-xs tracking-wider text-muted-foreground uppercase">{t('onboarding.currencies.title')}</h3>
      <p className="mb-2 text-sm text-muted-foreground">{t('onboarding.currencies.hint')}</p>
      <div className="flex items-center justify-between gap-3">
        <span className="text-sm">
          {t('onboarding.currencies.main_title')}
        </span>
        <button
          type="button"
          aria-label={t('onboarding.currencies.main_title')}
          aria-haspopup="dialog"
          className="flex h-9 min-w-0 max-w-[60%] items-center gap-2 rounded-lg border bg-transparent px-3 text-sm hover:bg-accent focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none"
          onClick={() => setPickingMain(true)}
        >
          <span className="truncate">{main ? fullCurrencyLabel(main) : t('onboarding.currencies.main_pick')}</span>
          <ChevronDown className="size-4 shrink-0 text-muted-foreground" />
        </button>
      </div>
      <ul className="mt-2 flex flex-wrap items-center gap-1.5" aria-label={t('onboarding.currencies.selected')}>
        {selected.map((id) => (
          <li key={id} className={chipClass}>
            {code(id)}
            {extras.includes(id) && id !== mainId && (
              <button
                type="button"
                aria-label={t('onboarding.currencies.remove', { code: code(id) })}
                className="-mr-1 rounded-full p-0.5 text-muted-foreground hover:bg-accent hover:text-foreground"
                onClick={() => onRemoveExtra(id)}
              >
                <X className="size-3" />
              </button>
            )}
          </li>
        ))}
        <li>
          <button type="button" className={`${chipClass} border-dashed text-primary hover:bg-accent`} onClick={() => setAdding('extra')}>
            <Plus className="size-3" />
            {t('onboarding.currencies.add')}
          </button>
        </li>
      </ul>
      <CurrencyListSheet
        open={adding !== null}
        title={t('onboarding.currencies.add')}
        options={addable}
        onClose={() => setAdding(null)}
        onPick={(id) => {
          onAddExtra(id)
          if (adding === 'main') onMainChange(id)
        }}
      />
      <CurrencyListSheet
        open={pickingMain}
        title={t('onboarding.currencies.main_title')}
        options={selected.map((id) => byId.get(id)).filter((c): c is CurrencyListItemDto => !!c)}
        value={mainId}
        onClose={() => setPickingMain(false)}
        onPick={onMainChange}
        onAddMore={addable.length > 0 ? () => setAdding('main') : undefined}
      />
    </section>
  )
}
