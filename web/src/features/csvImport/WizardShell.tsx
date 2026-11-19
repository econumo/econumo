import { useId } from 'react'
import type { ReactNode } from 'react'
import { AlertTriangle, ArrowLeft, Check, ChevronRight, FileSpreadsheet, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { useIsPhone } from '@/hooks/useIsPhone'
import { pluralPick } from '@/lib/plural'
import { cn } from '@/lib/utils'
import type { StepId } from './prepare'

export interface ShellStep {
  id: Exclude<StepId, 'file'>
  title: string
  summary: string
  answered: boolean
  /** a required answer is missing */
  warning: boolean
}

interface WizardShellProps {
  open: boolean
  onClose: () => void
  /** closing is refused while rows are being sent */
  closable: boolean
  file: { name: string; rows: number } | null
  steps: ShellStep[]
  current: StepId
  onSelect: (step: StepId) => void
  /** nothing to move between: no file yet, or the import is running */
  locked: boolean
  /** "Step 2 of 6 · Date", null on the file step */
  position: string | null
  question: string
  /** "12 transactions ready · 1 left out", once a file is read */
  ready: string | null
  /** the phone's overview shows the step list; a step opens over it */
  phoneList: boolean
  onPhoneBack: () => void
  phoneCheck: { label: string; onClick: () => void } | null
  actions: ReactNode
  /** progress and errors, above the actions */
  status?: ReactNode
  children: ReactNode
}

function StepList({ steps, current, onSelect, locked, phone }: Pick<WizardShellProps, 'steps' | 'current' | 'onSelect' | 'locked'> & { phone: boolean }) {
  const { t } = useTranslation()
  const baseId = useId()
  return (
    <ol>
      {steps.map((step, index) => {
        const active = !phone && step.id === current
        const id = `${baseId}-${step.id}`
        const done = !active && step.answered && !step.warning
        return (
          <li key={step.id} className="relative">
            {index < steps.length - 1 ? <span aria-hidden className="absolute top-10 bottom-0 left-[1.45rem] w-px bg-border" /> : null}
            <button
              type="button"
              disabled={locked}
              aria-current={step.id === current ? 'step' : undefined}
              aria-labelledby={step.warning ? `${id}-title ${id}-warning` : `${id}-title`}
              aria-describedby={`${id}-summary`}
              onClick={() => onSelect(step.id)}
              className={cn(
                'relative flex w-full items-start gap-3 rounded-xl px-2.5 py-2.5 text-left transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring/50 disabled:opacity-60',
                active ? 'bg-background shadow-sm' : 'hover:bg-background/60',
              )}
            >
              <span
                aria-hidden
                className={cn(
                  'flex size-6 shrink-0 items-center justify-center rounded-full text-xs',
                  active
                    ? 'bg-primary text-primary-foreground'
                    : step.warning
                      ? 'bg-econumo-yellow/40 text-econumo-yellow-text'
                      : done
                        ? 'bg-avatar-emerald text-white'
                        : 'bg-background text-muted-foreground ring-1 ring-border',
                )}
              >
                {step.warning && !active ? <AlertTriangle className="size-3.5" /> : done ? <Check className="size-3.5" /> : index + 1}
              </span>
              <span className="min-w-0 flex-1">
                <span id={`${id}-title`} className={cn('block text-sm', active ? 'text-foreground' : 'text-foreground/80')}>
                  {step.title}
                </span>
                {step.warning ? (
                  <span id={`${id}-warning`} className="sr-only">
                    , {t('transactions.import_wizard.needs_attention')}
                  </span>
                ) : null}
                <span id={`${id}-summary`} className="mt-0.5 block truncate text-xs text-muted-foreground">
                  {step.summary}
                </span>
              </span>
              {phone ? <ChevronRight aria-hidden className="mt-1 size-4 shrink-0 text-muted-foreground" /> : null}
            </button>
          </li>
        )
      })}
    </ol>
  )
}

function FileChip({ file, current, onSelect, locked, phone }: Pick<WizardShellProps, 'file' | 'current' | 'onSelect' | 'locked'> & { phone: boolean }) {
  const { t, i18n } = useTranslation()
  const label = t('transactions.import_wizard.steps.file')
  return (
    <button
      type="button"
      disabled={locked}
      aria-current={current === 'file' ? 'step' : undefined}
      aria-label={file ? `${label}: ${file.name}` : label}
      onClick={() => onSelect('file')}
      className={cn(
        'flex w-full min-w-0 items-center gap-2 rounded-xl px-3 py-2 text-left text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring/50 disabled:opacity-60',
        phone ? 'bg-econumo-card' : 'bg-background',
        current === 'file' && 'ring-1 ring-primary',
      )}
    >
      <FileSpreadsheet aria-hidden className="size-4 shrink-0 text-avatar-emerald" />
      <span className="truncate">{file ? file.name : t('transactions.import_wizard.file.none')}</span>
      {file ? (
        <span className="ml-auto shrink-0 text-xs text-muted-foreground">{pluralPick(t('transactions.import_wizard.rows'), file.rows, i18n.language)}</span>
      ) : null}
    </button>
  )
}

function CloseButton({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation()
  return (
    <Button type="button" variant="ghost" size="icon-sm" className="ml-auto text-muted-foreground" aria-label={t('common.button.close.label')} onClick={onClose}>
      <X />
    </Button>
  )
}

/** The import dialog's frame: the step list beside the open step on a wide screen; on a phone the list first, a step over it */
export function WizardShell(props: WizardShellProps) {
  const { open, onClose, closable, file, steps, current, onSelect, locked, position, question, ready, phoneList, onPhoneBack, phoneCheck, actions, status, children } =
    props
  const { t } = useTranslation()
  const isPhone = useIsPhone()
  const title = t('transactions.import_wizard.title')
  const nav = (phone: boolean) => (
    <nav aria-label={t('transactions.import_wizard.steps_label')} className="flex min-h-0 flex-1 flex-col">
      <div className={phone ? 'pt-3' : 'px-5 pt-3'}>
        <FileChip file={file} current={current} onSelect={onSelect} locked={locked} phone={phone} />
      </div>
      {phone ? <p className="pt-4 pb-2 text-xs text-muted-foreground">{t('transactions.import_wizard.list_hint')}</p> : null}
      <div className={phone ? 'py-2' : 'flex-1 overflow-y-auto px-3 py-3'}>
        <StepList steps={steps} current={current} onSelect={onSelect} locked={locked || file === null} phone={phone} />
      </div>
    </nav>
  )
  const heading = (
    <div className="min-w-0">
      {position ? <div className="text-xs text-muted-foreground">{position}</div> : null}
      <h2 className="mt-1 text-lg md:text-2xl">{question}</h2>
    </div>
  )

  if (isPhone) {
    const showList = phoneList && file !== null
    return (
      <ResponsiveDialog
        open={open}
        onOpenChange={(o) => !o && onClose()}
        dismissible={closable}
        title={title}
        hideHeader
        fullScreen
        footer={
          showList ? (
            phoneCheck ? (
              <Button type="button" className="w-full" disabled={locked} onClick={phoneCheck.onClick}>
                {phoneCheck.label}
              </Button>
            ) : undefined
          ) : (
            <div className="flex flex-col gap-2">
              {status}
              <div className="flex items-center gap-2">{actions}</div>
            </div>
          )
        }
      >
        <div className="flex min-h-full flex-col pt-[max(env(safe-area-inset-top),0.75rem)]">
          {showList ? (
            <>
              <div className="flex items-center gap-2">
                <span className="text-lg">{title}</span>
                {closable ? <CloseButton onClose={onClose} /> : null}
              </div>
              {nav(true)}
            </>
          ) : (
            <>
              <header className="-mx-4 mb-4 flex items-start gap-2 border-b border-border px-4 pb-3">
                {file !== null ? (
                  <Button type="button" variant="ghost" size="icon-sm" className="-ml-1" disabled={locked} aria-label={t('transactions.import_wizard.all_steps')} onClick={onPhoneBack}>
                    <ArrowLeft />
                  </Button>
                ) : null}
                {heading}
                {file === null && closable ? <CloseButton onClose={onClose} /> : null}
              </header>
              {children}
            </>
          )}
        </div>
      </ResponsiveDialog>
    )
  }

  return (
    <ResponsiveDialog open={open} onOpenChange={(o) => !o && onClose()} dismissible={closable} title={title} hideHeader size="split">
      <aside className="flex w-64 shrink-0 flex-col border-r border-border bg-econumo-card/70 md:w-80">
        <div className="flex items-center gap-2 px-5 pt-5">
          <span className="text-lg">{title}</span>
          {closable ? <CloseButton onClose={onClose} /> : null}
        </div>
        {nav(false)}
        {ready ? <div className="border-t border-border px-5 py-4 text-xs text-muted-foreground">{ready}</div> : null}
      </aside>
      <section className="flex min-w-0 flex-1 flex-col">
        <header className="border-b border-border px-8 py-6">{heading}</header>
        <div className="min-h-0 flex-1 overflow-y-auto px-8 py-5">{children}</div>
        <footer className="flex flex-col gap-2 border-t border-border px-8 py-3">
          {status}
          <div className="flex items-center gap-2">{actions}</div>
        </footer>
      </section>
    </ResponsiveDialog>
  )
}
