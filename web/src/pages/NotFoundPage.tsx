import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { RouterPage } from '@/app/router-pages'

export function NotFoundPage() {
  const { t } = useTranslation()
  return (
    <div className="flex min-h-svh flex-col items-center justify-center gap-4">
      <h1 className="text-5xl font-bold">404</h1>
      <Link to={RouterPage.HOME} className="text-primary underline">
        {t('common.econumo.label')}
      </Link>
    </div>
  )
}
