import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import { AvatarPickerDialog } from './AvatarPickerDialog'

function mockMatchMedia() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function renderDialog(onClose = vi.fn()) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <AvatarPickerDialog open onClose={onClose} />
    </QueryClientProvider>,
  )
  return { onClose, queryClient }
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  mockMatchMedia()
  server.use(...coreHandlers({ user: { ...fixtureUser, avatar: 'face:fuchsia' } }))
})

it('shows the current avatar as the preview when opened', async () => {
  renderDialog()
  await waitFor(() => expect(screen.getByTestId('user-avatar')).toHaveAttribute('data-avatar', 'face:fuchsia'))
})

it('picking a color and an icon updates the preview', async () => {
  const user = userEvent.setup()
  renderDialog()
  await waitFor(() => expect(screen.getByTestId('user-avatar')).toHaveAttribute('data-avatar', 'face:fuchsia'))
  await user.click(screen.getByRole('radio', { name: 'teal' }))
  await user.click(screen.getByRole('option', { name: 'pets' }))
  expect(screen.getByTestId('user-avatar')).toHaveAttribute('data-avatar', 'pets:teal')
})

it('save posts the picked icon/color and closes the dialog', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/user/update-avatar', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { user: { ...fixtureUser, avatar: 'pets:teal' } } })
    }),
  )
  const user = userEvent.setup()
  const { onClose } = renderDialog()
  await waitFor(() => expect(screen.getByTestId('user-avatar')).toHaveAttribute('data-avatar', 'face:fuchsia'))
  await user.click(screen.getByRole('radio', { name: 'teal' }))
  await user.click(screen.getByRole('option', { name: 'pets' }))
  await user.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(body).toEqual({ icon: 'pets', color: 'teal' }))
  await waitFor(() => expect(onClose).toHaveBeenCalled())
})

it('a user-cache rewrite while open keeps the in-progress selection', async () => {
  const user = userEvent.setup()
  const { queryClient } = renderDialog()
  await waitFor(() => expect(screen.getByTestId('user-avatar')).toHaveAttribute('data-avatar', 'face:fuchsia'))
  await user.click(screen.getByRole('radio', { name: 'teal' }))
  await user.click(screen.getByRole('option', { name: 'pets' }))
  expect(screen.getByTestId('user-avatar')).toHaveAttribute('data-avatar', 'pets:teal')
  // simulate a mutation/refetch rewriting the user cache while the dialog is
  // open (e.g. useUpdateName's onSuccess) — the changed fields defeat structural
  // sharing, so useUserData hands out a new user reference
  act(() => {
    queryClient.setQueryData(queryKeys.user, { ...fixtureUser, avatar: 'wallet:red', name: 'Grace' })
  })
  // flush TanStack Query's batched observer notification (a macrotask) + effects
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0))
  })
  expect(queryClient.getQueryData<{ name: string }>(queryKeys.user)!.name).toBe('Grace')
  expect(screen.getByTestId('user-avatar')).toHaveAttribute('data-avatar', 'pets:teal')
})

it('cancel closes the dialog without saving', async () => {
  let posts = 0
  server.use(
    http.post('*/api/v1/user/update-avatar', () => {
      posts += 1
      return HttpResponse.json({ success: true, message: '', data: { user: fixtureUser } })
    }),
  )
  const user = userEvent.setup()
  const { onClose } = renderDialog()
  await waitFor(() => expect(screen.getByTestId('user-avatar')).toHaveAttribute('data-avatar', 'face:fuchsia'))
  await user.click(screen.getByRole('button', { name: 'Cancel' }))
  expect(onClose).toHaveBeenCalled()
  expect(posts).toBe(0)
})
