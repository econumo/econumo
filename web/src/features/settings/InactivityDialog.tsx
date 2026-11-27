import { Check } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { useOnline } from '@/hooks/useOnline'
import { apiErrorMessage } from '@/lib/apiError'
import { pluralPick } from '@/lib/plural'
import i18n from '@/app/i18n'
import { useUpdateInactivityPeriod } from '@/features/user/queries'

// Picked like the language list: a tap saves and closes. A current period outside the choices
// (36 months seeded for lifetime accounts, or a choice under an older server setting) is listed
// so the user sees it, but the server refuses it, so tapping it only closes.
export function InactivityDialog({ open, onClose, months, choices }: {
  open: boolean
  onClose: () => void
  months: number
  choices: number[]
}) {
  const { t } = useTranslation()
  const online = useOnline()
  const update = useUpdateInactivityPeriod()
  const entries = choices.includes(months) ? choices : [...choices, months].sort((a, b) => a - b)

  const close = () => {
    update.reset()
    onClose()
  }

  const pick = (value: number) => {
    if (value === months) {
      close()
      return
    }
    update.mutate(value, { onSuccess: close })
  }

  return (
    <ResponsiveDialog open={open} onOpenChange={(o) => !o && close()} title={t('user.inactivity.header')}>
      <div className="flex flex-col gap-4">
        <p className="text-sm text-muted-foreground">{t('user.inactivity.information')}</p>
        <div className="flex flex-col gap-2 [&_button]:h-11">
          {entries.map((value) => (
            <Button
              key={value}
              type="button"
              variant={value === months ? 'secondary' : 'ghost'}
              className="justify-between"
              aria-pressed={value === months}
              disabled={!online || update.isPending}
              onClick={() => pick(value)}
            >
              {pluralPick(t('user.inactivity.months'), value, i18n.language)}
              {value === months ? <Check className="size-4" /> : null}
            </Button>
          ))}
        </div>
        {update.isError ? <p role="alert" className="text-sm text-destructive">{apiErrorMessage(update.error)}</p> : null}
      </div>
    </ResponsiveDialog>
  )
}
