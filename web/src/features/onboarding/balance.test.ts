import { decimalInput, parseBalance } from './balance'

describe('parseBalance', () => {
  it('reads en numbers with a comma group mark and a dot decimal', () => {
    expect(parseBalance('2,500', 'en')).toBe('2500')
    expect(parseBalance('2.50', 'en')).toBe('2.50')
    expect(parseBalance('1,234,567.89', 'en')).toBe('1234567.89')
  })

  it('reads de numbers with a dot group mark and a comma decimal', () => {
    expect(parseBalance('2.500', 'de')).toBe('2500')
    expect(parseBalance('2,50', 'de')).toBe('2.50')
    expect(parseBalance('1.234,50', 'de')).toBe('1234.50')
  })

  it('reads signs, blanks and spaces', () => {
    expect(parseBalance('-80', 'en')).toBe('-80')
    expect(parseBalance('-80', 'de')).toBe('-80')
    expect(parseBalance('', 'en')).toBe('0')
    expect(parseBalance('1 234,50', 'ru')).toBe('1234.50')
  })

  it('refuses text and malformed numbers', () => {
    expect(parseBalance('abc', 'en')).toBeNull()
    expect(parseBalance('abc', 'de')).toBeNull()
    expect(parseBalance('-', 'en')).toBeNull()
    expect(parseBalance('1,23', 'en')).toBeNull()
    expect(parseBalance('1.2.3', 'en')).toBeNull()
    expect(parseBalance('1,234.5.6', 'en')).toBeNull()
  })

  it('takes a foreign mark as the decimal unless three digits follow it', () => {
    // ru uses a comma decimal and a space group mark, so a dot is foreign
    expect(parseBalance('1234.50', 'ru')).toBe('1234.50')
    expect(parseBalance('2.500', 'ru')).toBeNull()
  })
})

it('decimalInput keeps every digit and writes the locale decimal mark', () => {
  expect(decimalInput('1234.123456789', 'en')).toBe('1234.123456789')
  expect(decimalInput('250.5', 'de')).toBe('250,5')
  expect(decimalInput('3000', 'de')).toBe('3000')
  expect(parseBalance(decimalInput('0.123456789012', 'ru'), 'ru')).toBe('0.123456789012')
})
