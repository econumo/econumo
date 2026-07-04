import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { AccountDto } from '@/api/dto/account'
import type { TransactionDto, TransactionType } from '@/api/dto/transaction'
import type { Id } from '@/api/types'

export interface OpenTransactionParams {
  transaction?: TransactionDto
  type?: TransactionType
  accountId?: Id
}

export interface OpenAccountParams {
  account?: AccountDto
  folderId?: Id | null
}

interface UiState {
  transactionModal: OpenTransactionParams | null
  openTransactionModal: (params: OpenTransactionParams) => void
  closeTransactionModal: () => void
  accountModal: OpenAccountParams | null
  openAccountModal: (params: OpenAccountParams) => void
  closeAccountModal: () => void
  switchAccountPrompt: Id | null
  setSwitchAccountPrompt: (id: Id | null) => void
}

export const useUiStore = create<UiState>()((set) => ({
  transactionModal: null,
  openTransactionModal: (params) => set({ transactionModal: params }),
  closeTransactionModal: () => set({ transactionModal: null }),
  accountModal: null,
  openAccountModal: (params) => set({ accountModal: params }),
  closeAccountModal: () => set({ accountModal: null }),
  switchAccountPrompt: null,
  setSwitchAccountPrompt: (id) => set({ switchAccountPrompt: id }),
}))

interface SidebarState {
  folderOpen: Record<Id, boolean>
  toggleFolder: (id: Id) => void
}

// Persisted separately (replaces the Vue ACCOUNT_FOLDERS_OPENED localStorage map).
export const useSidebarStore = create<SidebarState>()(
  persist(
    (set) => ({
      folderOpen: {},
      toggleFolder: (id) =>
        set((state) => ({ folderOpen: { ...state.folderOpen, [id]: !(state.folderOpen[id] ?? true) } })),
    }),
    { name: 'sidebarFolders' },
  ),
)
