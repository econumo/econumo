import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronRight } from 'lucide-react'
import { Link, useNavigate } from 'react-router'
import { toast } from 'sonner'
import { RouterPage } from '@/app/router-pages'
import { SettingsShell } from '@/features/settings/SettingsShell'
import { ExportCsvDialog } from '@/features/transactions/ExportCsvDialog'
import { ImportCsvDialog } from '@/features/transactions/ImportCsvDialog'
import { ImportResultDialog } from '@/features/transactions/ImportResultDialog'
import type { AggregatedImportResult } from '@/features/transactions/importCsv'
import { apiErrorMessage } from '@/lib/apiError'
import { decryptCredential } from '@/lib/importCrypto'
import { syncStartDate } from './syncWindow'
import { useImportKey } from './useImportKey'
import { useImportSources, useSyncImportSource } from './queries'

function ActionRow({ label, onClick, disabled }: { label: string; onClick: () => void; disabled?: boolean }) {
  return (
    <button type="button" onClick={onClick} disabled={disabled} className="flex w-full items-center justify-between gap-2 rounded-lg bg-econumo-card px-4 py-3.5 text-left text-sm hover:bg-econumo-hover disabled:opacity-60">
      <span>{label}</span>
      <ChevronRight className="size-4 text-muted-foreground" />
    </button>
  )
}

function LinkRow({ label, to }: { label: string; to: string }) {
  return (
    <Link to={to} className="flex w-full items-center justify-between gap-2 rounded-lg bg-econumo-card px-4 py-3.5 text-left text-sm hover:bg-econumo-hover">
      <span>{label}</span>
      <ChevronRight className="size-4 text-muted-foreground" />
    </Link>
  )
}

export function ImportsDataPage() {
  const { t } = useTranslation()
  const [exportOpen, setExportOpen] = useState(false)
  const [importOpen, setImportOpen] = useState(false)
  const [importResult, setImportResult] = useState<AggregatedImportResult | null>(null)

  const navigate = useNavigate()
  const { data: sources = [] } = useImportSources()
  const { state: keyState } = useImportKey()
  const sync = useSyncImportSource()
  const [syncing, setSyncing] = useState(false)
  const pullSources = sources.filter((s) => s.provider === 'simplefin')

  // One source at a time: each sync holds the server's single SQLite writer
  // for a moment, and the summary toast wants the totals.
  const syncAll = async () => {
    if (syncing) {
      return
    }
    if (keyState.status !== 'unlocked') {
      navigate(RouterPage.SETTINGS_SIMPLEFIN)
      return
    }
    setSyncing(true)
    let imported = 0
    let matched = 0
    let failed = 0
    try {
      for (const source of pullSources) {
        try {
          const accessUrl = await decryptCredential(source.credentialCiphertext)
          const result = await sync.mutateAsync({ sourceId: source.id, accessUrl, startDate: syncStartDate(source) })
          imported += result.run.importedCount
          matched += result.run.matchedCount
          if (result.run.status !== 'completed') {
            failed += 1
          }
        } catch (err) {
          failed += 1
          toast.error(`${source.name}: ${apiErrorMessage(err)}`)
        }
      }
      toast[failed ? 'error' : 'success'](t('imports.data_page.sync_all_done', { imported, matched, failed }))
    } finally {
      setSyncing(false)
    }
  }

  return (
    <SettingsShell
      title={t('imports.data_page.header')}
      backTo={RouterPage.SETTINGS}
    >
      <div className="mx-auto flex w-full max-w-xl flex-col gap-2">
        <ActionRow label={t('settings.import_csv.menu_item')} onClick={() => setImportOpen(true)} />
        <ActionRow label={t('settings.export_csv.menu_item')} onClick={() => setExportOpen(true)} />
        {pullSources.length > 0 ? (
          <ActionRow label={syncing ? t('imports.simplefin.sync.running') : t('imports.data_page.sync_all')} onClick={() => void syncAll()} disabled={syncing} />
        ) : null}
        <LinkRow label={t('imports.rules.page.menu_item')} to={RouterPage.SETTINGS_IMPORT_RULES} />
        <LinkRow label={t('imports.data_page.history')} to={RouterPage.IMPORT_RUNS} />
      </div>

      <ExportCsvDialog open={exportOpen} onClose={() => setExportOpen(false)} />
      <ImportCsvDialog open={importOpen} onClose={() => setImportOpen(false)} onComplete={setImportResult} />
      <ImportResultDialog open={importResult !== null} result={importResult} onClose={() => setImportResult(null)} />
    </SettingsShell>
  )
}
