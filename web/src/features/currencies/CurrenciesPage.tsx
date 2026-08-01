import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { CurrencyListItemDto } from '@/api/dto/currency'
import { apiErrorMessage } from '@/lib/apiError'
import { ClassificationList, type RowSwitchState } from '@/features/classifications/ClassificationList'
import { useUserData, userCurrencyId } from '@/features/user/queries'
import { CurrencyDialog } from './CurrencyDialog'
import {
  useCurrencies,
  useCurrencyRates,
  useCreateCurrency,
  useUpdateCurrency,
  useArchiveCurrency,
  useUnarchiveCurrency,
  useDeleteCurrency,
  useHideCurrency,
  useShowCurrency,
} from './queries'

// ClassificationItem wants a position; currencies have none (the server
// orders by code and the list is not user-orderable), so the index fills in.
type CurrencyRow = CurrencyListItemDto & { position: number }

export function CurrenciesPage() {
  const { t } = useTranslation()
  const { data: user } = useUserData()
  const { data: currencies } = useCurrencies()
  const { data: rates } = useCurrencyRates()

  const createCurrency = useCreateCurrency()
  const updateCurrency = useUpdateCurrency()
  const archiveCurrency = useArchiveCurrency()
  const unarchiveCurrency = useUnarchiveCurrency()
  const deleteCurrency = useDeleteCurrency()
  const hideCurrency = useHideCurrency()
  const showCurrency = useShowCurrency()

  const [dialog, setDialog] = useState<{ open: boolean; currency: CurrencyListItemDto | null }>({ open: false, currency: null })
  const [error, setError] = useState<string | null>(null)

  const own = currencies?.filter((c) => c.scope === 'own') ?? []
  const globals = currencies?.filter((c) => c.scope === 'global') ?? []
  const items: CurrencyRow[] = [...own, ...globals].map((c, i) => ({ ...c, position: i }))
  const baseId = rates?.[0]?.baseCurrencyId
  const profileId = userCurrencyId(user)
  const rateFor = (id: string) => rates?.find((r) => r.currencyId === id)
  const baseCurrency = currencies?.find((c) => c.id === baseId)

  const mutate = (fn: { mutate: (id: string, opts: { onError: (e: unknown) => void }) => void }, id: string) => {
    setError(null)
    fn.mutate(id, { onError: (e) => setError(apiErrorMessage(e)) })
  }

  const closeDialog = () => setDialog({ open: false, currency: null })

  return (
    <>
      <ClassificationList<CurrencyRow>
        title={t('classifications.currencies.pages.settings.header')}
        heading={t('classifications.currencies.pages.settings.menu_item')}
        info={own.length === 0 ? t('classifications.currencies.pages.settings.empty_state') : undefined}
        alert={error ? <p className="px-1 text-sm text-destructive">{error}</p> : null}
        createLabel={t('classifications.currencies.pages.settings.create_currency')}
        deleteTitle={t('classifications.currencies.modals.delete.title')}
        items={items}
        sections={[
          { label: t('classifications.currencies.pages.settings.my_currencies'), match: (c) => c.scope === 'own' },
          { label: t('classifications.currencies.pages.settings.global_currencies'), match: (c) => c.scope === 'global' },
        ]}
        hasActions={(c) => c.scope === 'own'}
        meta={(c) => {
          const rate = c.scope === 'own' ? rateFor(c.id) : undefined
          return (
            <>
              <span className="truncate text-xs text-muted-foreground">{c.scope === 'own' ? `${c.code} · ${c.symbol}` : c.code}</span>
              {rate ? (
                <span className="truncate text-xs text-muted-foreground">
                  {t('classifications.currencies.pages.settings.rate_caption', {
                    base: baseCurrency?.code ?? '',
                    rate: rate.rate,
                    code: c.code,
                  })}
                </span>
              ) : null}
            </>
          )
        }}
        rowSwitch={(c): RowSwitchState => {
          if (c.scope === 'own') {
            return {
              checked: c.isArchived === 0,
              ariaLabel: `archive ${c.name}`,
              onToggle: () => mutate(c.isArchived === 0 ? archiveCurrency : unarchiveCurrency, c.id),
            }
          }
          const locked = c.id === baseId || c.id === profileId
          return {
            checked: c.isHidden === 0,
            disabled: locked,
            title:
              c.id === baseId
                ? t('classifications.currencies.pages.settings.locked_base')
                : c.id === profileId
                  ? t('classifications.currencies.pages.settings.locked_profile')
                  : undefined,
            ariaLabel: `show ${c.name}`,
            onToggle: () => mutate(c.isHidden === 0 ? hideCurrency : showCurrency, c.id),
          }
        }}
        onCreate={() => setDialog({ open: true, currency: null })}
        onEdit={(c) => setDialog({ open: true, currency: c })}
        onDelete={(id) => mutate(deleteCurrency, id)}
      />

      <CurrencyDialog
        open={dialog.open}
        currency={dialog.currency}
        currentRate={dialog.currency ? rateFor(dialog.currency.id)?.rate : undefined}
        onClose={closeDialog}
        onSubmit={(form) => {
          setError(null)
          if (dialog.currency) {
            updateCurrency.mutate(
              { id: dialog.currency.id, name: form.name, symbol: form.symbol, fractionDigits: form.fractionDigits, rate: form.rate },
              { onSuccess: closeDialog, onError: (e) => setError(apiErrorMessage(e)) },
            )
          } else {
            createCurrency.mutate(
              { code: form.code, name: form.name, symbol: form.symbol || undefined, fractionDigits: form.fractionDigits, rate: form.rate },
              { onSuccess: closeDialog, onError: (e) => setError(apiErrorMessage(e)) },
            )
          }
        }}
      />

    </>
  )
}
