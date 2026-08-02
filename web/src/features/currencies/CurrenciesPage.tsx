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
  useDeleteCurrency,
  useHideCurrency,
  useShowCurrency,
  useHideAllCurrencies,
  useShowAllCurrencies,
} from './queries'

// ClassificationItem wants a position (the index fills in — the server
// orders by code and the list is not user-orderable) and an isArchived flag
// (currencies have no archival, so it is constant 0).
type CurrencyRow = CurrencyListItemDto & { position: number; isArchived: 0 }

export function CurrenciesPage() {
  const { t } = useTranslation()
  const { data: user } = useUserData()
  const { data: currencies } = useCurrencies()
  const { data: rates } = useCurrencyRates()

  const createCurrency = useCreateCurrency()
  const updateCurrency = useUpdateCurrency()
  const deleteCurrency = useDeleteCurrency()
  const hideCurrency = useHideCurrency()
  const showCurrency = useShowCurrency()
  const hideAllCurrencies = useHideAllCurrencies()
  const showAllCurrencies = useShowAllCurrencies()

  const [dialog, setDialog] = useState<{ open: boolean; currency: CurrencyListItemDto | null }>({ open: false, currency: null })
  const [error, setError] = useState<string | null>(null)

  const own = currencies?.filter((c) => c.scope === 'own' && c.isDeleted === 0) ?? []
  const globals = currencies?.filter((c) => c.scope === 'global' && c.isDeleted === 0) ?? []
  const items: CurrencyRow[] = [...own, ...globals].map((c, i) => ({ ...c, position: i, isArchived: 0 as const }))
  const baseId = rates?.[0]?.baseCurrencyId
  const profileId = userCurrencyId(user)
  const rateFor = (id: string) => rates?.find((r) => r.currencyId === id)
  const baseCurrency = currencies?.find((c) => c.id === baseId)

  const mutate = (fn: { mutate: (id: string, opts: { onError: (e: unknown) => void }) => void }, id: string) => {
    setError(null)
    fn.mutate(id, { onError: (e) => setError(apiErrorMessage(e)) })
  }

  // The bulk link acts on the non-locked globals: with any still enabled it
  // disables all, otherwise it re-enables all. No non-locked globals = no link.
  const bulkTargets = globals.filter((c) => c.id !== baseId && c.id !== profileId)
  const anyGlobalEnabled = bulkTargets.some((c) => c.isHidden === 0)
  const bulkAction =
    bulkTargets.length === 0 ? null : (
      <button
        type="button"
        className="cursor-pointer text-sm text-muted-foreground hover:text-foreground hover:underline"
        onClick={() => {
          setError(null)
          ;(anyGlobalEnabled ? hideAllCurrencies : showAllCurrencies).mutate(undefined, {
            onError: (e) => setError(apiErrorMessage(e)),
          })
        }}
      >
        {anyGlobalEnabled
          ? t('classifications.currencies.pages.settings.disable_all')
          : t('classifications.currencies.pages.settings.enable_all')}
      </button>
    )

  const closeDialog = () => {
    setDialog({ open: false, currency: null })
    setError(null)
  }
  const openDialog = (currency: CurrencyListItemDto | null) => {
    setError(null)
    setDialog({ open: true, currency })
  }

  return (
    <>
      <ClassificationList<CurrencyRow>
        title={t('classifications.currencies.pages.settings.header')}
        heading={t('classifications.currencies.pages.settings.menu_item')}
        info={t('classifications.currencies.pages.settings.info')}
        alert={error && !dialog.open ? <p className="px-1 text-sm text-destructive">{error}</p> : null}
        createLabel={t('classifications.currencies.pages.settings.create_currency')}
        deleteTitle={t('classifications.currencies.modals.delete.title')}
        items={items}
        analyticsType="currency"
        sections={[
          { label: t('classifications.currencies.pages.settings.my_currencies'), match: (c) => c.scope === 'own' },
          { label: t('classifications.currencies.pages.settings.global_currencies'), match: (c) => c.scope === 'global', action: bulkAction },
        ]}
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
        rowActions={(c) => {
          if (c.scope === 'own') {
            return undefined // default Edit/Delete menu; Delete is the only lifecycle action
          }
          // Globals still need the kebab menu for the mobile sheet's
          // Enable/Disable action, since the switch itself isn't reachable there.
          const locked = c.id === baseId || c.id === profileId
          return [
            {
              label: c.isHidden === 0 ? t('classifications.currencies.pages.settings.disable_currency') : t('classifications.currencies.pages.settings.enable_currency'),
              disabled: locked,
              title:
                c.id === baseId
                  ? t('classifications.currencies.pages.settings.locked_base')
                  : c.id === profileId
                    ? t('classifications.currencies.pages.settings.locked_profile')
                    : undefined,
              onSelect: () => mutate(c.isHidden === 0 ? hideCurrency : showCurrency, c.id),
            },
          ]
        }}
        rowSwitch={(c): RowSwitchState | null => {
          // Own customs have one lifecycle action, Delete, so they carry no
          // switch. Globals keep the enable/disable toggle; the base and
          // profile currencies must stay enabled, so theirs are locked.
          if (c.scope === 'own') {
            return null
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
            ariaLabel: `enable ${c.name}`,
            onToggle: () => mutate(c.isHidden === 0 ? hideCurrency : showCurrency, c.id),
          }
        }}
        onCreate={() => openDialog(null)}
        onEdit={(c) => openDialog(c)}
        onDelete={(id) => mutate(deleteCurrency, id)}
      />

      <CurrencyDialog
        open={dialog.open}
        currency={dialog.currency}
        currentRate={dialog.currency ? rateFor(dialog.currency.id)?.rate : undefined}
        baseCode={baseCurrency?.code}
        serverError={error}
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
