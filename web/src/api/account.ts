import { api, apiUrl } from './client'
import type { Id } from './types'
import type { AccountDto, AccountItemDto, AccountRole } from './dto/account'
import type { FolderDto } from './dto/folder'

interface Envelope<T> {
  data: T
}

export interface CreateAccountForm {
  id: Id
  name: string
  currencyId: Id
  balance: string
  icon: string
  folderId: Id | null
}

export interface UpdateAccountForm {
  id: Id
  name: string
  balance: string
  icon: string
  currencyId: Id
  updatedAt: string
}

export interface AccountPositionChange {
  id: Id
  folderId: Id | null
  position: number
}

export async function getAccountList(): Promise<AccountDto[]> {
  const response = await api.get<Envelope<{ items: AccountDto[] }>>(apiUrl('/api/v1/account/get-account-list'))
  return response.data.data.items
}

export async function createAccount(form: CreateAccountForm): Promise<AccountItemDto> {
  const response = await api.post<Envelope<AccountItemDto>>(apiUrl('/api/v1/account/create-account'), form)
  return response.data.data
}

export async function updateAccount(form: UpdateAccountForm): Promise<AccountItemDto> {
  const response = await api.post<Envelope<AccountItemDto>>(apiUrl('/api/v1/account/update-account'), form)
  return response.data.data
}

export async function deleteAccount(id: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/account/delete-account'), { id })
}

export async function orderAccountList(changes: AccountPositionChange[]): Promise<AccountDto[]> {
  const response = await api.post<Envelope<{ items: AccountDto[] }>>(apiUrl('/api/v1/account/order-account-list'), { changes })
  return response.data.data.items
}

export async function getFolderList(): Promise<FolderDto[]> {
  const response = await api.get<Envelope<{ items: FolderDto[] }>>(apiUrl('/api/v1/account/get-folder-list'))
  return response.data.data.items
}

export async function createFolder(name: string): Promise<FolderDto> {
  const response = await api.post<Envelope<{ item: FolderDto }>>(apiUrl('/api/v1/account/create-folder'), { name })
  return response.data.data.item
}

export async function updateFolder(id: Id, name: string): Promise<FolderDto> {
  const response = await api.post<Envelope<{ item: FolderDto }>>(apiUrl('/api/v1/account/update-folder'), { id, name })
  return response.data.data.item
}

export async function replaceFolder(id: Id, replaceId: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/account/replace-folder'), { id, replaceId })
}

export async function hideFolder(id: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/account/hide-folder'), { id })
}

export async function showFolder(id: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/account/show-folder'), { id })
}

export async function orderFolderList(changes: { id: Id; position: number }[]): Promise<FolderDto[]> {
  const response = await api.post<Envelope<{ items: FolderDto[] }>>(apiUrl('/api/v1/account/order-folder-list'), { changes })
  return response.data.data.items
}

export async function grantAccess(form: { accountId: Id; userId: Id; role: AccountRole }): Promise<void> {
  await api.post(apiUrl('/api/v1/account/grant-access'), form)
}

export async function acceptAccess(form: { accountId: Id; folderId?: Id }): Promise<void> {
  await api.post(apiUrl('/api/v1/account/accept-access'), { accountId: form.accountId, folderId: form.folderId ?? '' })
}

export async function declineAccess(accountId: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/account/decline-access'), { accountId })
}

export async function revokeAccess(form: { accountId: Id; userId: Id }): Promise<void> {
  await api.post(apiUrl('/api/v1/account/revoke-access'), form)
}
