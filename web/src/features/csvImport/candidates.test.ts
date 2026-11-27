import { describe, expect, it } from 'vitest'
import { candidates } from './candidates'
import { profileColumns } from './columns'
import { guessAnswers } from './guess'
import { parseCsvText } from './parse'
import type { WizardAnswers } from './prepare'

const ctx = { writableAccountIds: ['acc-1', 'acc-2'], userCurrencyId: 'cur-usd', locale: 'en-GB', defaultAccountName: 'statement' }
const uuid = (i: number) => `01a1be03-f8df-7603-b2b1-2c3d4e5f6a${String(i).padStart(2, '0')}`
const day = (i: number) => `2026-11-${String(i + 1).padStart(2, '0')}`
const RENT = 'Monthly rent for the flat on Main Street and the garage'

function load(header: string, rows: string[]) {
  const file = parseCsvText(`${header}\n${rows.join('\n')}\n`)
  const profiles = profileColumns(file)
  return { file, profiles, answers: guessAnswers(file, profiles, ctx) }
}

const names = (profiles: { name: string }[], indices: number[]) => indices.map((i) => profiles[i].name)

describe('candidates', () => {
  it('an export offers only its account names as the account: not CAD codes, ids, free text or what other steps use', () => {
    const rows = Array.from({ length: 12 }, (_, i) =>
      [
        uuid(i),
        i % 3 ? 'WS Cash' : 'Visa',
        'CAD',
        ['Strata', 'Interest', 'Car'][i % 3],
        `Transfer of ${i + 1} CAD to Home Savings`,
        '',
        '',
        ['ICBC', 'Shell'][i % 2],
        `-${i + 1}.00`,
        `${day(i)} 09:00:00`,
      ].join(','),
    )
    const { profiles, answers } = load('transaction_id,account_name,account_currency,category,description,tag,labels,payee,amount,date', rows)
    expect(names(profiles, candidates('account', profiles, answers))).toEqual(['account_name'])
  })

  it('the account is short text that repeats, fewest names first', () => {
    const rows = Array.from({ length: 12 }, (_, i) =>
      [day(i), `-${i + 1}.00`, ['Cash', 'Visa', 'Savings'][i % 3], 'EUR', uuid(i), RENT, `Note ${i}`, i % 2 ? 'Card' : 'Bank'].join(','),
    )
    const { profiles, answers } = load('Date,Amount,Wallet,Cur,Ref,Rent,Note,Kind', rows)
    expect(names(profiles, candidates('account', profiles, answers))).toEqual(['Kind', 'Wallet'])
  })

  it('a column another step reads is not an account, the description or a detail', () => {
    const rows = Array.from({ length: 12 }, (_, i) => [day(i), `-${i + 1}.00`, i % 2 ? 'Cash' : 'Visa', 'Coffee'].join(','))
    const { profiles, answers } = load('Date,Amount,Wallet,Memo', rows)
    const taken: WizardAnswers = { ...answers, description: 2 }
    expect(names(profiles, candidates('account', profiles, taken))).toEqual(['Memo'])
    const perRow: WizardAnswers = { ...answers, description: null, account: { kind: 'column', column: 2, currencyColumn: null, currencyId: '', overrides: {}, currencies: {} } }
    expect(names(profiles, candidates('description', profiles, perRow))).toEqual(['Memo'])
    expect(names(profiles, candidates('payee', profiles, perRow))).toEqual(['Memo'])
  })

  it('the currency is a part of the file holding codes; once it reads one, no other step is offered it', () => {
    const rows = Array.from({ length: 12 }, (_, i) => [day(i), `-${i + 1}.00`, i % 2 ? 'Cash' : 'Visa', i % 2 ? 'EUR' : 'usd', ['Food', 'Fun'][i % 2]].join(','))
    const { profiles, answers } = load('Date,Amount,Wallet,Cur,Kind', rows)
    expect(names(profiles, candidates('currency', profiles, answers))).toEqual(['Cur'])
    const perRow: WizardAnswers = { ...answers, account: { kind: 'column', column: 2, currencyColumn: 3, currencyId: '', overrides: {}, currencies: {} } }
    for (const role of ['category', 'payee', 'description', 'tag', 'labels'] as const) {
      expect(names(profiles, candidates(role, profiles, perRow))).not.toContain('Cur')
    }
  })

  it('a small file repeats little, so a few different names still count as accounts', () => {
    const { profiles, answers } = load('Date,Amount,Wallet', ['2026-01-02,-5,Cash', '2026-01-03,7,Travel card', '2026-01-04,1,Home'])
    expect(names(profiles, candidates('account', profiles, answers))).toEqual(['Wallet'])
  })

  it('dates are only dates, amounts only money, the description the longest text first', () => {
    const rows = Array.from({ length: 4 }, (_, i) => [day(i), day(i + 1), `-${i + 1}.00`, `${i + 1}.00`, `123456789${i}`, ['Rewe', 'Aldi', 'Lidl', 'Edeka'][i], RENT].join(','))
    const { profiles, answers } = load('Booked,Valued,Out,In,Account no,Shop,Text', rows)
    expect(names(profiles, candidates('date', profiles, answers))).toEqual(['Booked', 'Valued'])
    expect(names(profiles, candidates('amount', profiles, answers))).toEqual(['Out', 'In'])
    expect(names(profiles, candidates('amountIn', profiles, answers))).toEqual(['Out', 'In'])
    expect(names(profiles, candidates('description', profiles, answers))).toEqual(['Text', 'Shop'])
  })
})
