import { useMemo, useState } from 'react'
import { DndContext, DragOverlay, KeyboardSensor, MeasuringStrategy, PointerSensor, closestCenter, useSensor, useSensors, useDroppable } from '@dnd-kit/core'
import type { DragEndEvent, DragOverEvent, DragStartEvent } from '@dnd-kit/core'
import { SortableContext, arrayMove, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { ChevronDown, EyeOff, FolderPlus, GripVertical, MoreVertical, PlusCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { EntityIcon } from '@/components/EntityIcon'
import { InfoBox } from '@/components/InfoBox'
import { PromptDialog } from '@/components/PromptDialog'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { moneyFormat } from '@/lib/money'
import { getChangedPositions } from '@/lib/ordering'
import { getItem, setItem } from '@/lib/storage'
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
import { snapRowToPointer } from '@/lib/dnd'

const COLLAPSED_FOLDERS_KEY = 'settings.accounts.collapsedFolders'


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
  const [menuOpen, setMenuOpen] = useState(false)
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: account.id })
  return (
    <li ref={setNodeRef} style={{ transform: CSS.Transform.toString(transform), transition }} className={isDragging ? 'opacity-60' : undefined}>
      {/* the whole row is the click target: compact opens the preview sheet, desktop the context menu */}
      <div
        className={`flex items-center gap-2 rounded-md px-1 py-1.5 ${isCompact ? 'active:bg-econumo-hover' : 'cursor-pointer hover:bg-econumo-hover'}`}
        onClick={() => (isCompact ? onMenu('view') : setMenuOpen(true))}
      >
        <button
          type="button"
          aria-label={`drag ${account.name}`}
          className="cursor-grab touch-none text-muted-foreground"
          onClick={(e) => e.stopPropagation()}
          {...attributes}
          {...(listeners ?? {})}
        >
          <GripVertical className="size-4" />
        </button>
        <span className="grid size-9 shrink-0 place-items-center rounded-lg bg-econumo-card">
          <EntityIcon name={account.icon} className="text-lg text-[#666666]" />
        </span>
        <span className="flex min-w-0 flex-1 flex-col">
          <span className="truncate text-sm leading-tight" title={account.name}>
            {account.name}
          </span>
          <span className="text-[13px] leading-tight text-muted-foreground">{moneyFormat(account.balance, account.currency)}</span>
        </span>
        {account.sharedAccess.length > 0 ? (
          <span className="flex items-center -space-x-2" data-testid={`shared-avatars-${account.name}`}>
            <img src={`${account.owner.avatar}?s=50`} alt={account.owner.name} className="size-7 rounded-full ring-2 ring-background" />
            {account.sharedAccess.map((entry) => (
              <img key={entry.user.id} src={`${entry.user.avatar}?s=50`} alt={entry.user.name} className="size-7 rounded-full ring-2 ring-background" />
            ))}
          </span>
        ) : null}
        {!isCompact ? (
          <DropdownMenu open={menuOpen} onOpenChange={setMenuOpen}>
            <DropdownMenuTrigger asChild>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label={`account actions ${account.name}`}
                onClick={(e) => e.stopPropagation()}
              >
                <MoreVertical className="size-4" />
              </Button>
            </DropdownMenuTrigger>
            {/* portaled content still bubbles React clicks to the row — don't reopen the menu */}
            <DropdownMenuContent align="end" onClick={(e) => e.stopPropagation()}>
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
      </div>
    </li>
  )
}

function FolderSection({
  folder,
  accounts,
  index,
  total,
  collapsed,
  folderDragging,
  onToggleCollapse,
  onAction,
  children,
}: {
  folder: FolderDto
  accounts: AccountDto[]
  index: number
  total: number
  /** user-toggled collapse (persisted) */
  collapsed: boolean
  /** a folder drag is in progress: every folder renders collapsed */
  folderDragging: boolean
  onToggleCollapse: () => void
  onAction: (action: 'add' | 'up' | 'down' | 'rename' | 'hide' | 'show' | 'delete') => void
  children: React.ReactNode
}) {
  const { t } = useTranslation()
  const isCompact = useIsCompact()
  const [menuOpen, setMenuOpen] = useState(false)
  // the section is both a sortable item (folder reorder, desktop) and a drop
  // container for accounts; the account droppable pauses during a folder drag
  const { attributes, listeners, setNodeRef: setSortableRef, transform, transition, isDragging } = useSortable({
    id: folder.id,
    disabled: isCompact,
  })
  const { setNodeRef: setDroppableRef } = useDroppable({ id: `folder:${folder.id}`, disabled: folderDragging })
  const isHidden = folder.isVisible === 0
  const showAccounts = !collapsed && !folderDragging
  return (
    <section
      ref={(el) => {
        setSortableRef(el)
        setDroppableRef(el)
      }}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={`rounded-md border p-2 ${isHidden ? 'border-dashed bg-muted/50' : ''} ${isDragging ? 'opacity-40' : ''}`}
      data-testid={`folder-${folder.name}`}
    >
      <header
        className={`flex items-center gap-2 rounded-md px-1 ${isCompact ? '' : 'cursor-pointer hover:bg-econumo-hover'} ${showAccounts ? 'mb-1' : ''}`}
        onClick={() => (isCompact ? undefined : setMenuOpen(true))}
      >
        {!isCompact ? (
          <button
            type="button"
            aria-label={`drag folder ${folder.name}`}
            className="cursor-grab touch-none text-muted-foreground"
            onClick={(e) => e.stopPropagation()}
            {...attributes}
            {...(listeners ?? {})}
          >
            <GripVertical className="size-4" />
          </button>
        ) : null}
        <span className={`flex-1 truncate text-sm font-medium ${isHidden ? 'text-muted-foreground' : ''}`} title={folder.name}>
          {folder.name}
        </span>
        {!showAccounts && accounts.length > 0 ? (
          <span className="text-xs text-muted-foreground" data-testid={`folder-count-${folder.name}`}>
            {accounts.length}
          </span>
        ) : null}
        {isHidden ? <EyeOff className="size-4 text-muted-foreground" aria-label="hidden" /> : null}
        <Button
          type="button"
          variant="ghost"
          size="icon"
          aria-label={`toggle folder ${folder.name}`}
          title={t('pages.settings.accounts.folder_toggle')}
          onClick={(e) => {
            e.stopPropagation()
            onToggleCollapse()
          }}
        >
          <ChevronDown className={`size-4 transition-transform ${collapsed ? '-rotate-90' : ''}`} />
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          aria-label={`add account to ${folder.name}`}
          onClick={(e) => {
            e.stopPropagation()
            onAction('add')
          }}
        >
          <PlusCircle className="size-4" />
        </Button>
        <DropdownMenu open={menuOpen} onOpenChange={setMenuOpen}>
          <DropdownMenuTrigger asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={`folder actions ${folder.name}`}
              onClick={(e) => e.stopPropagation()}
            >
              <MoreVertical className="size-4" />
            </Button>
          </DropdownMenuTrigger>
          {/* portaled content still bubbles React clicks to the header — don't reopen the menu */}
          <DropdownMenuContent align="end" onClick={(e) => e.stopPropagation()}>
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
      {showAccounts ? (
        <SortableContext items={accounts.map((a) => a.id)} strategy={verticalListSortingStrategy}>
          <ul>{children}</ul>
        </SortableContext>
      ) : null}
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

  // While a folder itself is dragged, every section collapses to a bare header
  // so the reorder reads as a plain folder list.
  const [draggingFolderId, setDraggingFolderId] = useState<string | null>(null)
  const draggingFolder = draggingFolderId ? folders.find((f) => f.id === draggingFolderId) ?? null : null

  const [collapsedIds, setCollapsedIds] = useState<Set<string>>(
    () => new Set((getItem(COLLAPSED_FOLDERS_KEY) as string[] | null) ?? []),
  )
  const toggleCollapsed = (id: string) => {
    setCollapsedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      setItem(COLLAPSED_FOLDERS_KEY, [...next])
      return next
    })
  }

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

  const handleDragStart = (event: DragStartEvent) => {
    if (folderIds.includes(String(event.active.id))) {
      setDraggingFolderId(String(event.active.id))
      return
    }
    setDragBuckets(serverBuckets)
  }

  const handleDragOver = (event: DragOverEvent) => {
    const { active, over } = event
    // folder reorder previews via the sortable transforms, no bucket math needed
    if (draggingFolderId || !over || active.id === over.id) {
      return
    }
    setDragBuckets((prev) => moveAccount(prev ?? serverBuckets, String(active.id), String(over.id)))
  }

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event
    if (draggingFolderId) {
      setDraggingFolderId(null)
      const overId = over ? String(over.id).replace(/^folder:/, '') : null
      const from = folderIds.indexOf(draggingFolderId)
      const to = overId ? folderIds.indexOf(overId) : -1
      if (from === -1 || to === -1 || from === to) {
        return
      }
      const changes = getChangedPositions(folders, arrayMove(folderIds, from, to))
      if (changes.length > 0) {
        orderFolders.mutate(changes)
      }
      return
    }
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
      <InfoBox>{t('pages.settings.accounts.info')}</InfoBox>

      {accounts.length === 0 ? (
        <button type="button" className="px-1 py-2 text-sm text-primary underline" onClick={() => openAccountModal({ folderId: folders[0]?.id ?? null })}>
          {t('pages.settings.accounts.list_empty_create')} {t('pages.settings.accounts.list_empty_new_account')}
        </button>
      ) : null}

      <DndContext
        sensors={sensors}
        collisionDetection={closestCenter}
        // collapsing the folders on drag start reshuffles the layout, so the
        // droppable rects must be re-measured mid-drag, not cached from before
        measuring={{ droppable: { strategy: MeasuringStrategy.Always } }}
        modifiers={draggingFolderId ? [snapRowToPointer] : undefined}
        onDragStart={handleDragStart}
        onDragOver={handleDragOver}
        onDragEnd={handleDragEnd}
        onDragCancel={() => {
          setDragBuckets(null)
          setDraggingFolderId(null)
        }}
      >
        <SortableContext items={folderIds} strategy={verticalListSortingStrategy}>
          <div className="mt-3 flex flex-col gap-3">
            {folders.map((folder, index) => (
              <FolderSection
                key={folder.id}
                folder={folder}
                index={index}
                total={folders.length}
                accounts={bucketAccounts(folder.id)}
                collapsed={collapsedIds.has(folder.id)}
                folderDragging={draggingFolderId !== null}
                onToggleCollapse={() => toggleCollapsed(folder.id)}
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
        </SortableContext>
        {/* the pointer-following copy: the in-list section only marks the drop slot,
            so the collapse-on-grab layout shift cannot fling the dragged folder away */}
        <DragOverlay>
          {draggingFolder ? (
            <section
              className={`rounded-md border bg-background p-2 shadow-lg ${
                draggingFolder.isVisible === 0 ? 'border-dashed bg-muted/50' : ''
              }`}
            >
              <header className="flex items-center gap-2 px-1">
                <GripVertical className="size-4 cursor-grabbing text-muted-foreground" />
                <span
                  className={`flex-1 truncate text-sm font-medium ${
                    draggingFolder.isVisible === 0 ? 'text-muted-foreground' : ''
                  }`}
                >
                  {draggingFolder.name}
                </span>
                {bucketAccounts(draggingFolder.id).length > 0 ? (
                  <span className="text-xs text-muted-foreground">{bucketAccounts(draggingFolder.id).length}</span>
                ) : null}
              </header>
            </section>
          ) : null}
        </DragOverlay>
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
