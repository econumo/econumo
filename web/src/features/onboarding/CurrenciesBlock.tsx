import { useState } from 'react'
import { Plus, Star, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { CommandEmpty, CommandItem } from '@/components/ui/command'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'
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
  const [adding, setAdding] = useState(false)
  const [pickingMain, setPickingMain] = useState(false)
  const byId = new Map(currencies.map((c) => [c.id, c]))
  const code = (id: Id) => byId.get(id)?.code ?? ''
  const extras = extraIds.filter((id) => !usedIds.includes(id))
  const others = [...usedIds, ...extras].filter((id) => id !== mainId)
  const selected = mainId ? [mainId, ...others] : others
  const addable = selectableOptions.filter((c) => !selected.includes(c.id))
  const chipClass = 'inline-flex h-8 items-center gap-1 rounded-md border px-2.5 text-sm'

  return (
    <section>
      <h3 className="mt-4 mb-1 text-xs tracking-wider text-muted-foreground uppercase">{t('onboarding.currencies.title')}</h3>
      <p className="mb-2 text-sm text-muted-foreground">{t('onboarding.currencies.hint')}</p>
      <ul className="flex flex-wrap items-center gap-1.5" aria-label={t('onboarding.currencies.selected')}>
        <li>
          {/* the main chip picks the main currency from the whole list; it is never removed */}
          <button
            type="button"
            aria-label={mainId ? t('onboarding.currencies.main_label', { code: code(mainId) }) : t('onboarding.currencies.main_title')}
            aria-haspopup="dialog"
            className={cn(chipClass, 'border-primary/40 bg-primary/15 text-foreground hover:bg-primary/25')}
            onClick={() => setPickingMain(true)}
          >
            <Star className="size-3.5 fill-primary text-primary" />
            {mainId ? code(mainId) : t('onboarding.currencies.main_title')}
            {mainId && (
              <span className="text-muted-foreground">
                <span aria-hidden>· </span>
                {t('onboarding.currencies.main_suffix')}
              </span>
            )}
          </button>
        </li>
        {others.map((id) => (
          <li key={id}>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <button type="button" className={cn(chipClass, 'hover:bg-accent')}>
                  {code(id)}
                </button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start">
                <DropdownMenuItem onSelect={() => onMainChange(id)}>
                  <Star className="size-4" />
                  {t('onboarding.currencies.make_main')}
                </DropdownMenuItem>
                {/* a currency a listed account uses stays visible */}
                {extras.includes(id) && (
                  <DropdownMenuItem variant="destructive" onSelect={() => onRemoveExtra(id)}>
                    <X className="size-4" />
                    {t('onboarding.currencies.remove_action')}
                  </DropdownMenuItem>
                )}
              </DropdownMenuContent>
            </DropdownMenu>
          </li>
        ))}
        <li>
          <button type="button" className={cn(chipClass, 'border-dashed text-primary hover:bg-accent')} onClick={() => setAdding(true)}>
            <Plus className="size-3.5" />
            {t('onboarding.currencies.add')}
          </button>
        </li>
      </ul>
      <p className="mt-1.5 text-xs text-muted-foreground">{t('onboarding.currencies.chips_hint')}</p>
      <CurrencyListSheet open={adding} title={t('onboarding.currencies.add')} options={addable} onClose={() => setAdding(false)} onPick={onAddExtra} />
      <CurrencyListSheet
        open={pickingMain}
        title={t('onboarding.currencies.main_title')}
        options={selectableOptions}
        value={mainId}
        onClose={() => setPickingMain(false)}
        onPick={(id) => {
          if (!selected.includes(id)) onAddExtra(id)
          onMainChange(id)
        }}
      />
    </section>
  )
}
