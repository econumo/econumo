import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import i18n from '@/app/i18n'
import { locale } from '@/lib/config'
import { removeToken, setToken } from '@/lib/storage'
import { server } from '@/test/msw'
import { LanguageBadge } from './LanguageBadge'

beforeEach(() => {
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

afterEach(() => {
  removeToken()
})

it('switches language via the dialog, persists it, and updates <html lang>', async () => {
  const user = userEvent.setup()
  render(<LanguageBadge />)
  await user.click(screen.getByRole('button', { name: /language/i }))
  await user.click(await screen.findByRole('button', { name: 'Русский' }))
  await waitFor(() => expect(i18n.language).toBe('ru'))
  expect(locale()).toBe('ru')
  expect(document.documentElement.lang).toBe('ru')
  await user.click(screen.getByRole('button', { name: /язык|language/i }))
  await user.click(await screen.findByRole('button', { name: 'English' }))
  await waitFor(() => expect(i18n.language).toBe('en'))
})

it('shows the current locale short code on the badge', async () => {
  render(<LanguageBadge />)
  expect(screen.getByRole('button', { name: /language/i })).toHaveTextContent('EN')
})

it('persists the choice to the API when authenticated', async () => {
  setToken('eco_ses_test')
  let body: unknown
  server.use(
    http.post('*/api/v1/user/update-language', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: null })
    }),
  )
  const user = userEvent.setup()
  render(<LanguageBadge />)
  await user.click(screen.getByRole('button', { name: /language/i }))
  await user.click(await screen.findByRole('button', { name: 'Русский' }))
  await waitFor(() => expect(i18n.language).toBe('ru'))
  await waitFor(() => expect(body).toEqual({ language: 'ru' }))
  await user.click(screen.getByRole('button', { name: /язык|language/i }))
  await user.click(await screen.findByRole('button', { name: 'English' }))
  await waitFor(() => expect(i18n.language).toBe('en'))
})

it('does not call the API when logged out', async () => {
  let called = false
  server.use(
    http.post('*/api/v1/user/update-language', () => {
      called = true
      return HttpResponse.json({ success: true, message: '', data: null })
    }),
  )
  const user = userEvent.setup()
  render(<LanguageBadge />)
  await user.click(screen.getByRole('button', { name: /language/i }))
  await user.click(await screen.findByRole('button', { name: 'Русский' }))
  await waitFor(() => expect(i18n.language).toBe('ru'))
  expect(called).toBe(false)
  await user.click(screen.getByRole('button', { name: /язык|language/i }))
  await user.click(await screen.findByRole('button', { name: 'English' }))
  await waitFor(() => expect(i18n.language).toBe('en'))
})
