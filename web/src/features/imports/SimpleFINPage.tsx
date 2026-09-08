import { useEffect, useMemo, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { toast } from 'sonner'
import type { ImportCardDto } from '@/api/dto/imports'
import { queryKeys } from '@/app/queryKeys'
import { RouterPage } from '@/app/router-pages'
import { Button } from '@/components/ui/button'
import { InfoBox } from '@/components/InfoBox'
import { SettingsShell } from '@/features/settings/SettingsShell'
import { apiErrorMessage } from '@/lib/apiError'
import { formatDateTime, parseDateTime } from '@/lib/datetime'
import { decryptCredential } from '@/lib/importCrypto'
import { ImportCards } from './ImportCards'
import { ImportRunSummary } from './ImportRunSummary'
import { SimpleFINConnect } from './SimpleFINConnect'
import { SimpleFINUnlock, forgetThisDevice } from './SimpleFINUnlock'
import { syncStartDate } from './syncWindow'
import { useImportKey } from './useImportKey'
import { useExternalAccounts, useImportRuns, useImportSources, useSyncImportSource } from './queries'

export function SimpleFINPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { data: sources = [], isPending: sourcesPending } = useImportSources()
  const { state: keyState, refresh: refreshKey } = useImportKey()
  const source = sources.find((s) => s.provider === 'simplefin') ?? null
  const [accessUrl, setAccessUrl] = useState<string | null>(null)
  const [reconnecting, setReconnecting] = useState(false)
  const [startDate, setStartDate] = useState('')
  const sync = useSyncImportSource()
  const { data: runs = [] } = useImportRuns(source?.id ?? '')
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

  useEffect(() => {
    if (source && !startDate) {
      setStartDate(syncStartDate(source))
    }
  }, [source, startDate])

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
      return <p className="text-sm text-muted-foreground">{t('common.app.modal.loading.data_loading')}</p>
    }
    if (!source || reconnecting) {
      return (
        <SimpleFINConnect keyState={keyState} reconnect={reconnecting} onConnected={onConnected} />
      )
    }
    if (keyState.status === 'locked') {
      return <SimpleFINUnlock wrapped={keyState.wrapped} onUnlocked={refreshKey} onReconnect={() => setReconnecting(true)} />
    }
    if (keyState.status === 'none') {
      // a source exists but its key was deleted server-side (never happens through the UI); reconnect
      return <SimpleFINConnect keyState={keyState} reconnect onConnected={onConnected} />
    }
    return (
      <>
        <div className="flex flex-col gap-2 rounded-lg bg-econumo-card px-4 py-3.5 text-sm">
          <div className="flex items-center justify-between gap-2">
            <span className="font-medium">{source.name}</span>
            <span className="text-xs text-muted-foreground">
              {source.lastSyncedAt ? t('imports.simplefin.sync.last_synced', { date: formatDateTime(parseDateTime(source.lastSyncedAt)) }) : t('imports.simplefin.sync.never')}
            </span>
          </div>
          <label className="flex items-center gap-2 text-xs text-muted-foreground" htmlFor="simplefin-start">
            {t('imports.simplefin.sync.from')}
            <input id="simplefin-start" type="date" value={startDate} onChange={(e) => setStartDate(e.target.value)} className="rounded border bg-background px-2 py-1 text-sm text-foreground" />
          </label>
          <Button type="button" className="w-full sm:w-auto" disabled={sync.isPending || !accessUrl} onClick={runSync}>
            {sync.isPending ? t('imports.simplefin.sync.running') : t('imports.simplefin.sync.button')}
          </Button>
        </div>
        {lastRun ? <ImportRunSummary run={lastRun} accountName={accountName} to={RouterPage.IMPORT_RUN.replace(':id', lastRun.id)} /> : null}
        {source ? (
          <Link to={`${RouterPage.IMPORT_RUNS}?sourceId=${source.id}`} className="px-1 text-sm text-primary hover:underline">{t('imports.runs.history_link')}</Link>
        ) : null}
        {external.isError ? <p role="alert" className="text-sm text-destructive">{apiErrorMessage(external.error)}</p> : null}
        <ImportCards source={source} cards={cards} variant="account" />
        <div className="flex flex-wrap gap-2 pt-2">
          <Button type="button" variant="secondary" size="sm" onClick={() => setReconnecting(true)}>{t('imports.simplefin.unlock.reconnect')}</Button>
          <Button type="button" variant="ghost" size="sm" onClick={() => void forget()}>{t('imports.simplefin.forget_device')}</Button>
        </div>
      </>
    )
  }

  return (
    <SettingsShell title={t('imports.simplefin.title')} backTo={RouterPage.SETTINGS}>
      <div className="mx-auto flex w-full max-w-xl flex-col gap-3">
        <InfoBox>{t('imports.simplefin.intro')}</InfoBox>
        {body()}
      </div>
    </SettingsShell>
  )
}
