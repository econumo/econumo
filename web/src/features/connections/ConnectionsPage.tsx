import { useState } from 'react'
import { MoreVertical, UserPlus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { UserAvatar } from '@/components/UserAvatar'
import type { ConnectionDto, InviteDto } from '@/api/dto/connection'
import { RouterPage } from '@/app/router-pages'
import { apiErrorMessage } from '@/lib/apiError'
import { SettingsShell } from '@/features/settings/SettingsShell'
import { GenerateInviteDialog } from './GenerateInviteDialog'
import { AcceptInviteDialog } from './AcceptInviteDialog'
import { PreviewConnectionDialog } from './PreviewConnectionDialog'
import { useAcceptInvite, useConnections, useDeleteConnection, useGenerateInvite } from './queries'

export function ConnectionsPage() {
  const { t } = useTranslation()
  const { data: connections = [] } = useConnections({ poll: true })
  const generateInvite = useGenerateInvite()
  const acceptInvite = useAcceptInvite()
  const deleteConnection = useDeleteConnection()

  const [invite, setInvite] = useState<InviteDto | null>(null)
  const [acceptOpen, setAcceptOpen] = useState(false)
  const [acceptError, setAcceptError] = useState<string | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<ConnectionDto | null>(null)
  const [preview, setPreview] = useState<ConnectionDto | null>(null)

  const openAccept = () => {
    setAcceptError(null)
    setAcceptOpen(true)
  }

  return (
    <SettingsShell
      title={t('connections.pages.settings.header')}
      backTo={RouterPage.SETTINGS}
      actions={
        <div className="flex gap-2">
          <Button
            type="button"
            size="sm"
            aria-label={t('connections.pages.settings.generate_invite')}
            title={t('connections.pages.settings.generate_invite')}
            onClick={() => generateInvite.mutate(undefined, { onSuccess: setInvite })}
          >
            <UserPlus className="size-4" />
            <span className="hidden sm:inline">{t('connections.pages.settings.generate_invite')}</span>
          </Button>
          <Button type="button" size="sm" variant="secondary" onClick={openAccept}>
            {t('connections.pages.settings.accept_invite')}
          </Button>
        </div>
      }
    >
      {connections.length === 0 ? (
        <p className="px-1 py-2 text-sm text-muted-foreground">{t('common.list.list_empty')}</p>
      ) : (
        <ul className="flex max-w-md flex-col gap-2">
          {connections.map((connection) => (
            <li key={connection.user.id} className="flex items-center gap-3 rounded-lg bg-econumo-card px-3 py-2 hover:bg-econumo-hover">
              <button
                type="button"
                className="flex min-w-0 flex-1 items-center gap-3 text-left"
                onClick={() => setPreview(connection)}
              >
                <UserAvatar avatar={connection.user.avatar} size="md" />
                <span className="min-w-0 flex-1 truncate text-sm" title={connection.user.name}>
                  {connection.user.name}
                </span>
              </button>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button type="button" variant="ghost" size="icon" aria-label={`connection actions ${connection.user.name}`}>
                    <MoreVertical className="size-4" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem onSelect={() => setPreview(connection)}>{t('common.button.view.label')}</DropdownMenuItem>
                  <DropdownMenuItem variant="destructive" onSelect={() => setDeleteTarget(connection)}>
                    {t('common.button.delete.label')}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </li>
          ))}
        </ul>
      )}

      <PreviewConnectionDialog
        open={preview !== null}
        connection={preview}
        onDelete={(userId) => {
          const target = connections.find((c) => c.user.id === userId) ?? null
          setPreview(null)
          setDeleteTarget(target)
        }}
        onClose={() => setPreview(null)}
      />

      <GenerateInviteDialog open={invite !== null} code={invite?.code ?? ''} onClose={() => setInvite(null)} />

      <AcceptInviteDialog
        open={acceptOpen}
        pending={acceptInvite.isPending}
        error={acceptError}
        onSubmit={(code) =>
          acceptInvite.mutate(code, {
            onSuccess: () => {
              setAcceptOpen(false)
              setAcceptError(null)
            },
            onError: (error) => setAcceptError(apiErrorMessage(error)),
          })
        }
        onClose={() => setAcceptOpen(false)}
      />

      <ConfirmDialog
        open={deleteTarget !== null}
        onClose={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (deleteTarget) {
            deleteConnection.mutate(deleteTarget.user.id, { onSettled: () => setDeleteTarget(null) })
          }
        }}
        question={t('connections.modals.delete_connection.question', { name: deleteTarget?.user.name ?? '' })}
        confirmLabel={t('common.button.delete.label')}
        cancelLabel={t('common.button.cancel.label')}
        destructive
      />
    </SettingsShell>
  )
}
