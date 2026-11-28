import { useRef, useState } from 'react'
import type { DragEvent } from 'react'
import { FileUp, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'

export type FileError = 'too_big' | 'empty' | 'unreadable'

interface FileStepProps {
  /** the loaded file's name, null before one is loaded */
  fileName: string | null
  /** the file is loaded and its columns are being read (the AI's answer is awaited) */
  reading?: boolean
  error: FileError | null
  onFile: (file: File) => void
}

export function FileStep({ fileName, reading = false, error, onFile }: FileStepProps) {
  const { t } = useTranslation()
  const input = useRef<HTMLInputElement>(null)
  const [dragging, setDragging] = useState(false)

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
    const file = e.dataTransfer.files[0]
    if (file) onFile(file)
  }

  return (
    <div className="flex flex-col gap-2">
      <button
        type="button"
        data-dragging={dragging || undefined}
        className={cn(
          'flex flex-col items-center gap-2 rounded-xl border-2 border-dashed border-primary/40 bg-primary/5 p-8 text-sm text-muted-foreground transition-colors outline-none hover:bg-primary/10 focus-visible:ring-2 focus-visible:ring-ring/50',
          dragging && 'border-primary bg-primary/15 text-foreground',
        )}
        onClick={() => input.current?.click()}
        onDragEnter={dragOver}
        onDragOver={dragOver}
        onDragLeave={dragLeave}
        onDrop={drop}
      >
        <FileUp className="pointer-events-none size-6" />
        <span className="pointer-events-none">{fileName ? t('transactions.import_wizard.file.choose_another') : t('transactions.import_wizard.file.choose')}</span>
        <span aria-hidden className="pointer-events-none hidden text-xs sm:block">{t('transactions.import_wizard.file.drop_here')}</span>
      </button>
      <input
        ref={input}
        type="file"
        accept=".csv,text/csv"
        className="sr-only"
        tabIndex={-1}
        aria-label={t('transactions.import_wizard.file.choose')}
        onChange={(e) => {
          const file = e.target.files?.[0]
          // the same file picked again after an error must fire change again
          e.target.value = ''
          if (file) onFile(file)
        }}
      />
      {fileName ? <p className="truncate text-sm">{fileName}</p> : null}
      {reading ? (
        <p role="status" className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 aria-hidden className="size-4 animate-spin" />
          {t('transactions.import_wizard.file.reading')}
        </p>
      ) : null}
      <p className="text-xs text-muted-foreground">{t('transactions.import_wizard.file.hint')}</p>
      {error ? (
        <p role="alert" className="text-sm text-destructive">
          {t(`transactions.import_wizard.file.error.${error}`)}
        </p>
      ) : null}
    </div>
  )
}
