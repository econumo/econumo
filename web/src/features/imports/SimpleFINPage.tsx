import { useEffect, useMemo, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ChevronRight } from 'lucide-react'
import { Link } from 'react-router'
import { toast } from 'sonner'
import type { ImportCardDto } from '@/api/dto/imports'
import { queryKeys } from '@/app/queryKeys'
import { RouterPage } from '@/app/router-pages'
import { Button } from '@/components/ui/button'
import { OfflineUnavailable } from '@/components/OfflineUnavailable'
import { useOnline } from '@/hooks/useOnline'
import { useUnavailableOffline } from '@/lib/offlineQuery'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { SettingsShell } from '@/features/settings/SettingsShell'
import { apiErrorMessage } from '@/lib/apiError'
import { formatDate, formatDateTime, parseDateTime } from '@/lib/datetime'
import { TransactionDatePicker } from '@/features/transactions/TransactionDatePicker'
import { decryptCredential } from '@/lib/importCrypto'
import { ImportCards } from './ImportCards'
import { ImportRunSummary } from './ImportRunSummary'
import { SimpleFINConnect } from './SimpleFINConnect'
import { SimpleFINUnlock, forgetThisDevice } from './SimpleFINUnlock'
import { syncStartDate } from './syncWindow'
import { useImportKey } from './useImportKey'
import { useDeleteImportSource, useExternalAccounts, useImportRuns, useImportSources, useSyncImportSource } from './queries'

export function SimpleFINPage() {
  const { t } = useTranslation()
  const online = useOnline()
  const queryClient = useQueryClient()
  const sourcesQuery = useImportSources()
  const { data: sources = [], isPending: sourcesPending } = sourcesQuery
  const { state: keyState, refresh: refreshKey, remote: remoteKey } = useImportKey()
  // only network data counts: the local key read settles offline too
  const unavailableOffline = useUnavailableOffline(sourcesQuery, remoteKey)
  const source = sources.find((s) => s.provider === 'simplefin') ?? null
  const [accessUrl, setAccessUrl] = useState<string | null>(null)
  const [reconnecting, setReconnecting] = useState(false)
  // a reconnect forced by a credential this key cannot decrypt has nothing to go back to
  const [userReconnect, setUserReconnect] = useState(false)
  // '' = the source's default window, computed during render so a new source
  // (after a disconnect) starts from its own first-sync default
  const [pickedDate, setStartDate] = useState('')
  const startDate = pickedDate || (source ? syncStartDate(source) : '')
  const [editingDate, setEditingDate] = useState(false)
  const sync = useSyncImportSource()
  const disconnect = useDeleteImportSource({ handlesOwnErrors: true })
  const [forgetOpen, setForgetOpen] = useState(false)
  const [disconnectOpen, setDisconnectOpen] = useState(false)
  const { data: runs = [] } = useImportRuns(source?.id)
  const external = useExternalAccounts(source?.id ?? '', accessUrl)

  // Decrypt the stored credential once the device key is available. The
  // plaintext stays in React state for this page's lifetime only.
  useEffect(() => {
    if (!source?.credentialCiphertext || keyState.status !== 'unlocked' || accessUrl !== null) {
      return
    }
    let cancelled = false
    decryptCredential(source.credentialCiphertext)
      .then((url) => { if (!cancelled) setAccessUrl(url) })
      .catch(() => { if (!cancelled) setReconnecting(true) })  // ciphertext from another key: reconnect
    return () => { cancelled = true }
  }, [source?.credentialCiphertext, keyState.status, accessUrl])

  // Bridge accounts merged over the server's mapping rows: an account the
  // bridge reports that no row exists for yet shows up unmapped.
  const cards: ImportCardDto[] = useMemo(() => {
    if (!source) {
      return []
    }
    const byId = new Map(source.cards.map((c) => [c.externalAccountId, c]))
    const merged = (external.data ?? []).map((a) => byId.get(a.externalAccountId) ?? {
      externalAccountId: a.externalAccountId, externalName: a.externalName, externalCurrency: a.externalCurrency,
      state: a.state, accountId: a.accountId, queuedCount: 0, tapCount: 0, lastSeenAt: '',
    })
    const seen = new Set(merged.map((c) => c.externalAccountId))
    return [...merged, ...source.cards.filter((c) => !seen.has(c.externalAccountId))]
  }, [source, external.data])

  const accountName = (externalAccountId: string) => cards.find((c) => c.externalAccountId === externalAccountId)?.externalName ?? ''
  const lastRun = runs[0]

  const runSync = () => {
    if (!source || !accessUrl) {
      return
    }
    sync.mutate({ sourceId: source.id, accessUrl, startDate }, {
      onSuccess: (result) => {
        setStartDate(syncStartDate({ ...source, lastSyncedAt: result.run.finishedAt || source.lastSyncedAt }))
        setEditingDate(false)
        if (result.run.status === 'completed') {
          toast.success(t('imports.simplefin.sync.done_toast', { imported: result.run.importedCount, matched: result.run.matchedCount }))
        }
      },
      onError: (err) => toast.error(apiErrorMessage(err)),
    })
  }

  // A reconnect can point at a different bridge account set, and the access
  // url is deliberately not part of the query key, so the cached list has to
  // be dropped explicitly.
  const onConnected = (url: string) => {
    setAccessUrl(url)
    setReconnecting(false)
    setUserReconnect(false)
    if (source) {
      void queryClient.invalidateQueries({ queryKey: queryKeys.importExternalAccounts(source.id) })
    }
    refreshKey()
  }

  const forget = async () => {
    await forgetThisDevice()
    setAccessUrl(null)
    refreshKey()
  }

  const body = () => {
    if (sourcesPending || keyState.status === 'loading') {
      if (unavailableOffline) {
        return <OfflineUnavailable />
      }
      return <p className="text-sm text-muted-foreground">{t('common.app.modal.loading.data_loading')}</p>
    }
    if (!source || reconnecting) {
      return (
        <SimpleFINConnect
          keyState={keyState} reconnect={reconnecting} onConnected={onConnected}
          onCancel={source && userReconnect ? () => { setReconnecting(false); setUserReconnect(false) } : undefined}
        />
      )
    }
    if (keyState.status === 'locked') {
      return <SimpleFINUnlock wrapped={keyState.wrapped} stale={keyState.stale} onUnlocked={refreshKey} onReconnect={() => { setReconnecting(true); setUserReconnect(true) }} />
    }
    if (keyState.status === 'none') {
      // a source exists but its key was deleted server-side (never happens through the UI); reconnect
      return <SimpleFINConnect keyState={keyState} reconnect onConnected={onConnected} />
    }
    return (
      <>
        <div className="rounded-lg bg-econumo-card px-4 py-3 text-sm">
          <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1 border-b border-border pb-3">
            <h2 className="font-medium">{source.name}</h2>
            <span className="text-xs text-muted-foreground">
              {source.lastSyncedAt ? t('imports.simplefin.sync.last_synced', { date: formatDateTime(parseDateTime(source.lastSyncedAt)) }) : t('imports.simplefin.sync.never')}
            </span>
          </div>
          <div className="flex flex-wrap items-center justify-between gap-3 pt-3">
            {/* after the first sync the default window is right for almost every sync,
                so the date hides behind Change, kept for re-pulling an older range */}
            {source.lastSyncedAt && !editingDate ? (
              <span className="flex flex-wrap items-center gap-x-2 gap-y-1 text-muted-foreground">
                {t('imports.simplefin.sync.since', { date: startDate })}
                <button type="button" className="text-econumo-link underline-offset-2 hover:underline" onClick={() => setEditingDate(true)}>
                  {t('imports.simplefin.sync.change')}
                </button>
              </span>
            ) : (
              <span className="flex items-center gap-2 text-muted-foreground">
                {t('imports.simplefin.sync.from')}
                <TransactionDatePicker
                  value={startDate} onChange={(day) => setStartDate(formatDate(day))}
                  label={t('imports.simplefin.sync.from')} className="bg-background text-foreground max-sm:h-10"
                />
              </span>
            )}
            <Button type="button" className="h-10 max-sm:w-full" disabled={sync.isPending || !accessUrl || !online} onClick={runSync}>
              {sync.isPending ? t('imports.simplefin.sync.running') : t('imports.simplefin.sync.button')}
            </Button>
          </div>
        </div>
        {lastRun ? <ImportRunSummary run={lastRun} accountName={accountName} to={RouterPage.SETTINGS_SIMPLEFIN_RUN.replace(':id', lastRun.id)} /> : null}
        {lastRun ? (
          <Link
            to={RouterPage.SETTINGS_SIMPLEFIN_HISTORY}
            className="flex items-center justify-between gap-2 rounded-lg bg-econumo-card px-4 py-3 text-sm hover:bg-econumo-hover"
          >
            <span>{t('imports.runs.history_link')}</span>
            <ChevronRight className="size-4 text-muted-foreground" />
          </Link>
        ) : null}
        {external.isError ? <p role="alert" className="px-1 text-sm text-destructive">{apiErrorMessage(external.error)}</p> : null}
        <ImportCards source={source} cards={cards} variant="account" />
        <div className="flex flex-wrap justify-center gap-2 pt-4">
          <Button type="button" variant="ghost" disabled={!online} onClick={() => { setReconnecting(true); setUserReconnect(true) }}>{t('imports.simplefin.unlock.reconnect')}</Button>
          <Button type="button" variant="ghost" onClick={() => setForgetOpen(true)}>{t('imports.simplefin.forget_device')}</Button>
          <Button type="button" variant="ghost" className="text-destructive hover:text-destructive" disabled={!online} onClick={() => setDisconnectOpen(true)}>
            {t('imports.simplefin.disconnect_full')}
          </Button>
        </div>
        <p className="px-1 pt-2 text-center text-xs text-muted-foreground">{t('imports.simplefin.disclaimer')}</p>
        <ConfirmDialog
          open={forgetOpen}
          onClose={() => setForgetOpen(false)}
          onConfirm={() => {
            setForgetOpen(false)
            void forget()
          }}
          title={t('imports.simplefin.forget_modal.title')}
          question={t('imports.simplefin.forget_modal.question')}
          confirmLabel={t('imports.simplefin.forget_device')}
          cancelLabel={t('common.button.cancel.label')}
          offlineOk
        />
        <ConfirmDialog
          open={disconnectOpen}
          onClose={() => setDisconnectOpen(false)}
          onConfirm={() => {
            setDisconnectOpen(false)
            disconnect.mutate(source.id, {
              onSuccess: () => {
                setAccessUrl(null)
                setStartDate('')
                setEditingDate(false)
              },
              onError: (err) => toast.error(apiErrorMessage(err)),
            })
          }}
          title={t('imports.simplefin.disconnect_modal.title')}
          question={t('imports.simplefin.disconnect_modal.question')}
          confirmLabel={t('imports.simplefin.disconnect')}
          cancelLabel={t('common.button.cancel.label')}
          destructive
        />
      </>
    )
  }

  return (
    <SettingsShell title={t('imports.simplefin.title')} backTo={RouterPage.SETTINGS}>
      <div className="mx-auto flex w-full max-w-xl flex-col gap-2">
        {body()}
      </div>
    </SettingsShell>
  )
}
