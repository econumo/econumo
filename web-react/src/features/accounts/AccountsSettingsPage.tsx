import { useMemo, useState } from 'react'
import { DndContext, KeyboardSensor, PointerSensor, closestCenter, useSensor, useSensors, useDroppable } from '@dnd-kit/core'
import type { DragEndEvent, DragOverEvent, DragStartEvent } from '@dnd-kit/core'
import { SortableContext, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { EyeOff, FolderPlus, GripVertical, MoreVertical, PlusCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { EntityIcon } from '@/components/EntityIcon'
import { PromptDialog } from '@/components/PromptDialog'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { moneyFormat } from '@/lib/money'
import { getChangedPositions } from '@/lib/ordering'
import { isNotEmpty, isValidFolderName } from '@/lib/validation'
import { useIsCompact } from '@/hooks/useIsCompact'
import { useUiStore } from '@/app/uiStore'
import { RouterPage } from '@/app/router-pages'
import type { AccountDto } from '@/api/dto/account'
import type { FolderDto } from '@/api/dto/folder'
import { SettingsShell } from '@/features/settings/SettingsShell'
import { AccessLevelDialog } from '@/features/connections/AccessLevelDialog'
import { ShareAccessDialog } from '@/features/connections/ShareAccessDialog'
import type { ShareEntry } from '@/features/connections/shared'
import { buildShareEntries, hasAccountAdminAccess } from '@/features/connections/shared'
import { useConnections, useRevokeAccountAccess, useSetAccountAccess } from '@/features/connections/queries'
import { useUserData } from '@/features/user/queries'
import {
  useAccounts,
  useFolders,
  useCreateFolder,
  useUpdateFolder,
  useReplaceFolder,
  useHideFolder,
  useShowFolder,
  useOrderFolders,
  useOrderAccounts,
  useDeleteAccount,
} from './queries'
import type { FolderBucket } from './accountOrdering'
import { bucketsFromAccounts, moveAccount, buildAccountChanges } from './accountOrdering'

function AccountRow({
  account,
  showAccess,
  onMenu,
}: {
  account: AccountDto
  showAccess: boolean
  onMenu: (action: 'edit' | 'delete' | 'view' | 'access') => void
}) {
  const { t } = useTranslation()
  const isCompact = useIsCompact()
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: account.id })
  return (
    <li
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={`flex items-center gap-2 rounded-md px-1 py-1.5 ${isDragging ? 'opacity-60' : ''}`}
    >
      <button type="button" aria-label={`drag ${account.name}`} className="cursor-grab text-muted-foreground" {...attributes} {...(listeners ?? {})}>
        <GripVertical className="size-4" />
      </button>
      <button
        type="button"
        className="flex min-w-0 flex-1 items-center gap-3 text-left"
        onClick={() => (isCompact ? onMenu('view') : undefined)}
      >
        <span className="grid size-9 shrink-0 place-items-center rounded-lg bg-econumo-card">
          <EntityIcon name={account.icon} className="text-lg text-[#666666]" />
        </span>
        <span className="flex min-w-0 flex-col">
          <span className="truncate text-sm leading-tight" title={account.name}>
            {account.name}
          </span>
          <span className="text-[13px] leading-tight text-muted-foreground">{moneyFormat(account.balance, account.currency)}</span>
        </span>
      </button>
      {account.sharedAccess.length > 0 ? (
        <span className="flex items-center -space-x-2" data-testid={`shared-avatars-${account.name}`}>
          <img src={`${account.owner.avatar}?s=50`} alt={account.owner.name} className="size-7 rounded-full ring-2 ring-background" />
          {account.sharedAccess.map((entry) => (
            <img key={entry.user.id} src={`${entry.user.avatar}?s=50`} alt={entry.user.name} className="size-7 rounded-full ring-2 ring-background" />
          ))}
        </span>
      ) : null}
      {!isCompact ? (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button type="button" variant="ghost" size="icon" aria-label={`account actions ${account.name}`}>
              <MoreVertical className="size-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onSelect={() => onMenu('edit')}>{t('elements.button.edit.label')}</DropdownMenuItem>
            {showAccess ? (
              <DropdownMenuItem onSelect={() => onMenu('access')}>
                {t('pages.settings.accounts.list_actions.access')}
              </DropdownMenuItem>
            ) : null}
            <DropdownMenuItem variant="destructive" onSelect={() => onMenu('delete')}>
              {t('elements.button.delete.label')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      ) : null}
    </li>
  )
}

function FolderSection({
  folder,
  accounts,
  index,
  total,
  onAction,
  children,
}: {
  folder: FolderDto
  accounts: AccountDto[]
  index: number
  total: number
  onAction: (action: 'add' | 'up' | 'down' | 'rename' | 'hide' | 'show' | 'delete') => void
  children: React.ReactNode
}) {
  const { t } = useTranslation()
  const { setNodeRef } = useDroppable({ id: `folder:${folder.id}` })
  return (
    <section ref={setNodeRef} className="rounded-md border p-2" data-testid={`folder-${folder.name}`}>
      <header className="flex items-center gap-2 px-1 pb-1">
        <span className="flex-1 truncate text-sm font-medium" title={folder.name}>
          {folder.name}
        </span>
        {folder.isVisible === 0 ? <EyeOff className="size-4 text-muted-foreground" aria-label="hidden" /> : null}
        <Button
          type="button"
          variant="ghost"
          size="icon"
          aria-label={`add account to ${folder.name}`}
          onClick={() => onAction('add')}
        >
          <PlusCircle className="size-4" />
        </Button>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button type="button" variant="ghost" size="icon" aria-label={`folder actions ${folder.name}`}>
              <MoreVertical className="size-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            {index > 0 ? <DropdownMenuItem onSelect={() => onAction('up')}>{t('elements.button.up.label')}</DropdownMenuItem> : null}
            {index < total - 1 ? (
              <DropdownMenuItem onSelect={() => onAction('down')}>{t('elements.button.down.label')}</DropdownMenuItem>
            ) : null}
            <DropdownMenuItem onSelect={() => onAction('rename')}>{t('elements.button.edit.label')}</DropdownMenuItem>
            <DropdownMenuItem onSelect={() => onAction(folder.isVisible === 1 ? 'hide' : 'show')}>
              {folder.isVisible === 1 ? t('elements.button.hide.label') : t('elements.button.show.label')}
            </DropdownMenuItem>
            {index > 0 ? (
              <DropdownMenuItem variant="destructive" onSelect={() => onAction('delete')}>
                {t('elements.button.delete.label')}
              </DropdownMenuItem>
            ) : null}
          </DropdownMenuContent>
        </DropdownMenu>
      </header>
      <SortableContext items={accounts.map((a) => a.id)} strategy={verticalListSortingStrategy}>
        <ul>{children}</ul>
      </SortableContext>
    </section>
  )
}

export function AccountsSettingsPage() {
  const { t } = useTranslation()
  const { data: accounts = [] } = useAccounts()
  const { data: folders = [] } = useFolders()
  const { data: user } = useUserData()
  const { data: connections = [] } = useConnections()
  const openAccountModal = useUiStore((s) => s.openAccountModal)
  const setAccountAccess = useSetAccountAccess()
  const revokeAccountAccess = useRevokeAccountAccess()

  const createFolder = useCreateFolder()
  const updateFolder = useUpdateFolder()
  const replaceFolder = useReplaceFolder()
  const hideFolder = useHideFolder()
  const showFolder = useShowFolder()
  const orderFolders = useOrderFolders()
  const orderAccounts = useOrderAccounts()
  const deleteAccount = useDeleteAccount()

  const [createOpen, setCreateOpen] = useState(false)
  const [renameTarget, setRenameTarget] = useState<FolderDto | null>(null)
  const [deleteFolderTarget, setDeleteFolderTarget] = useState<FolderDto | null>(null)
  const [deleteAccountTarget, setDeleteAccountTarget] = useState<AccountDto | null>(null)
  const [previewAccount, setPreviewAccount] = useState<AccountDto | null>(null)
  const [accessAccountId, setAccessAccountId] = useState<string | null>(null)
  const [levelEntry, setLevelEntry] = useState<ShareEntry | null>(null)

  // read the live cache copy so optimistic grant/revoke updates show immediately
  const accessAccount = accessAccountId ? accounts.find((a) => a.id === accessAccountId) ?? null : null

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )

  const folderIds = useMemo(() => folders.map((f) => f.id), [folders])
  const serverBuckets = useMemo(() => bucketsFromAccounts(accounts, folderIds), [accounts, folderIds])

  // Live drag preview: while dragging (and until the reorder round-trip lands)
  // the list renders from this local arrangement so the drop never snaps back.
  const [dragBuckets, setDragBuckets] = useState<FolderBucket[] | null>(null)
  const buckets = dragBuckets ?? serverBuckets

  const accountsById = useMemo(() => new Map(accounts.map((a) => [a.id, a])), [accounts])
  const bucketAccounts = (folderId: string): AccountDto[] =>
    (buckets.find((b) => b.folderId === folderId)?.accountIds ?? [])
      .map((id) => accountsById.get(id))
      .filter((a): a is AccountDto => a !== undefined)

  const folderNameValidator = (value: string): string | null => {
    if (!isNotEmpty(value)) {
      return t('elements.form.account.folder.validation.empty_name')
    }
    if (!isValidFolderName(value)) {
      return t('elements.form.account.folder.validation.error_name_length')
    }
    return null
  }

  const handleFolderAction = (folder: FolderDto, index: number, action: 'add' | 'up' | 'down' | 'rename' | 'hide' | 'show' | 'delete') => {
    if (action === 'add') {
      openAccountModal({ folderId: folder.id })
    } else if (action === 'rename') {
      setRenameTarget(folder)
    } else if (action === 'hide') {
      hideFolder.mutate(folder.id)
    } else if (action === 'show') {
      showFolder.mutate(folder.id)
    } else if (action === 'delete') {
      setDeleteFolderTarget(folder)
    } else {
      const swapWith = action === 'up' ? index - 1 : index + 1
      const orderedIds = folders.map((f) => f.id)
      ;[orderedIds[index], orderedIds[swapWith]] = [orderedIds[swapWith], orderedIds[index]]
      const changes = getChangedPositions(folders, orderedIds)
      if (changes.length > 0) {
        orderFolders.mutate(changes)
      }
    }
  }

  const handleDragStart = (_event: DragStartEvent) => {
    setDragBuckets(serverBuckets)
  }

  const handleDragOver = (event: DragOverEvent) => {
    const { active, over } = event
    if (!over || active.id === over.id) {
      return
    }
    setDragBuckets((prev) => moveAccount(prev ?? serverBuckets, String(active.id), String(over.id)))
  }

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event
    const final =
      over && active.id !== over.id
        ? moveAccount(dragBuckets ?? serverBuckets, String(active.id), String(over.id))
        : dragBuckets ?? serverBuckets
    const changes = buildAccountChanges(accounts, final)
    if (changes.length === 0) {
      setDragBuckets(null)
      return
    }
    // keep the preview until the server echoes the new order (or roll back on error)
    setDragBuckets(final)
    orderAccounts.mutate(changes, { onSettled: () => setDragBuckets(null) })
  }

  return (
    <SettingsShell
      title={t('pages.settings.accounts.header')}
      backTo={RouterPage.SETTINGS}
      actions={
        <Button type="button" size="sm" onClick={() => setCreateOpen(true)}>
          <FolderPlus className="size-4" />
          <span className="hidden sm:inline">{t('pages.settings.accounts.create_folder')}</span>
        </Button>
      }
    >
      {accounts.length === 0 ? (
        <button type="button" className="px-1 py-2 text-sm text-primary underline" onClick={() => openAccountModal({ folderId: folders[0]?.id ?? null })}>
          {t('pages.settings.accounts.list_empty_create')} {t('pages.settings.accounts.list_empty_new_account')}
        </button>
      ) : null}

      <DndContext
        sensors={sensors}
        collisionDetection={closestCenter}
        onDragStart={handleDragStart}
        onDragOver={handleDragOver}
        onDragEnd={handleDragEnd}
        onDragCancel={() => setDragBuckets(null)}
      >
        <div className="flex flex-col gap-3">
          {folders.map((folder, index) => (
            <FolderSection
              key={folder.id}
              folder={folder}
              index={index}
              total={folders.length}
              accounts={bucketAccounts(folder.id)}
              onAction={(action) => handleFolderAction(folder, index, action)}
            >
              {bucketAccounts(folder.id)
                .map((account) => (
                  <AccountRow
                    key={account.id}
                    account={account}
                    showAccess={user ? hasAccountAdminAccess(account, user.id) : false}
                    onMenu={(action) => {
                      if (action === 'edit') {
                        openAccountModal({ account })
                      } else if (action === 'delete') {
                        setDeleteAccountTarget(account)
                      } else if (action === 'access') {
                        setAccessAccountId(account.id)
                      } else {
                        setPreviewAccount(account)
                      }
                    }}
                  />
                ))}
            </FolderSection>
          ))}
        </div>
      </DndContext>

      <ShareAccessDialog
        open={accessAccount !== null && levelEntry === null}
        title={accessAccount?.name ?? ''}
        kind="accounts"
        entries={accessAccount && user ? buildShareEntries(connections, accessAccount.sharedAccess, user.id, accessAccount.owner.id) : []}
        onPick={(entry) => {
          if (entry.role !== 'owner') {
            setLevelEntry(entry)
          }
        }}
        onClose={() => setAccessAccountId(null)}
      />

      <AccessLevelDialog
        open={levelEntry !== null}
        kind="accounts"
        user={levelEntry?.user ?? null}
        role={levelEntry?.role ?? null}
        onSelect={(role) => {
          if (levelEntry && accessAccountId) {
            setAccountAccess.mutate({ accountId: accessAccountId, userId: levelEntry.user.id, role })
          }
          setLevelEntry(null)
        }}
        onRevoke={() => {
          if (levelEntry && accessAccountId) {
            revokeAccountAccess.mutate({ accountId: accessAccountId, userId: levelEntry.user.id })
          }
          setLevelEntry(null)
        }}
        onClose={() => setLevelEntry(null)}
      />

      <PromptDialog
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        onSubmit={(name) => {
          createFolder.mutate(name, { onSuccess: () => setCreateOpen(false) })
        }}
        title={t('pages.settings.accounts.create_folder_modal.header')}
        inputLabel={t('elements.form.account.folder.label')}
        validate={folderNameValidator}
        submitLabel={t('elements.button.create.label')}
        cancelLabel={t('elements.button.cancel.label')}
      />

      <PromptDialog
        open={renameTarget !== null}
        onClose={() => setRenameTarget(null)}
        onSubmit={(name) => {
          if (renameTarget) {
            updateFolder.mutate({ id: renameTarget.id, name }, { onSuccess: () => setRenameTarget(null) })
          }
        }}
        title={t('pages.settings.accounts.update_folder_modal.header')}
        inputLabel={t('elements.form.account.folder.label')}
        initialValue={renameTarget?.name ?? ''}
        validate={folderNameValidator}
        submitLabel={t('elements.button.update.label')}
        cancelLabel={t('elements.button.cancel.label')}
      />

      <ConfirmDialog
        open={deleteFolderTarget !== null}
        onClose={() => setDeleteFolderTarget(null)}
        onConfirm={() => {
          if (deleteFolderTarget) {
            const fallback = [...folders].reverse().find((f) => f.id !== deleteFolderTarget.id)
            if (fallback) {
              replaceFolder.mutate({ id: deleteFolderTarget.id, replaceId: fallback.id }, { onSettled: () => setDeleteFolderTarget(null) })
            }
          }
        }}
        title={t('pages.settings.accounts.delete_folder_modal.title')}
        question={t('pages.settings.accounts.delete_folder_modal.question', { folder: deleteFolderTarget?.name ?? '' })}
        confirmLabel={t('elements.button.delete.label')}
        cancelLabel={t('elements.button.cancel.label')}
      />

      <ConfirmDialog
        open={deleteAccountTarget !== null}
        onClose={() => setDeleteAccountTarget(null)}
        onConfirm={() => {
          if (deleteAccountTarget) {
            deleteAccount.mutate(deleteAccountTarget.id, { onSettled: () => setDeleteAccountTarget(null) })
          }
        }}
        question={t('pages.settings.accounts.delete_account_modal.question', { account: deleteAccountTarget?.name ?? '' })}
        confirmLabel={t('elements.button.delete.label')}
        cancelLabel={t('elements.button.cancel.label')}
      />

      {previewAccount ? (
        <ResponsiveDialog
          open
          onOpenChange={(o) => !o && setPreviewAccount(null)}
          title={t('pages.settings.accounts.preview_account_modal.header')}
        >
          <div className="flex items-center gap-3">
            <EntityIcon name={previewAccount.icon} className="text-2xl text-muted-foreground" />
            <span className="flex min-w-0 flex-col">
              <span className="truncate text-sm font-medium">{previewAccount.name}</span>
              <span className="text-xs text-muted-foreground">
                {moneyFormat(previewAccount.balance, previewAccount.currency, { useNativePrecision: false })}
              </span>
            </span>
          </div>
          <div className="mt-4 grid grid-cols-2 gap-3">
            <Button
              type="button"
              variant="destructive"
              onClick={() => {
                setDeleteAccountTarget(previewAccount)
                setPreviewAccount(null)
              }}
            >
              {t('elements.button.delete.label')}
            </Button>
            <Button
              type="button"
              onClick={() => {
                openAccountModal({ account: previewAccount })
                setPreviewAccount(null)
              }}
            >
              {t('elements.button.edit.label')}
            </Button>
          </div>
        </ResponsiveDialog>
      ) : null}
    </SettingsShell>
  )
}
