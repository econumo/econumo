import { Link, useLocation } from 'react-router'
import { useTranslation } from 'react-i18next'
import { RouterPage } from '@/app/router-pages'
import { buttonVariants } from '@/components/ui/button'
import { pluralPick } from '@/lib/plural'
import { useImportQueue } from './queries'

export function ImportQueueBanner() {
  const { t, i18n } = useTranslation()
  const { pathname } = useLocation()
  const { data } = useImportQueue()
  const count = data?.queued.length ?? 0
  if (count === 0 || pathname === RouterPage.IMPORT_QUEUE) {
    return null
  }
  return (
    <div className="flex items-center gap-3 bg-primary/10 px-4 py-2 text-sm text-primary">
      <span className="min-w-0 flex-1">{pluralPick(t('imports.banner.text', { count }), count, i18n.language)}</span>
      <Link to={RouterPage.IMPORT_QUEUE} className={buttonVariants({ size: 'sm', variant: 'outline' })}>
        {t('imports.banner.cta')}
      </Link>
    </div>
  )
}
