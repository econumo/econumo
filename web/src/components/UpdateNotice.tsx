import { useState } from 'react'
import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useAvailableUpdate } from '@/hooks/useAvailableUpdate'
import { getDismissedUpdateVersion, setDismissedUpdateVersion } from '@/lib/version'

export function UpdateNotice() {
  const { t } = useTranslation()
  const update = useAvailableUpdate()
  const [dismissed, setDismissed] = useState<string | null>(getDismissedUpdateVersion)
  if (!update || dismissed === update.version) {
    return null
  }
  return (
    <div className="mx-3 mb-2 flex items-center gap-2 rounded-md bg-accent px-3 py-2 text-xs text-muted-foreground">
      <a
        href={update.url}
        target="_blank"
        rel="noreferrer"
        className="min-w-0 flex-1 truncate font-medium hover:text-foreground"
      >
        {t('common.update.notice', { version: update.version })}
      </a>
      <button
        type="button"
        aria-label={t('common.update.dismiss')}
        className="shrink-0 hover:text-foreground"
        onClick={() => {
          setDismissedUpdateVersion(update.version)
          setDismissed(update.version)
        }}
      >
        <X className="size-3.5" />
      </button>
    </div>
  )
}
