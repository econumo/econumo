import { useState } from 'react'
import type { DragEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { FileUp } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { ImportCsvDialog } from '@/features/transactions/ImportCsvDialog'
import { apiErrorMessage } from '@/lib/apiError'
import { cn } from '@/lib/utils'
import type { StepBodyProps } from './OnboardingPage'
import { useDay0 } from './useDay0'

const isCsv = (file: File) => file.name.toLowerCase().endsWith('.csv') || file.type === 'text/csv'

export function ImportStep({ onDone, onSkip }: StepBodyProps) {
  const { t } = useTranslation()
  const day0 = useDay0()
  const [open, setOpen] = useState(false)
  const [pending, setPending] = useState(false)
  const [dropped, setDropped] = useState<File | undefined>(undefined)
  const [dragging, setDragging] = useState(false)
  const [notCsv, setNotCsv] = useState(false)
  const skip = () => {
    setPending(true)
    void onSkip().finally(() => setPending(false))
  }
  // the dialog has settled the step through finish-import already; this retries one that failed
  // there. Categories and accounts stay open: the import's rows are reviewed there next.
  const finish = () => {
    void day0
      .finishImport()
      .then(() => onDone())
      .catch(() => undefined)
  }
  // the rows are in: importing the same file again while the step waits for its retry would duplicate them
  const blocked = day0.error != null || day0.pending
  const choose = () => {
    if (blocked) return
    setDropped(undefined)
    setNotCsv(false)
    setOpen(true)
  }
  const dragOver = (e: DragEvent) => {
    if (!e.dataTransfer.types.includes('Files')) return
    e.preventDefault()
    e.dataTransfer.dropEffect = 'copy'
    setDragging(true)
  }
  const dragLeave = (e: DragEvent) => {
    if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setDragging(false)
  }
  const drop = (e: DragEvent) => {
    e.preventDefault()
    setDragging(false)
    if (blocked) return
    const file = e.dataTransfer.files[0]
    if (!file) return
    if (!isCsv(file)) {
      setNotCsv(true)
      return
    }
    setNotCsv(false)
    setDropped(file)
    setOpen(true)
  }
  return (
    <section className="flex flex-col gap-2">
      <h2 className="text-2xl font-normal">{t('onboarding.import_step.heading')}</h2>
      <p className="text-sm text-muted-foreground">{t('onboarding.import_step.text')}</p>
      <button
        type="button"
        data-dragging={dragging || undefined}
        className={cn(
          'mt-4 flex flex-col items-center gap-2 rounded-xl border-2 border-dashed border-primary/40 bg-primary/5 p-8 text-sm text-muted-foreground transition-colors outline-none hover:bg-primary/10 focus-visible:ring-2 focus-visible:ring-ring/50 disabled:pointer-events-none disabled:opacity-50',
          dragging && 'border-primary bg-primary/15 text-foreground',
        )}
        disabled={blocked}
        onClick={choose}
        onDragEnter={dragOver}
        onDragOver={dragOver}
        onDragLeave={dragLeave}
        onDrop={drop}
      >
        <FileUp className="pointer-events-none size-6" />
        <span className="pointer-events-none">{t('onboarding.import_step.choose')}</span>
        <span aria-hidden className="pointer-events-none hidden text-xs sm:block">{t('onboarding.import_step.drop_here')}</span>
      </button>
      {day0.error != null && (
        <div className="flex items-center justify-between gap-4">
          <p role="alert" className="text-sm text-destructive">
            {apiErrorMessage(day0.error)}
          </p>
          <Button type="button" variant="outline" disabled={day0.pending} onClick={finish}>
            {t('onboarding.retry')}
          </Button>
        </div>
      )}
      {notCsv && (
        <p role="alert" className="text-sm text-destructive">
          {t('onboarding.import_step.not_csv')}
        </p>
      )}
      <div className="mt-6 flex items-center justify-between">
        <Button type="button" variant="ghost" disabled={pending || day0.pending} onClick={skip}>
          {t('onboarding.import_step.skip_short')}
        </Button>
        <Button type="button" variant="ghost" disabled={pending || day0.pending} onClick={skip}>
          {t('onboarding.import_step.skip')}
        </Button>
      </div>
      <ImportCsvDialog
        open={open}
        initialFile={dropped}
        onClose={() => setOpen(false)}
        onComplete={(result) => {
          if (result.imported > 0) finish()
        }}
      />
    </section>
  )
}
