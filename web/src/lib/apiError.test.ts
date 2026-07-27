import { apiErrorMessage, apiFieldErrors } from './apiError'
import i18n from '@/app/i18n'

function axiosErr(data: unknown) {
  return { isAxiosError: true, response: { data } }
}

it('renders the server-translated message', () => {
  const msg = apiErrorMessage(
    axiosErr({ success: false, message: 'Неверные учётные данные.', code: 0, errors: {} }),
  )
  expect(msg).toBe('Неверные учётные данные.')
})

it('prefers the first field error over the generic form label', () => {
  const msg = apiErrorMessage(
    axiosErr({
      success: false,
      message: 'Form validation error',
      code: 400,
      errors: { name: ['Название категории должно содержать от 3 до 64 символов.'] },
    }),
  )
  expect(msg).toBe('Название категории должно содержать от 3 до 64 символов.')
})

it('falls back to a generic message when there is no envelope', () => {
  expect(apiErrorMessage(new Error('boom'))).toBe(i18n.t('common.app.error'))
})

it('apiFieldErrors returns the field messages', () => {
  const err = axiosErr({
    success: false,
    message: 'Form validation error',
    code: 400,
    errors: { name: ['Это значение слишком длинное.'] },
  })
  expect(apiFieldErrors(err, 'name')).toEqual(['Это значение слишком длинное.'])
  expect(apiFieldErrors(err, 'other')).toBeUndefined()
})
