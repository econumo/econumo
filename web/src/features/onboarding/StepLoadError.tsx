import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { apiErrorMessage } from '@/lib/apiError'

// a step built on a list it failed to load would offer duplicates of what the user already has
export function StepLoadError({ error, onRetry }: { error: unknown; onRetry: () => void }) {
  const { t } = useTranslation()
  return (
    <section className="flex flex-col items-start gap-3">
      <p role="alert" className="text-sm text-destructive">
        {apiErrorMessage(error)}
      </p>
      <Button type="button" variant="outline" onClick={onRetry}>
        {t('onboarding.retry')}
      </Button>
    </section>
  )
}
