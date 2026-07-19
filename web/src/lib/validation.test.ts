import { isValidHttpUrl, isValidEmail, isValidName, isValidPassword, isNotEmpty, isValidRecoveryCode, isValidDecimalNumber } from './validation'

it('validates http(s) urls only', () => {
  expect(isValidHttpUrl('https://a.test')).toBe(true)
  expect(isValidHttpUrl('http://a.test')).toBe(true)
  expect(isValidHttpUrl('ftp://a.test')).toBe(false)
  expect(isValidHttpUrl('not a url')).toBe(false)
})

it('validates emails loosely (anything@anything)', () => {
  expect(isValidEmail('a@b')).toBe(true)
  expect(isValidEmail('nope')).toBe(false)
})

it('validates name 2-64, password 8-128, recovery code length 12', () => {
  expect(isValidName('ab')).toBe(true)
  expect(isValidName('a')).toBe(false)
  expect(isValidPassword('12345678')).toBe(true)
  expect(isValidPassword('1234567')).toBe(false)
  expect(isValidPassword('a'.repeat(129))).toBe(false)
  expect(isValidRecoveryCode('123456789012')).toBe(true)
  expect(isValidRecoveryCode('123')).toBe(false)
})

it('treats empty as valid decimal and enforces up to 8 fraction digits', () => {
  expect(isValidDecimalNumber('')).toBe(true)
  expect(isValidDecimalNumber('-12.12345678')).toBe(true)
  expect(isValidDecimalNumber('1.123456789')).toBe(false)
})

it('isNotEmpty rejects empty string and null', () => {
  expect(isNotEmpty('x')).toBe(true)
  expect(isNotEmpty('')).toBe(false)
})
