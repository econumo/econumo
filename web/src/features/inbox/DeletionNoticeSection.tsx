import { useTranslation } from 'react-i18next'
import type { ConnectionDto } from '@/api/dto/connection'
import { UserAvatar } from '@/components/UserAvatar'
import { isMarkedForDeletion } from './useInbox'

export function DeletionNoticeSection({ connections }: { connections: ConnectionDto[] }) {
  const { t } = useTranslation()
  const marked = connections.filter(isMarkedForDeletion)
  if (marked.length === 0) return null
  return (
    <div className="flex flex-col gap-2">
      {marked.map((connection) => (
        <div key={connection.user.id} data-testid="deletion-notice" className="flex items-center gap-3 rounded-lg bg-econumo-card p-3">
          <UserAvatar avatar={connection.user.avatar} size="md" />
          <p className="min-w-0 text-sm">
            {t('inbox.sharing.scheduled_for_deletion', { name: connection.user.name })}
          </p>
        </div>
      ))}
    </div>
  )
}
