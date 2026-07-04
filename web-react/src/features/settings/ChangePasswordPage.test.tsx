import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { ChangePasswordPage } from './ChangePasswordPage'

function mockViewport() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter([{ path: '/settings/profile/change-password', element: <ChangePasswordPage /> }], {
    initialEntries: ['/settings/profile/change-password'],
  })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  mockViewport()
})

it('shows the exact validation messages', async () => {
  const user = userEvent.setup()
  renderPage()
  await user.click(screen.getByRole('button', { name: 'Change password' }))
  expect(await screen.findByText('Enter current password')).toBeInTheDocument()
  expect(screen.getByText('Password must be at least 4 characters')).toBeInTheDocument()
  expect(screen.getByText('Required field')).toBeInTheDocument()

  await user.type(screen.getByLabelText('Current password'), 'samepass')
  await user.type(screen.getByLabelText('New password'), 'samepass')
  await user.click(screen.getByRole('button', { name: 'Change password' }))
  expect(await screen.findByText('New password must differ from the old password')).toBeInTheDocument()

  await user.clear(screen.getByLabelText('New password'))
  await user.type(screen.getByLabelText('New password'), 'newpass1')
  await user.type(screen.getByLabelText('Confirm new password'), 'different')
  await user.click(screen.getByRole('button', { name: 'Change password' }))
  expect(await screen.findByText('Passwords do not match')).toBeInTheDocument()
})

it('success posts only old/new, clears the form and shows the success dialog', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/user/update-password', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await user.type(screen.getByLabelText('Current password'), 'oldpass1')
  await user.type(screen.getByLabelText('New password'), 'newpass1')
  await user.type(screen.getByLabelText('Confirm new password'), 'newpass1')
  await user.click(screen.getByRole('button', { name: 'Change password' }))
  await waitFor(() => expect(body).toEqual({ oldPassword: 'oldpass1', newPassword: 'newpass1' }))
  expect(await screen.findByText('You have successfully changed your password.')).toBeInTheDocument()
  await user.click(screen.getAllByRole('button', { name: 'Close' })[0])
  expect(screen.getByLabelText('Current password')).toHaveValue('')
})

it('a 400 (wrong old password) shows the error dialog', async () => {
  server.use(
    http.post('*/api/v1/user/update-password', () =>
      HttpResponse.json({ success: false, message: 'Form validation error', code: 400, errors: {} }, { status: 400 }),
    ),
  )
  const user = userEvent.setup()
  renderPage()
  await user.type(screen.getByLabelText('Current password'), 'wrongold')
  await user.type(screen.getByLabelText('New password'), 'newpass1')
  await user.type(screen.getByLabelText('Confirm new password'), 'newpass1')
  await user.click(screen.getByRole('button', { name: 'Change password' }))
  expect(await screen.findByText('Change password error')).toBeInTheDocument()
  expect(
    screen.getByText('An error occurred while changing the password; please check the information or try again later.'),
  ).toBeInTheDocument()
})
