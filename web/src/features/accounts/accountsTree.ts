import type { AccountDto } from '@/api/dto/account'
import type { CurrencyDto } from '@/api/dto/currency'
import type { FolderDto } from '@/api/dto/folder'
import { add } from '@/lib/decimal'

export interface AccountsTreeItem {
  folder: FolderDto
  accounts: AccountDto[]
  total: string
  /** the folder's shared currency, or the user currency when mixed */
  currency: CurrencyDto | null
}

export const SYNTHETIC_FOLDER_ID = '0'

// Builds visible folders only in position order, computing per-folder totals
// in native currency when uniform or user-currency-converted when mixed,
// groups folderless accounts into a trailing synthetic folder, and drops
// empty folders. Accounts inside hidden folders disappear entirely.
export function buildAccountsTree(
  accounts: AccountDto[],
  folders: FolderDto[],
  userCurrency: CurrencyDto | null,
  exchangeFn: (fromCurrencyId: string, toCurrencyId: string, amount: string) => string,
  defaultFolderName: string,
): AccountsTreeItem[] {
  const orderedAccounts = [...accounts].sort((a, b) => a.position - b.position)
  const orderedFolders = [...folders].sort((a, b) => a.position - b.position)
  const items: AccountsTreeItem[] = []

  const buildItem = (folder: FolderDto, folderAccounts: AccountDto[]): AccountsTreeItem => {
    let sharedCurrency: CurrencyDto | null = null
    let mixed = false
    let nativeTotal = '0'
    let convertedTotal = '0'
    for (const account of folderAccounts) {
      if (sharedCurrency === null) {
        sharedCurrency = account.currency
      } else if (sharedCurrency.id !== account.currency.id) {
        mixed = true
      }
      nativeTotal = add(nativeTotal, account.balance)
      convertedTotal = add(convertedTotal, userCurrency ? exchangeFn(account.currency.id, userCurrency.id, account.balance) : account.balance)
    }
    if (sharedCurrency && !mixed) {
      return { folder, accounts: folderAccounts, total: nativeTotal, currency: sharedCurrency }
    }
    return { folder, accounts: folderAccounts, total: convertedTotal, currency: userCurrency }
  }

  for (const folder of orderedFolders) {
    if (folder.isVisible !== 1) {
      continue
    }
    const folderAccounts = orderedAccounts.filter((a) => a.folderId === folder.id)
    if (folderAccounts.length === 0) {
      continue
    }
    items.push(buildItem(folder, folderAccounts))
  }

  const folderless = orderedAccounts.filter((a) => !a.folderId)
  if (folderless.length > 0) {
    items.push(
      buildItem({ id: SYNTHETIC_FOLDER_ID, name: defaultFolderName, position: orderedFolders.length, isVisible: 1 }, folderless),
    )
  }

  return items
}
