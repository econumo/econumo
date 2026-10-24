import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { useUiStore } from '@/app/uiStore'
import { Toaster } from '@/components/ui/sonner'
import { METRICS, trackEvent } from '@/lib/metrics'
import { FeedbackDialog } from './FeedbackDialog'

vi.mock('@/lib/metrics', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/metrics')>()),
  trackEvent: vi.fn(),
}))

const FORM_URL = 'https://t.econumo.com/ingest/forms/feedback'

function renderDialog() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <FeedbackDialog />
      <Toaster />
    </QueryClientProvider>,
  )
}

function captureSubmissions(status = 200) {
  const bodies: Array<{ key: string; id: string; fields: Record<string, string>; attributes: Record<string, string> }> = []
  const urls: string[] = []
  server.use(
    http.post(FORM_URL, async ({ request }) => {
      urls.push(request.url)
      bodies.push(JSON.parse(await request.text()))
      return status === 200 ? HttpResponse.json({ id: 'x' }) : new HttpResponse(null, { status })
    }),
  )
  return { bodies, urls }
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = { VERSION: 'v1.7.0' }
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  server.use(...coreHandlers())
  useUiStore.setState({ feedbackOpen: true })
  vi.mocked(trackEvent).mockClear()
})

it('sends the type, message, email, version and hosting to the feedback form', async () => {
  const { bodies, urls } = captureSubmissions()
  renderDialog()
  await userEvent.click(screen.getByRole('radio', { name: 'Bug report' }))
  await userEvent.type(screen.getByLabelText('Message'), '  The plan grid is blank  ')
  await userEvent.click(await screen.findByRole('button', { name: 'Use my email' }))
  expect(screen.getByLabelText('Email (optional)')).toHaveValue('ada@example.test')
  await userEvent.click(screen.getByRole('button', { name: 'Send' }))

  await waitFor(() => expect(bodies).toHaveLength(1))
  expect(urls[0]).toContain('?key=ak_')
  expect(bodies[0].fields).toEqual({
    type: 'bug_report',
    message: 'The plan grid is blank',
    email: 'ada@example.test',
    app_version: 'v1.7.0',
    hosting: 'self-hosted',
  })
  // the synthetic host only: a self-hosted deployment's real hostname never rides along
  expect(bodies[0].attributes).toEqual({ $host: 'selfhosted_unknown' })
  expect(bodies[0].fields).not.toHaveProperty('$user_id')
  expect(await screen.findByText('Thanks for your feedback!')).toBeInTheDocument()
  expect(useUiStore.getState().feedbackOpen).toBe(false)
  expect(trackEvent).toHaveBeenCalledWith(METRICS.FEEDBACK_SEND, { type: 'bug_report', with_email: true })
})

it('starts as plain feedback with no email filled in', async () => {
  const { bodies } = captureSubmissions()
  renderDialog()
  expect(screen.getByRole('radio', { name: 'Feedback' })).toHaveAttribute('aria-checked', 'true')
  expect(screen.getByLabelText('Email (optional)')).toHaveValue('')
  await userEvent.type(screen.getByLabelText('Message'), 'Love it')
  await userEvent.click(screen.getByRole('button', { name: 'Send' }))
  await waitFor(() => expect(bodies).toHaveLength(1))
  expect(bodies[0].fields).toMatchObject({ type: 'feedback', email: '' })
})

it('refuses an empty message without sending', async () => {
  const { bodies } = captureSubmissions()
  renderDialog()
  await userEvent.type(screen.getByLabelText('Message'), '   ')
  await userEvent.click(screen.getByRole('button', { name: 'Send' }))
  expect(await screen.findByText('Write a message first.')).toBeInTheDocument()
  expect(bodies).toHaveLength(0)
  expect(useUiStore.getState().feedbackOpen).toBe(true)
})

it('keeps the message when the collector refuses it', async () => {
  captureSubmissions(400)
  renderDialog()
  await userEvent.type(screen.getByLabelText('Message'), 'Keep me')
  await userEvent.click(screen.getByRole('button', { name: 'Send' }))
  expect(await screen.findByText("Couldn't send your feedback. Please try again.")).toBeInTheDocument()
  expect(screen.getByLabelText('Message')).toHaveValue('Keep me')
  expect(useUiStore.getState().feedbackOpen).toBe(true)
  expect(trackEvent).not.toHaveBeenCalledWith(METRICS.FEEDBACK_SEND, expect.anything())
})
