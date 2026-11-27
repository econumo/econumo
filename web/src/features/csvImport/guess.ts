import { detectDecimalSeparator } from './amounts'
import type { ColumnProfile, ColumnType } from './columns'
import { detectDateOrder, localeDateOrder } from './dates'
import { fileCurrencyId, withAccountCurrencies } from './fileCurrency'
import type { ParsedFile } from './parse'
import type { AccountChoice, Role, StepId, WizardAnswers } from './prepare'

// The headers an Econumo export writes: fixed English names in every UI language.
const EXPORT_COLUMNS = {
  account: 'account_name',
  date: 'date',
  amount: 'amount',
  category: 'category',
  description: 'description',
  payee: 'payee',
  tag: 'tag',
  labels: 'labels',
  currency: 'account_currency',
} as const
const EXPORT_HEADER = [...Object.values(EXPORT_COLUMNS), 'transaction_id']

type GuessedRole = Exclude<Role, 'account' | 'currency'>

// No 'account' synonyms: a bank's account column usually repeats one IBAN, and
// "each row says which account" is preselected only for an Econumo export.
export const SYNONYMS: Record<GuessedRole, string[]> = {
  date: [
    'date', 'transaction date', 'posting date', 'booking date', 'value date', 'posted', 'posted date',
    'datum', 'buchungstag', 'buchungsdatum', 'valuta', 'valutadatum', 'wertstellung',
    'fecha', 'fecha operación', 'fecha de operación', 'fecha valor',
    'date opération', "date d'opération", 'date de valeur', 'date valeur',
    'data', 'data operazione', 'data valuta', 'data contabile',
    'boekdatum', 'transactiedatum', 'rentedatum',
    'data operacji', 'data księgowania', 'data transakcji',
    'data movimento', 'data valor', 'data do movimento',
    'дата', 'дата операции', 'дата проводки', 'дата транзакции',
    'дата операції', 'дата проведення',
    '日期', '交易日期', '记账日期',
  ],
  amount: [
    'amount', 'value', 'sum', 'transaction amount',
    'betrag', 'umsatz', 'summe',
    'importe', 'monto', 'cantidad',
    'montant', 'somme',
    'importo', 'ammontare',
    'bedrag',
    'kwota', 'kwota operacji',
    'montante', 'valor', 'quantia',
    'сумма', 'сумма операции',
    'сума', 'сума операції',
    '金额', '交易金额',
  ],
  amountIn: [
    'credit', 'credits', 'money in', 'paid in', 'deposit', 'deposits', 'inflow', 'credit amount',
    'haben', 'eingang', 'gutschrift', 'einnahme', 'einnahmen',
    'abono', 'abonos', 'ingreso', 'ingresos', 'haber',
    'crédit', 'entrée', 'entrées', 'recette', 'recettes',
    'avere', 'entrate', 'accredito', 'accrediti',
    'bij', 'ontvangsten',
    'uznania', 'wpływy', 'przychód',
    'crédito', 'entrada', 'entradas',
    'приход', 'поступление', 'поступления', 'зачисление', 'кредит',
    'надходження', 'прихід', 'зарахування',
    '收入', '存入', '贷方',
  ],
  amountOut: [
    'debit', 'debits', 'money out', 'paid out', 'withdrawal', 'withdrawals', 'outflow', 'debit amount',
    'soll', 'ausgang', 'lastschrift', 'ausgabe', 'ausgaben',
    'cargo', 'cargos', 'gasto', 'gastos', 'debe',
    'débit', 'sortie', 'sorties', 'dépense', 'dépenses',
    'dare', 'uscite', 'addebito', 'addebiti',
    'af', 'debet', 'uitgaven',
    'obciążenia', 'wydatki', 'rozchód',
    'débito', 'saída', 'saídas',
    'расход', 'списание', 'списания', 'дебет',
    'витрата', 'витрати', 'списання', 'видаток',
    '支出', '取出', '借方',
  ],
  description: [
    'description', 'details', 'transaction details', 'memo', 'narrative', 'particulars', 'reference',
    'verwendungszweck', 'beschreibung', 'buchungstext', 'text',
    'descripción', 'concepto', 'detalle',
    'libellé', 'détails', 'libellé opération',
    'descrizione', 'causale', 'dettagli',
    'omschrijving', 'mededelingen', 'beschrijving',
    'opis', 'tytuł', 'tytuł operacji',
    'descrição', 'descritivo', 'detalhes',
    'описание', 'назначение платежа', 'детали', 'комментарий',
    'опис', 'призначення платежу', 'деталі', 'коментар',
    '描述', '摘要', '备注', '说明',
  ],
  payee: [
    'payee', 'merchant', 'counterparty', 'beneficiary', 'recipient',
    'empfänger', 'zahlungsempfänger', 'auftraggeber', 'begünstigter', 'beguenstigter/zahlungspflichtiger',
    'beneficiario', 'comercio', 'destinatario',
    'bénéficiaire', 'tiers', 'commerçant',
    'esercente', 'controparte',
    'begunstigde', 'tegenpartij', 'naam tegenpartij',
    'odbiorca', 'kontrahent', 'nadawca/odbiorca',
    'beneficiário', 'comerciante', 'destinatário',
    'получатель', 'контрагент', 'продавец',
    'отримувач', 'продавець',
    '收款人', '商户', '对方',
  ],
  category: [
    'category', 'categories', 'kategorie', 'categoría', 'catégorie', 'categoria', 'categorie',
    'kategoria', 'категория', 'категорія', '分类', '类别',
  ],
  tag: [
    'tag', 'budget tag', 'budget-tag', 'etiqueta', 'etiqueta de presupuesto', 'tag budgétaire', 'tag di budget',
    'budgettag', 'tag budżetowy', 'etiqueta de orçamento', 'тег', 'тег для бюджета', 'тег для бюджету', '预算标签',
  ],
  labels: [
    'labels', 'reporting tags', 'auswertungs-tags', 'etiquetas', 'etiquetas de informes', 'tags de rapport',
    'tag per i report', 'rapportagetags', 'tagi raportowe', 'etiquetas de relatório', 'теги', 'теги для отчётов',
    'теги для звітів', '统计标签',
  ],
}

const ROLE_TYPE: Record<GuessedRole, ColumnType> = {
  date: 'date',
  amount: 'number',
  amountIn: 'number',
  amountOut: 'number',
  description: 'text',
  payee: 'text',
  category: 'text',
  tag: 'text',
  labels: 'text',
}

// Money in/out before amount: "Сумма (приход)" contains "сумма" and must not
// become the single amount column in the substring pass.
const ROLE_ORDER: GuessedRole[] = ['date', 'amountIn', 'amountOut', 'amount', 'description', 'payee', 'category', 'tag', 'labels']

const HAN = /\p{Script=Han}/u

// Short Latin words ("af", "bij", "tag") are too common inside other headers to
// match as substrings; Chinese words are short by nature.
const substringOk = (synonym: string) => synonym.length >= 4 || HAN.test(synonym)

export function isEconumoExport(file: ParsedFile): boolean {
  return EXPORT_HEADER.every((column) => file.header.includes(column))
}

interface GuessContext {
  writableAccountIds: readonly string[]
  userCurrencyId: string
  locale: string
  defaultAccountName: string
  // resolves an Econumo export's account_currency codes; unknown codes get userCurrencyId
  currencies?: ReadonlyArray<{ id: string; code: string }>
}

export function columnValues(file: ParsedFile, column: number | null): string[] {
  if (column === null) return []
  return file.rows.map((row) => (row[column] ?? '').trim()).filter((v) => v !== '')
}

function exportAnswers(file: ParsedFile, ctx: GuessContext): WizardAnswers {
  const at = (name: string) => file.header.indexOf(name)
  const account = withAccountCurrencies(
    file,
    { kind: 'column', column: at(EXPORT_COLUMNS.account), currencyColumn: at(EXPORT_COLUMNS.currency), currencyId: ctx.userCurrencyId, overrides: {}, currencies: {} },
    ctx.currencies ?? [],
  )
  return {
    account,
    date: { column: at(EXPORT_COLUMNS.date), order: 'ymd' },
    amount: { mode: 'single', column: at(EXPORT_COLUMNS.amount), inColumn: null, outColumn: null, flip: false, sep: '.' },
    description: at(EXPORT_COLUMNS.description),
    extras: {
      payee: at(EXPORT_COLUMNS.payee),
      category: at(EXPORT_COLUMNS.category),
      tag: at(EXPORT_COLUMNS.tag),
      labels: at(EXPORT_COLUMNS.labels),
      labelsSeparator: ';',
    },
  }
}

function guessRoles(profiles: ColumnProfile[]): Partial<Record<GuessedRole, number>> {
  const found: Partial<Record<GuessedRole, number>> = {}
  const claimed = new Set<number>()
  const names = profiles.map((p) => p.name.trim().toLowerCase())

  // Synonyms are in order of preference: "Description" beats "Reference".
  const pass = (matches: (name: string, synonym: string) => boolean) => {
    for (const role of ROLE_ORDER) {
      if (found[role] !== undefined) continue
      for (const synonym of SYNONYMS[role]) {
        const index = profiles.findIndex(
          (p, i) => !claimed.has(i) && p.type === ROLE_TYPE[role] && matches(names[i], synonym),
        )
        if (index !== -1) {
          found[role] = index
          claimed.add(index)
          break
        }
      }
    }
  }
  pass((name, synonym) => name === synonym)
  pass((name, synonym) => substringOk(synonym) && name.includes(synonym))
  return found
}

export function guessAnswers(file: ParsedFile, profiles: ColumnProfile[], ctx: GuessContext): WizardAnswers {
  if (isEconumoExport(file)) return exportAnswers(file, ctx)

  const roles = guessRoles(profiles)
  const dual = roles.amountIn !== undefined && roles.amountOut !== undefined
  const writable = ctx.writableAccountIds
  const account: AccountChoice | null =
    writable.length === 0
      ? { kind: 'new', name: ctx.defaultAccountName, currencyId: fileCurrencyId(file, ctx.currencies ?? []) ?? ctx.userCurrencyId }
      : writable.length === 1
        ? { kind: 'existing', accountId: writable[0] }
        : null

  const answers: WizardAnswers = {
    account,
    date: { column: roles.date ?? null, order: 'ymd' },
    amount: {
      mode: dual ? 'dual' : 'single',
      column: roles.amount ?? null,
      inColumn: roles.amountIn ?? null,
      outColumn: roles.amountOut ?? null,
      flip: false,
      sep: '.',
    },
    description: roles.description ?? null,
    extras: {
      payee: roles.payee ?? null,
      category: roles.category ?? null,
      tag: roles.tag ?? null,
      labels: roles.labels ?? null,
      labelsSeparator: ';',
    },
  }
  return redetectFormats(answers, file, ctx.locale, { date: true, amount: true })
}

export function redetectFormats(
  answers: WizardAnswers,
  file: ParsedFile,
  locale: string,
  changed: { date?: boolean; amount?: boolean },
): WizardAnswers {
  let next = answers
  if (changed.date) {
    const { column } = answers.date
    const order = column === null ? localeDateOrder(locale) : detectDateOrder(columnValues(file, column), locale).order
    next = { ...next, date: { ...next.date, order } }
  }
  if (changed.amount) {
    const { mode, column, inColumn, outColumn } = answers.amount
    const columns = mode === 'single' ? [column] : [inColumn, outColumn]
    const sep = detectDecimalSeparator(columns.flatMap((c) => columnValues(file, c))).sep
    next = { ...next, amount: { ...next.amount, sep } }
  }
  return next
}

interface Slot {
  step: StepId
  role: Role
  get: (a: WizardAnswers) => number | null
  set: (a: WizardAnswers, column: number | null) => void
}

const SLOTS: Slot[] = [
  { step: 'account', role: 'account', get: (a) => (a.account?.kind === 'column' ? a.account.column : null), set: () => {} },
  { step: 'account', role: 'currency', get: (a) => (a.account?.kind === 'column' ? a.account.currencyColumn : null), set: () => {} },
  { step: 'date', role: 'date', get: (a) => a.date.column, set: (a, c) => (a.date.column = c) },
  { step: 'amount', role: 'amount', get: (a) => (a.amount.mode === 'single' ? a.amount.column : null), set: (a, c) => (a.amount.column = c) },
  { step: 'amount', role: 'amountIn', get: (a) => (a.amount.mode === 'dual' ? a.amount.inColumn : null), set: (a, c) => (a.amount.inColumn = c) },
  { step: 'amount', role: 'amountOut', get: (a) => (a.amount.mode === 'dual' ? a.amount.outColumn : null), set: (a, c) => (a.amount.outColumn = c) },
  { step: 'description', role: 'description', get: (a) => a.description, set: (a, c) => (a.description = c) },
  { step: 'payee', role: 'payee', get: (a) => a.extras.payee, set: (a, c) => (a.extras.payee = c) },
  { step: 'category', role: 'category', get: (a) => a.extras.category, set: (a, c) => (a.extras.category = c) },
  { step: 'extras', role: 'tag', get: (a) => a.extras.tag, set: (a, c) => (a.extras.tag = c) },
  { step: 'extras', role: 'labels', get: (a) => a.extras.labels, set: (a, c) => (a.extras.labels = c) },
]

// The AI's 'account' and 'currency' roles are not applied: it cannot list the account names a
// per-row account needs currencies for.
export function applyAiGuess(
  answers: WizardAnswers,
  roles: Partial<Record<Role, string>>,
  file: ParsedFile,
  locale: string,
  touched: ReadonlySet<StepId>,
): WizardAnswers {
  const { header } = file
  const next: WizardAnswers = { ...answers, date: { ...answers.date }, amount: { ...answers.amount }, extras: { ...answers.extras } }
  const open = SLOTS.filter((slot) => !touched.has(slot.step) && slot.step !== 'account')
  const taken = new Set(
    SLOTS.filter((slot) => touched.has(slot.step) || slot.step === 'account')
      .map((slot) => slot.get(answers))
      .filter((c) => c !== null),
  )

  const picks = new Map<Role, number>()
  const pick = (role: Role) => {
    const name = roles[role]
    const index = name === undefined ? -1 : header.indexOf(name)
    if (index === -1 || taken.has(index)) return
    picks.set(role, index)
    taken.add(index)
  }
  for (const slot of open) {
    if (slot.role !== 'amountIn' && slot.role !== 'amountOut') pick(slot.role)
  }
  if (!touched.has('amount')) {
    pick('amountIn')
    pick('amountOut')
    if (picks.has('amountIn') && picks.has('amountOut')) {
      picks.delete('amount')
      next.amount.mode = 'dual'
    } else {
      picks.delete('amountIn')
      picks.delete('amountOut')
      if (picks.has('amount')) next.amount.mode = 'single'
    }
  }

  const assigned = new Set(picks.values())
  for (const slot of open) {
    const picked = picks.get(slot.role)
    if (picked !== undefined) slot.set(next, picked)
    else if (assigned.has(slot.get(next) ?? -1)) slot.set(next, null)
  }

  const a = answers.amount
  const b = next.amount
  return redetectFormats(next, file, locale, {
    date: next.date.column !== answers.date.column,
    amount: a.mode !== b.mode || a.column !== b.column || a.inColumn !== b.inColumn || a.outColumn !== b.outColumn,
  })
}
