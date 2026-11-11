import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { MockInstance } from 'vitest'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import * as metrics from '@/lib/metrics'
import { CategoryDialog } from './CategoryDialog'

let asked: URLSearchParams[]

function renderDialog(props: Partial<React.ComponentProps<typeof CategoryDialog>> = {}) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const onSubmit = vi.fn()
  render(
    <QueryClientProvider client={queryClient}>
      <CategoryDialog open onClose={vi.fn()} onSubmit={onSubmit} {...props} />
    </QueryClientProvider>,
  )
  return onSubmit
}

const applied = (track: MockInstance<typeof metrics.trackEvent>) =>
  track.mock.calls.filter(([name]) => name === metrics.METRICS.ICON_SUGGESTION_APPLIED)

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = { AI_ENABLED: true }
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  asked = []
  server.use(...coreHandlers())
  server.use(
    http.get('*/api/v1/onboarding/suggest-icon', ({ request }) => {
      const params = new URL(request.url).searchParams
      asked.push(params)
      const icons: Record<string, string> = { Mortgage: 'house', Salary: '' }
      return HttpResponse.json({ success: true, message: '', data: { icon: icons[params.get('name') ?? ''] ?? 'pets' } })
    }),
  )
})

afterEach(() => {
  window.econumoConfig = {}
  vi.restoreAllMocks()
})

it('a new category takes the icon suggested for its name, once reported', async () => {
  const track = vi.spyOn(metrics, 'trackEvent').mockImplementation(() => undefined)
  const user = userEvent.setup()
  const onSubmit = renderDialog()
  await user.type(screen.getByLabelText('Name'), 'Mortgage')
  await waitFor(() => expect(applied(track)).toHaveLength(1), { timeout: 3000 })
  expect(applied(track)[0][1]).toEqual({ kind: 'category', source: 'dialog' })
  expect(Object.fromEntries(asked[0])).toEqual({ kind: 'category', name: 'Mortgage', type: 'expense' })
  // another name: the icon follows, the metric does not fire again
  await user.clear(screen.getByLabelText('Name'))
  await user.type(screen.getByLabelText('Name'), 'Vet bills')
  await waitFor(() => expect(asked).toHaveLength(2), { timeout: 3000 })
  await user.click(screen.getByRole('button', { name: 'Create' }))
  await waitFor(() => expect(onSubmit).toHaveBeenCalledWith({ name: 'Vet bills', type: 'expense', icon: 'pets' }))
  expect(applied(track)).toHaveLength(1)
})

it('renamed to a name with no suggestion, the icon goes back to the default', async () => {
  const user = userEvent.setup()
  const onSubmit = renderDialog()
  await user.type(screen.getByLabelText('Name'), 'Mortgage')
  await waitFor(() => expect(screen.getByRole('option', { name: 'home' })).toHaveAttribute('aria-selected', 'false'), { timeout: 3000 })
  await user.clear(screen.getByLabelText('Name'))
  await user.type(screen.getByLabelText('Name'), 'Salary')
  await waitFor(() => expect(asked).toHaveLength(2), { timeout: 3000 })
  await new Promise((resolve) => setTimeout(resolve, 100))
  await user.click(screen.getByRole('button', { name: 'Create' }))
  expect(onSubmit).toHaveBeenCalledWith({ name: 'Salary', type: 'expense', icon: 'home' })
})

it('an icon the user picked is never replaced', async () => {
  const user = userEvent.setup()
  const onSubmit = renderDialog()
  await user.click(screen.getByRole('option', { name: 'wallet' }))
  await user.type(screen.getByLabelText('Name'), 'Mortgage')
  await new Promise((resolve) => setTimeout(resolve, 900))
  await user.click(screen.getByRole('button', { name: 'Create' }))
  expect(onSubmit).toHaveBeenCalledWith({ name: 'Mortgage', type: 'expense', icon: 'wallet' })
  expect(asked).toHaveLength(0)
})

it('editing a category never changes its icon', async () => {
  const user = userEvent.setup()
  const onSubmit = renderDialog({ category: { name: 'Rent', type: 'expense', icon: 'home' } })
  await user.clear(screen.getByLabelText('Name'))
  await user.type(screen.getByLabelText('Name'), 'Mortgage')
  await new Promise((resolve) => setTimeout(resolve, 900))
  await user.click(screen.getByRole('button', { name: 'Update' }))
  expect(onSubmit).toHaveBeenCalledWith({ name: 'Mortgage', type: 'expense', icon: 'home' })
  expect(asked).toHaveLength(0)
})

it('without AI nothing is asked and the default icon stays', async () => {
  window.econumoConfig = {}
  const user = userEvent.setup()
  const onSubmit = renderDialog()
  await user.type(screen.getByLabelText('Name'), 'Mortgage')
  await new Promise((resolve) => setTimeout(resolve, 900))
  await user.click(screen.getByRole('button', { name: 'Create' }))
  expect(onSubmit).toHaveBeenCalledWith({ name: 'Mortgage', type: 'expense', icon: 'home' })
  expect(asked).toHaveLength(0)
})
