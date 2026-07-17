import { create } from 'zustand'
import { METRICS, trackEvent } from '@/lib/metrics'
import { persist } from 'zustand/middleware'
import type { AccountDto } from '@/api/dto/account'
import type { RecurringDto } from '@/api/dto/recurring'
import type { TransactionPrefill, TransactionType } from '@/api/dto/transaction'
import type { Id } from '@/api/types'

export interface OpenTransactionParams {
  transaction?: TransactionPrefill
  type?: TransactionType
  accountId?: Id
  // present when the caller is posting a due recurring template — switches
  // TransactionDialog into posting mode (prefilled from the template, with a
  // recurringId sent alongside the created transaction)
  postRecurring?: RecurringDto
}

export interface OpenAccountParams {
  account?: AccountDto
  folderId?: Id | null
}

export interface OpenRecurringParams {
  recurring?: RecurringDto
  fromTransaction?: TransactionPrefill
  accountId?: Id
}

interface UiState {
  transactionModal: OpenTransactionParams | null
  openTransactionModal: (params: OpenTransactionParams) => void
  closeTransactionModal: () => void
  accountModal: OpenAccountParams | null
  openAccountModal: (params: OpenAccountParams) => void
  closeAccountModal: () => void
  recurringModal: OpenRecurringParams | null
  openRecurringModal: (params: OpenRecurringParams) => void
  closeRecurringModal: () => void
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
  recurringModal: null,
  openRecurringModal: (params) => {
    trackEvent(METRICS.UI_MODAL_RECURRING_OPEN)
    set({ recurringModal: params })
  },
  closeRecurringModal: () => {
    trackEvent(METRICS.UI_MODAL_RECURRING_CLOSE)
    set({ recurringModal: null })
  },
  switchAccountPrompt: null,
  setSwitchAccountPrompt: (id) => set({ switchAccountPrompt: id }),
}))

interface SidebarState {
  folderOpen: Record<Id, boolean>
  toggleFolder: (id: Id) => void
  collapsed: boolean
  toggleCollapsed: () => void
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
      // Desktop-only icon-rail mode, toggled by clicking the sidebar divider.
      collapsed: false,
      toggleCollapsed: () => set((state) => ({ collapsed: !state.collapsed })),
    }),
    { name: 'sidebarFolders' },
  ),
)
