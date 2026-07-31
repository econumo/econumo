import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { fixtureUser } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import { ChangeEmailPage } from './ChangeEmailPage'

function mockViewport() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  queryClient.setQueryData(queryKeys.user, fixtureUser)
  const router = createMemoryRouter(
    [
      { path: '/settings/profile/change-email', element: <ChangeEmailPage /> },
      { path: '/settings/profile', element: <div>Profile page</div> },
    ],
    { initialEntries: ['/settings/profile/change-email'] },
  )
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return { queryClient }
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  mockViewport()
})

it('request phase validates, then submits and switches to the code phase', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/user/request-email-change', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage()

  await user.click(screen.getByRole('button', { name: 'Send confirmation code' }))
  expect(await screen.findByText('Required field')).toBeInTheDocument()
  expect(screen.getByText('Enter your current password')).toBeInTheDocument()

  await user.type(screen.getByLabelText('New email'), 'newmail@example.test')
  await user.type(screen.getByLabelText('Current password'), 'secret123')
  await user.click(screen.getByRole('button', { name: 'Send confirmation code' }))

  await waitFor(() => expect(body).toEqual({ newEmail: 'newmail@example.test', password: 'secret123' }))
  expect(await screen.findByLabelText('Confirmation code')).toBeInTheDocument()
  expect(screen.getByText(/newmail@example\.test/)).toBeInTheDocument()
})

async function goToConfirmPhase(user: ReturnType<typeof userEvent.setup>) {
  server.use(
    http.post('*/api/v1/user/request-email-change', () => HttpResponse.json({ success: true, message: '', data: {} })),
  )
  await user.type(screen.getByLabelText('New email'), 'newmail@example.test')
  await user.type(screen.getByLabelText('Current password'), 'secret123')
  await user.click(screen.getByRole('button', { name: 'Send confirmation code' }))
  await screen.findByLabelText('Confirmation code')
}

it('confirm success updates the user cache and shows the success dialog, closing navigates to profile', async () => {
  const user = userEvent.setup()
  const { queryClient } = renderPage()
  await goToConfirmPhase(user)

  server.use(
    http.post('*/api/v1/user/confirm-email-change', async ({ request }) => {
      expect(await request.json()).toEqual({ code: '482913' })
      return HttpResponse.json({ success: true, message: '', data: { ...fixtureUser, email: 'newmail@example.test' } })
    }),
  )
  await user.type(screen.getByLabelText('Confirmation code'), '482913')
  await user.click(screen.getByRole('button', { name: 'Confirm new email' }))

  expect(await screen.findByText('Email changed')).toBeInTheDocument()
  await waitFor(() => expect(queryClient.getQueryData<{ email: string }>(queryKeys.user)!.email).toBe('newmail@example.test'))

  await user.click(screen.getAllByRole('button', { name: 'Close' })[0])
  expect(await screen.findByText('Profile page')).toBeInTheDocument()
})

it('a wrong code surfaces the server error message inline', async () => {
  const user = userEvent.setup()
  renderPage()
  await goToConfirmPhase(user)

  server.use(
    http.post('*/api/v1/user/confirm-email-change', () =>
      HttpResponse.json({ success: false, message: 'The confirmation code is not valid.', code: 400, errors: {} }, { status: 400 }),
    ),
  )
  await user.type(screen.getByLabelText('Confirmation code'), '999999')
  await user.click(screen.getByRole('button', { name: 'Confirm new email' }))
  expect(await screen.findByText('The confirmation code is not valid.')).toBeInTheDocument()
})

it('holds resend behind the 60s cooldown seeded on entering the confirm phase', async () => {
  const user = userEvent.setup()
  renderPage()
  await goToConfirmPhase(user)

  const resendButton = () => screen.getByRole('button', { name: /resend code/i })
  expect(resendButton()).toBeDisabled()
  expect(resendButton()).toHaveTextContent('60s')
})
