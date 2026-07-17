import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import type { SortListForm } from '@/api/category'

interface SortDialogProps {
  open: boolean
  onClose: () => void
  onPick: (form: SortListForm) => void
}

const USAGE_PERIOD_KEY = 'econumo.sort.usagePeriodMonths'
const PERIODS = [1, 2, 3, 4, 5, 6]

function storedPeriod(): number {
  const raw = Number(localStorage.getItem(USAGE_PERIOD_KEY))
  return PERIODS.includes(raw) ? raw : 3
}

export function SortDialog({ open, onClose, onPick }: SortDialogProps) {
  const { t } = useTranslation()
  const [period, setPeriod] = useState(storedPeriod)

  const pickUsage = (direction: 'asc' | 'desc') => {
    localStorage.setItem(USAGE_PERIOD_KEY, String(period))
    onPick({ by: 'usage', direction, periodMonths: period })
  }

  return (
    <ResponsiveDialog open={open} onOpenChange={(o) => !o && onClose()} title={t('common.sort.header')}>
      <div className="flex flex-col gap-2 [&>button]:h-11">
        <Button type="button" variant="secondary" onClick={() => onPick({ by: 'name', direction: 'asc' })}>
          {t('common.sort.mode.alphabet.asc')}
        </Button>
        <Button type="button" variant="secondary" onClick={() => onPick({ by: 'name', direction: 'desc' })}>
          {t('common.sort.mode.alphabet.desc')}
        </Button>
        <Button type="button" variant="secondary" onClick={() => pickUsage('desc')}>
          {t('common.sort.mode.usage.desc')}
        </Button>
        <Button type="button" variant="secondary" onClick={() => pickUsage('asc')}>
          {t('common.sort.mode.usage.asc')}
        </Button>
        <div className="flex items-center justify-between gap-2 px-1 text-sm text-muted-foreground">
          <span>{t('common.sort.period')}</span>
          <span className="flex gap-1">
            {PERIODS.map((p) => (
              <Button
                key={p}
                type="button"
                size="sm"
                variant={p === period ? 'default' : 'ghost'}
                onClick={() => setPeriod(p)}
              >
                {p}
              </Button>
            ))}
          </span>
        </div>
        <Button type="button" variant="ghost" onClick={onClose}>
          {t('common.button.cancel.label')}
        </Button>
      </div>
    </ResponsiveDialog>
  )
}
