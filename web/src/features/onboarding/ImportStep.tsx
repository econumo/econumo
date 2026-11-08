import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { FileUp } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { ImportCsvDialog } from '@/features/transactions/ImportCsvDialog'
import type { StepBodyProps } from './OnboardingPage'

export function ImportStep({ onDone, onSkip }: StepBodyProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [pending, setPending] = useState(false)
  const skip = () => {
    setPending(true)
    void onSkip().finally(() => setPending(false))
  }
  return (
    <section className="flex flex-col gap-2">
      <h2 className="text-2xl font-normal">{t('onboarding.import_step.heading')}</h2>
      <p className="text-sm text-muted-foreground">{t('onboarding.import_step.text')}</p>
      <button
        type="button"
        className="mt-4 flex flex-col items-center gap-2 rounded-xl border-2 border-dashed border-primary/40 bg-primary/5 p-8 text-sm text-primary hover:bg-primary/10"
        onClick={() => setOpen(true)}
      >
        <FileUp className="size-6" />
        {t('onboarding.import_step.choose')}
      </button>
      <div className="mt-6 flex items-center justify-between">
        <Button type="button" variant="ghost" disabled={pending} onClick={skip}>
          {t('onboarding.import_step.skip')}
        </Button>
        <Button type="button" disabled={pending} onClick={() => setOpen(true)}>
          {t('onboarding.import_step.import')}
        </Button>
      </div>
      <ImportCsvDialog
        open={open}
        onClose={() => setOpen(false)}
        onComplete={(result) => {
          if (result.imported > 0) void onDone(['categories', 'accounts'])
        }}
      />
    </section>
  )
}
