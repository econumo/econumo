import { describe, expect, it } from 'vitest'
import type { CurrencyListItemDto } from '@/api/dto/currency'
import { selectableCurrencies } from './selectable'

const cur = (over: Partial<CurrencyListItemDto>): CurrencyListItemDto => ({
  id: 'x', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2,
  scope: 'global', isHidden: 0, isDeleted: 0, ...over,
})

describe('selectableCurrencies', () => {
  it('keeps visible globals, hides hidden globals, and keeps all own customs', () => {
    const items = [
      cur({ id: 'usd' }),
      cur({ id: 'eur', code: 'EUR', isHidden: 1 }),
      cur({ id: 'pts', code: 'PTS', scope: 'own' }),
      cur({ id: 'old', code: 'OLD', scope: 'own', isHidden: 1 }),
      cur({ id: 'gem', code: 'GEM', scope: 'shared' }),
    ]
    expect(selectableCurrencies(items).map((c) => c.id)).toEqual(['usd', 'pts', 'old'])
  })
  // Own customs have no hide affordance; a hidden row left by a previous UI
  // version or an API/MCP client must not become stranded out of every picker.
  it('keeps a hidden own custom selectable', () => {
    const items = [cur({ id: 'pts', code: 'PTS', scope: 'own', isHidden: 1 })]
    expect(selectableCurrencies(items).map((c) => c.id)).toEqual(['pts'])
  })
  it('keeps the current value even when filtered out', () => {
    const items = [cur({ id: 'usd' }), cur({ id: 'gem', code: 'GEM', scope: 'shared' })]
    expect(selectableCurrencies(items, 'gem').map((c) => c.id)).toEqual(['usd', 'gem'])
  })
  it('handles undefined', () => {
    expect(selectableCurrencies(undefined)).toEqual([])
  })
  it('excludes deleted currencies', () => {
    const items = [cur({ id: 'usd' }), cur({ id: 'pts', code: 'PTS', scope: 'own', isDeleted: 1 })]
    expect(selectableCurrencies(items).map((c) => c.id)).toEqual(['usd'])
  })
  // An account denominated in a deleted currency must stay editable, so the
  // current value survives the filter.
  it('keeps a deleted currency when it is the current value', () => {
    const items = [cur({ id: 'usd' }), cur({ id: 'pts', code: 'PTS', scope: 'own', isDeleted: 1 })]
    expect(selectableCurrencies(items, 'pts').map((c) => c.id)).toEqual(['usd', 'pts'])
  })
})
