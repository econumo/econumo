import { apiErrorMessage } from './apiError'
import i18n from '@/app/i18n'

function axiosErr(data: unknown) {
  return { isAxiosError: true, response: { data } }
}

beforeAll(async () => {
  await i18n.changeLanguage('ru')
})

afterAll(async () => {
  await i18n.changeLanguage('en')
})

it('renders messageCode via the catalogue', () => {
  const msg = apiErrorMessage(
    axiosErr({ success: false, message: 'Invalid credentials.', code: 0, errors: {}, messageCode: 'auth.invalid_credentials' }),
  )
  expect(msg).toBe(i18n.t('errors.auth.invalid_credentials'))
  expect(msg).not.toBe('Invalid credentials.')
})

it('renders the first field errorCode with params', () => {
  const msg = apiErrorMessage(
    axiosErr({
      success: false,
      message: 'Form validation error',
      code: 400,
      errors: { name: ['Category name must be 3-64 characters'] },
      errorCodes: { name: [{ code: 'category.name_length', params: { min: 3, max: 64 } }] },
    }),
  )
  expect(msg).toContain('3')
  expect(msg).toContain('64')
  expect(msg).not.toBe('Category name must be 3-64 characters')
})

it('falls back to the raw English message for unknown codes', () => {
  expect(
    apiErrorMessage(axiosErr({ success: false, message: 'Something odd', code: 400, errors: {}, messageCode: 'no.such.code' })),
  ).toBe('Something odd')
})

it('falls back to a generic message when there is no envelope', () => {
  expect(apiErrorMessage(new Error('boom'))).toBe(i18n.t('common.app.error'))
})
