import type { Id } from '../types'
import type { UserDto } from './user'

export type BudgetRole = 'owner' | 'admin' | 'user' | 'guest'

export interface BudgetAccessDto {
  user: UserDto
  role: BudgetRole
  isAccepted: 0 | 1
}

export interface BudgetMetaDto {
  id: Id
  ownerUserId: Id
  name: string
  /** full datetime Y-m-d H:i:s */
  startedAt: string
  /** last covered month as a full datetime, '' when the budget is open-ended */
  endedAt: string
  currencyId: Id
  isArchived: 0 | 1
  access: BudgetAccessDto[]
}

export const BudgetElementType = { ENVELOPE: 0, CATEGORY: 1, TAG: 2, INCOME_CATEGORY: 3, INCOME_ENVELOPE: 4 } as const
export type BudgetElementType = (typeof BudgetElementType)[keyof typeof BudgetElementType]

export const isIncomeType = (t: BudgetElementType): boolean =>
  t === BudgetElementType.INCOME_CATEGORY || t === BudgetElementType.INCOME_ENVELOPE

/** the presentation-only element the backend emits for spending with no category */
export const UNCATEGORIZED_ID = 'uncategorized'

export interface BudgetChildElementDto {
  id: Id
  type: BudgetElementType
  name: string
  icon: string
  isArchived: 0 | 1
  /** decimal string (wire format, kept verbatim) */
  spent: string
  budgetSpent: string
  ownerUserId: Id
}

export interface BudgetElementDto extends Omit<BudgetChildElementDto, 'ownerUserId'> {
  /** null = inherit the budget base currency */
  currencyId: Id | null
  folderId: Id | null
  position: number
  budgeted: string
  available: string
  ownerUserId: Id | null
  children: BudgetChildElementDto[]
}

export interface BudgetFolderDto {
  id: Id
  name: string
  position: number
}

/** a reporting label's period spend. No budgeted/available: a label never
 *  becomes a budget element or carries a limit. Amounts across labels
 *  deliberately overlap (one transaction can carry several) and do not sum
 *  to total spend. */
export interface LabelSpendDto {
  id: Id
  name: string
  icon: string
  isArchived: 0 | 1
  /** decimal string (wire format, kept verbatim), already in the budget currency */
  spent: string
  ownerUserId: Id
  /** per-category breakdown of this label's spend; unlike the labels
   *  themselves these DO sum to the label's own total. Optional on the wire:
   *  servers older than this release omit it. */
  children?: BudgetChildElementDto[]
}

// nullable by period phase: future month = all null except holdings; current month = endBalance null
export interface BudgetBalanceDto {
  currencyId: Id
  startBalance: string | null
  endBalance: string | null
  income: string | null
  expenses: string | null
  exchanges: string | null
  holdings: string | null
}

export interface BudgetRateDto {
  currencyId: Id
  baseCurrencyId: Id
  rate: string
  /** date-only Y-m-d */
  periodStart: string
  periodEnd: string
}

// budget/get-transaction-list has its own wire shape: spentAt (not date),
// embedded category/payee/tag refs and a per-transaction currencyId
export interface BudgetTransactionDto {
  id: Id
  author: UserDto
  currencyId: Id
  /** decimal string (wire format, kept verbatim) */
  amount: string
  description: string
  category: { id: Id; name: string; icon: string } | null
  payee: { id: Id; name: string } | null
  tag: { id: Id; name: string } | null
  /** reporting labels attached to the row; [] when none */
  labelIds?: Id[]
  /** full datetime Y-m-d H:i:s */
  spentAt: string
  /** only on rows of the transfers selector: which side of the boundary the
   *  included account is on; amount/currencyId are that side's */
  direction?: 'in' | 'out'
}

export interface BudgetDto {
  meta: BudgetMetaDto
  filters: { periodStart: string; periodEnd: string; accounts: { id: Id; removable: boolean }[] }
  balances: BudgetBalanceDto[]
  currencyRates: BudgetRateDto[]
  /** labels is optional on the wire: servers older than the labels release —
   *  still accepted by the app's compat floor — omit it. */
  structure: { folders: BudgetFolderDto[]; elements: BudgetElementDto[]; labels?: LabelSpendDto[] }
}

/** per-month cell on a plan-view parent row. planned '' = no limit set that month. */
export interface PlanCellDto {
  actual: string
  planned: string
}

/** per-month cell on a plan-view child row: children never carry their own limit. */
export interface PlanChildDto {
  id: Id
  type: BudgetElementType
  name: string
  icon: string
  isArchived: 0 | 1
  ownerUserId: Id
  cells: { actual: string }[]
}

export interface PlanElementDto {
  id: Id
  type: BudgetElementType
  name: string
  icon: string
  currencyId: Id
  isArchived: 0 | 1
  folderId: Id | null
  position: number
  ownerUserId: Id | null
  cells: PlanCellDto[]
  children: PlanChildDto[]
}

export interface PlanMonthRatesDto {
  /** date-only Y-m-d, first of the month */
  period: string
  rates: BudgetRateDto[]
}

/** one currency's transfers across the budget boundary in one window month:
 *  in = moved into included accounts, out = moved out of them (each side in
 *  its own account's currency); never planned */
export interface PlanTransferDto {
  currencyId: Id
  in: string
  out: string
}

export interface PlanMonthTransfersDto {
  /** date-only Y-m-d, first of the month */
  period: string
  /** [] for a month nothing crossed; budget currency first, then by id */
  items: PlanTransferDto[]
}

export interface BudgetPlanDto {
  meta: BudgetMetaDto
  /** date-only Y-m-d, first of each month in the fetched window */
  months: string[]
  openingBalances: { currencyId: Id; amount: string }[]
  currencyRates: PlanMonthRatesDto[]
  /** one entry per months[i] */
  transfers: PlanMonthTransfersDto[]
  structure: { folders: BudgetFolderDto[]; elements: PlanElementDto[] }
}
