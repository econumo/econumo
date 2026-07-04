import { create } from 'zustand'
import { METRICS, trackEvent } from '@/lib/metrics'
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
  openTransactionModal: (params) => {
    trackEvent(METRICS.UI_MODAL_TRANSACTION_OPEN)
    set({ transactionModal: params })
  },
  closeTransactionModal: () => {
    trackEvent(METRICS.UI_MODAL_TRANSACTION_CLOSE)
    set({ transactionModal: null })
  },
  accountModal: null,
  openAccountModal: (params) => {
    trackEvent(METRICS.UI_MODAL_ACCOUNT_OPEN)
    set({ accountModal: params })
  },
  closeAccountModal: () => {
    trackEvent(METRICS.UI_MODAL_ACCOUNT_CLOSE)
    set({ accountModal: null })
  },
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
        set((state) => {
          const next = !(state.folderOpen[id] ?? true)
          trackEvent(next ? METRICS.ACCOUNT_FOLDER_EXPAND : METRICS.ACCOUNT_FOLDER_COLLAPSE)
          return { folderOpen: { ...state.folderOpen, [id]: next } }
        }),
    }),
    { name: 'sidebarFolders' },
  ),
)
