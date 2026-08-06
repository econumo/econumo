import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { TagDialog } from './TagDialog'

function mockViewport() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function renderDialog(props: Partial<React.ComponentProps<typeof TagDialog>> = {}) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const onClose = vi.fn()
  render(
    <QueryClientProvider client={queryClient}>
      <TagDialog open onClose={onClose} {...props} />
    </QueryClientProvider>,
  )
  return { queryClient, onClose }
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  server.use(...coreHandlers())
  mockViewport()
})

it('previews the kind icon and swaps it live with the radio', async () => {
  const user = userEvent.setup()
  renderDialog()
  expect(screen.getByTestId('kind-icon')).toHaveTextContent('tag')
  await user.click(screen.getByRole('radio', { name: /Tag transactions for reporting/ }))
  expect(screen.getByTestId('kind-icon')).toHaveTextContent('label')
})

it('creates a label when the label kind is selected', async () => {
  let labelBody: Record<string, unknown> | undefined
  let tagCalled = false
  server.use(
    http.post('*/api/v1/label/create-label', async ({ request }) => {
      labelBody = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({
        success: true, message: '',
        data: { item: { id: 'label-new', ownerUserId: 'u1', name: 'Health', icon: 'label', position: 1, isArchived: 0, createdAt: '2026-01-01 00:00:00', updatedAt: '2026-01-01 00:00:00' } },
      })
    }),
    http.post('*/api/v1/tag/create-tag', () => {
      tagCalled = true
      return HttpResponse.json({ success: true, message: '', data: { item: {} } })
    }),
  )
  const user = userEvent.setup()
  const { onClose } = renderDialog()
  await user.click(screen.getByRole('radio', { name: /Tag transactions for reporting/ }))
  await user.type(screen.getByLabelText('Name'), 'Health')
  await user.click(screen.getByRole('button', { name: 'Create' }))
  await waitFor(() => expect(labelBody).toBeDefined())
  expect(labelBody!.name).toBe('Health')
  expect(tagCalled).toBe(false)
  await waitFor(() => expect(onClose).toHaveBeenCalled())
})

it('creates a tag by default when the kind is left untouched', async () => {
  let tagBody: Record<string, unknown> | undefined
  let labelCalled = false
  server.use(
    http.post('*/api/v1/tag/create-tag', async ({ request }) => {
      tagBody = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({
        success: true, message: '',
        data: { item: { id: 'tag-new', ownerUserId: 'u1', name: 'Vacation', icon: 'tag', position: 1, isArchived: 0, createdAt: '2026-01-01 00:00:00', updatedAt: '2026-01-01 00:00:00' } },
      })
    }),
    http.post('*/api/v1/label/create-label', () => {
      labelCalled = true
      return HttpResponse.json({ success: true, message: '', data: { item: {} } })
    }),
  )
  const user = userEvent.setup()
  renderDialog()
  await user.type(screen.getByLabelText('Name'), 'Vacation')
  await user.click(screen.getByRole('button', { name: 'Create' }))
  await waitFor(() => expect(tagBody).toBeDefined())
  expect(tagBody!.name).toBe('Vacation')
  expect(labelCalled).toBe(false)
})

it('hides the kind radio when editing an existing row', async () => {
  renderDialog({ item: { id: 'label1', name: 'health', kind: 'label', icon: 'sell' } })
  expect(await screen.findByLabelText('Name')).toHaveValue('health')
  expect(screen.queryByRole('radiogroup')).not.toBeInTheDocument()
  expect(screen.queryByRole('radio')).not.toBeInTheDocument()
})

it('edit previews the row\'s own stored icon, not the kind default', async () => {
  // 'sell' != DEFAULT_ICON.label ('label') — if the dialog recomputed the
  // default instead of reading the stored icon, this would show 'label'.
  renderDialog({ item: { id: 'label1', name: 'health', kind: 'label', icon: 'sell' } })
  expect(await screen.findByTestId('kind-icon')).toHaveTextContent('sell')
})

it('offers both purposes with their explanations when creating', () => {
  renderDialog({ item: null })
  expect(screen.getByRole('radio', { name: /Budget money for this tag/ })).toBeInTheDocument()
  expect(screen.getByText(/One per transaction/)).toBeInTheDocument()
  expect(screen.getByRole('radio', { name: /Tag transactions for reporting/ })).toBeInTheDocument()
  expect(screen.getByText(/Several per transaction/)).toBeInTheDocument()
})

it('locks the purpose when editing', () => {
  renderDialog({ item: { id: 'x', name: 'Trip', kind: 'tag', icon: 'tag' } })
  expect(screen.queryByRole('radiogroup')).not.toBeInTheDocument()
  expect(screen.getByTestId('kind-locked-note')).toBeInTheDocument()
})

it('editing a label posts to update-label, not update-tag', async () => {
  let labelBody: unknown
  let tagCalled = false
  server.use(
    http.post('*/api/v1/label/update-label', async ({ request }) => {
      labelBody = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { item: {} } })
    }),
    http.post('*/api/v1/tag/update-tag', () => {
      tagCalled = true
      return HttpResponse.json({ success: true, message: '', data: { item: {} } })
    }),
  )
  const user = userEvent.setup()
  const { onClose } = renderDialog({ item: { id: 'label1', name: 'health', kind: 'label', icon: 'sell' } })
  const input = await screen.findByLabelText('Name')
  await user.clear(input)
  await user.type(input, 'Wellness')
  await user.click(screen.getByRole('button', { name: 'Update' }))
  await waitFor(() => expect(labelBody).toEqual({ id: 'label1', name: 'Wellness' }))
  expect(tagCalled).toBe(false)
  await waitFor(() => expect(onClose).toHaveBeenCalled())
})

it('surfaces a server rejection instead of silently staying open', async () => {
  const user = userEvent.setup()
  server.use(
    http.post('*/api/v1/tag/create-tag', () =>
      HttpResponse.json({ success: false, message: 'Tag already exists.', code: 400, errors: {} }, { status: 400 }),
    ),
  )
  const { onClose } = renderDialog()
  await user.type(screen.getByLabelText(/name/i), 'Italy 2026')
  await user.click(screen.getByRole('button', { name: /create/i }))
  expect(await screen.findByText('Tag already exists.')).toBeInTheDocument()
  expect(onClose).not.toHaveBeenCalled()
})
