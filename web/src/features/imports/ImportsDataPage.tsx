import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronRight } from 'lucide-react'
import { RouterPage } from '@/app/router-pages'
import { SettingsShell } from '@/features/settings/SettingsShell'
import { ExportCsvDialog } from '@/features/transactions/ExportCsvDialog'
import { ImportCsvDialog } from '@/features/transactions/ImportCsvDialog'
import { ImportResultDialog } from '@/features/transactions/ImportResultDialog'
import type { AggregatedImportResult } from '@/features/transactions/importCsv'
import { AppleWalletSetup } from './AppleWalletSetup'
import { ImportCards } from './ImportCards'
import { useImportSources } from './queries'

function ActionRow({ label, onClick }: { label: string; onClick: () => void }) {
  return (
    <button type="button" onClick={onClick} className="flex w-full items-center justify-between gap-2 rounded-lg bg-econumo-card px-4 py-3.5 text-left text-sm hover:bg-econumo-hover">
      <span>{label}</span>
      <ChevronRight className="size-4 text-muted-foreground" />
    </button>
  )
}

export function ImportsDataPage() {
  const { t } = useTranslation()
  const [exportOpen, setExportOpen] = useState(false)
  const [importOpen, setImportOpen] = useState(false)
  const [importResult, setImportResult] = useState<AggregatedImportResult | null>(null)
  const { data: sources = [] } = useImportSources()
  const wallet = sources.find((s) => s.provider === 'apple-wallet') ?? null

  return (
    <SettingsShell
      title={t('imports.data_page.header')}
      backTo={RouterPage.SETTINGS}
    >
      <div className="mx-auto flex w-full max-w-xl flex-col gap-6">
        <section className="flex flex-col gap-2">
          <p className="px-1 text-xs uppercase text-muted-foreground">{t('imports.data_page.csv.header')}</p>
          <ActionRow label={t('settings.import_csv.menu_item')} onClick={() => setImportOpen(true)} />
          <ActionRow label={t('settings.export_csv.menu_item')} onClick={() => setExportOpen(true)} />
        </section>

        <section className="flex flex-col gap-2">
          <p className="px-1 text-xs uppercase text-muted-foreground">{t('imports.apple_wallet.header')}</p>
          <AppleWalletSetup key={wallet?.id ?? 'none'} source={wallet} />
          {wallet ? <ImportCards source={wallet} /> : null}
        </section>
      </div>

      <ExportCsvDialog open={exportOpen} onClose={() => setExportOpen(false)} />
      <ImportCsvDialog open={importOpen} onClose={() => setImportOpen(false)} onComplete={setImportResult} />
      <ImportResultDialog open={importResult !== null} result={importResult} onClose={() => setImportResult(null)} />
    </SettingsShell>
  )
}
