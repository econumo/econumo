import { useState } from 'react'
import { isAxiosError } from 'axios'
import { MoreVertical, Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Switch } from '@/components/ui/switch'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { useIsCompact } from '@/hooks/useIsCompact'
import type { CurrencyListItemDto } from '@/api/dto/currency'
import { RouterPage } from '@/app/router-pages'
import { SettingsShell } from '@/features/settings/SettingsShell'
import { useUserData, userCurrencyId } from '@/features/user/queries'
import { CurrencyDialog } from './CurrencyDialog'
import { RateDialog } from './RateDialog'
import {
  useCurrencies,
  useCurrencyRates,
  useCreateCurrency,
  useUpdateCurrency,
  useSetCurrencyRate,
  useArchiveCurrency,
  useUnarchiveCurrency,
  useDeleteCurrency,
  useHideCurrency,
  useShowCurrency,
} from './queries'

function serverMessage(error: unknown): string {
  if (isAxiosError(error)) {
    const message = (error.response?.data as { message?: string } | undefined)?.message
    if (message) return message
  }
  return 'Something went wrong'
}

export function CurrenciesPage() {
  const { t } = useTranslation()
  const isCompact = useIsCompact()
  const { data: user } = useUserData()
  const { data: currencies } = useCurrencies()
  const { data: rates } = useCurrencyRates()

  const createCurrency = useCreateCurrency()
  const updateCurrency = useUpdateCurrency()
  const setCurrencyRate = useSetCurrencyRate()
  const archiveCurrency = useArchiveCurrency()
  const unarchiveCurrency = useUnarchiveCurrency()
  const deleteCurrency = useDeleteCurrency()
  const hideCurrency = useHideCurrency()
  const showCurrency = useShowCurrency()

  const [dialog, setDialog] = useState<{ open: boolean; currency: CurrencyListItemDto | null }>({ open: false, currency: null })
  const [rateDialog, setRateDialog] = useState<{ open: boolean; currency: CurrencyListItemDto | null }>({ open: false, currency: null })
  const [deleteTarget, setDeleteTarget] = useState<CurrencyListItemDto | null>(null)
  const [openMenuId, setOpenMenuId] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [rateError, setRateError] = useState<string | null>(null)

  const own = currencies?.filter((c) => c.scope === 'own') ?? []
  const globals = currencies?.filter((c) => c.scope === 'global') ?? []
  const baseId = rates?.[0]?.baseCurrencyId
  const profileId = userCurrencyId(user)
  const rateFor = (id: string) => rates?.find((r) => r.currencyId === id)
  const baseCurrency = currencies?.find((c) => c.id === baseId)

  const closeDialog = () => setDialog({ open: false, currency: null })
  const closeRateDialog = () => {
    setRateDialog({ open: false, currency: null })
    setRateError(null)
  }

  return (
    <SettingsShell
      title={t('modules.classifications.currencies.pages.settings.header')}
      heading={t('modules.classifications.currencies.pages.settings.menu_item')}
      backTo={RouterPage.SETTINGS}
      actions={
        isCompact ? (
          <Button
            type="button"
            size="icon"
            aria-label={t('modules.classifications.currencies.pages.settings.create_currency')}
            title={t('modules.classifications.currencies.pages.settings.create_currency')}
            onClick={() => setDialog({ open: true, currency: null })}
          >
            <Plus className="size-4" />
          </Button>
        ) : (
          <Button type="button" size="sm" onClick={() => setDialog({ open: true, currency: null })}>
            <Plus className="size-4" />
            {t('modules.classifications.currencies.pages.settings.create_currency')}
          </Button>
        )
      }
    >
      <div className="flex flex-col gap-6">
        {error ? <p className="px-1 text-sm text-destructive">{error}</p> : null}
        <section className="flex flex-col gap-1">
          <h2 className="mt-2 mb-1 px-1 text-sm font-semibold uppercase tracking-wide">
            {t('modules.classifications.currencies.pages.settings.my_currencies')}
          </h2>
          {own.length === 0 ? (
            <p className="px-1 py-2 text-sm text-muted-foreground">{t('modules.classifications.currencies.pages.settings.empty_state')}</p>
          ) : (
            own.map((currency) => {
              const rate = rateFor(currency.id)
              return (
                <div key={currency.id} className="flex items-center gap-2 rounded-md px-1 py-1.5 hover:bg-accent">
                  <span className="flex min-w-0 flex-1 flex-col">
                    <span className="truncate text-sm">{currency.name}</span>
                    <span className="truncate text-xs text-muted-foreground">
                      {currency.code} · {currency.symbol}
                    </span>
                    {rate ? (
                      <span className="truncate text-xs text-muted-foreground">
                        {t('modules.classifications.currencies.pages.settings.rate_caption', {
                          base: baseCurrency?.code ?? '',
                          rate: rate.rate,
                          code: currency.code,
                        })}
                      </span>
                    ) : null}
                  </span>
                  {currency.isArchived === 1 ? (
                    <Badge variant="secondary">{t('modules.classifications.currencies.pages.settings.archived_item')}</Badge>
                  ) : null}
                  <Switch
                    aria-label={`archive ${currency.name}`}
                    checked={currency.isArchived === 0}
                    onCheckedChange={() => {
                      setError(null)
                      if (currency.isArchived === 0) {
                        archiveCurrency.mutate(currency.id, { onError: (e) => setError(serverMessage(e)) })
                      } else {
                        unarchiveCurrency.mutate(currency.id, { onError: (e) => setError(serverMessage(e)) })
                      }
                    }}
                  />
                  <DropdownMenu open={openMenuId === currency.id} onOpenChange={(open) => setOpenMenuId(open ? currency.id : null)}>
                    <DropdownMenuTrigger asChild>
                      <Button type="button" variant="ghost" size="icon" aria-label={`actions ${currency.name}`}>
                        <MoreVertical className="size-4" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem onSelect={() => setDialog({ open: true, currency })}>{t('elements.button.edit.label')}</DropdownMenuItem>
                      <DropdownMenuItem onSelect={() => setRateDialog({ open: true, currency })}>
                        {t('modules.classifications.currencies.modals.rate.header')}
                      </DropdownMenuItem>
                      <DropdownMenuItem variant="destructive" onSelect={() => setDeleteTarget(currency)}>
                        {t('elements.button.delete.label')}
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
              )
            })
          )}
        </section>

        <section className="flex flex-col gap-1">
          <h2 className="mt-2 mb-1 px-1 text-sm font-semibold uppercase tracking-wide">
            {t('modules.classifications.currencies.pages.settings.global_currencies')}
          </h2>
          {globals.map((currency) => {
            const locked = currency.id === baseId || currency.id === profileId
            const lockedTitle =
              currency.id === baseId
                ? t('modules.classifications.currencies.pages.settings.locked_base')
                : currency.id === profileId
                  ? t('modules.classifications.currencies.pages.settings.locked_profile')
                  : undefined
            return (
              <div key={currency.id} className="flex items-center gap-2 rounded-md px-1 py-1.5 hover:bg-accent">
                <span className="flex min-w-0 flex-1 flex-col">
                  <span className="truncate text-sm">{currency.name}</span>
                  <span className="truncate text-xs text-muted-foreground">{currency.code}</span>
                </span>
                <Switch
                  aria-label={`show ${currency.name}`}
                  checked={currency.isHidden === 0}
                  disabled={locked}
                  title={lockedTitle}
                  onCheckedChange={(checked) => {
                    setError(null)
                    if (checked) {
                      showCurrency.mutate(currency.id, { onError: (e) => setError(serverMessage(e)) })
                    } else {
                      hideCurrency.mutate(currency.id, { onError: (e) => setError(serverMessage(e)) })
                    }
                  }}
                />
              </div>
            )
          })}
        </section>
      </div>

      <CurrencyDialog
        open={dialog.open}
        currency={dialog.currency}
        onClose={closeDialog}
        onSubmit={(form) => {
          setError(null)
          if (dialog.currency) {
            updateCurrency.mutate(
              { id: dialog.currency.id, name: form.name, symbol: form.symbol, fractionDigits: form.fractionDigits },
              { onSuccess: closeDialog, onError: (e) => setError(serverMessage(e)) },
            )
          } else {
            createCurrency.mutate(
              { code: form.code, name: form.name, symbol: form.symbol || undefined, fractionDigits: form.fractionDigits, rate: form.rate || undefined },
              { onSuccess: closeDialog, onError: (e) => setError(serverMessage(e)) },
            )
          }
        }}
      />

      <RateDialog
        open={rateDialog.open}
        currency={rateDialog.currency}
        serverError={rateError}
        onClose={closeRateDialog}
        onSubmit={(form) => {
          if (!rateDialog.currency) {
            return
          }
          setRateError(null)
          setCurrencyRate.mutate(
            { currencyId: rateDialog.currency.id, rate: form.rate, date: form.date },
            { onSuccess: closeRateDialog, onError: (e) => setRateError(serverMessage(e)) },
          )
        }}
      />

      <ConfirmDialog
        open={deleteTarget !== null}
        onClose={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (deleteTarget) {
            setError(null)
            deleteCurrency.mutate(deleteTarget.id, { onError: (e) => setError(serverMessage(e)) })
            setDeleteTarget(null)
          }
        }}
        title={t('modules.classifications.currencies.modals.delete.title')}
        question={t('modules.classifications.currencies.modals.delete.question', { name: deleteTarget?.name ?? '' })}
        confirmLabel={t('elements.button.delete.label')}
        cancelLabel={t('elements.button.cancel.label')}
        destructive
      />
    </SettingsShell>
  )
}
