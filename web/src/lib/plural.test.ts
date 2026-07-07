import { pluralPick } from './plural'

it('pluralPick picks the vue-i18n pipe variant and interpolates count', () => {
  const s = '{count} transaction(s) imported | {count} transactions imported'
  expect(pluralPick(s, 1)).toBe('1 transaction(s) imported')
  expect(pluralPick(s, 3)).toBe('3 transactions imported')
  expect(pluralPick('no pipes {count}', 5)).toBe('no pipes 5')
  expect(pluralPick(s, 0)).toBe('0 transactions imported')
})
