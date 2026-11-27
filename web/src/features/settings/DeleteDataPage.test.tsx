import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider, onlineManager } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { AxiosError } from 'axios'
import { createAppQueryClient } from '@/lib/appQueryClient'
import { server } from '@/test/msw'
import { coreHandlers, fixtureAccounts, fixtureUser } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import { setToken } from '@/lib/storage'
import { api, apiUrl } from '@/api/client'
import { addChange, settleRefused } from '@/features/pendingWrites/queue'
import { getOps, loadPendingWrites, mutate } from '@/features/pendingWrites/store'
import { LoginPage } from '@/features/auth/LoginPage'
import { backendHost, selfHosted } from '@/lib/config'
import { DeleteDataPage } from './DeleteDataPage'

const queueOverride = vi.hoisted(() => ({ settled: undefined as boolean | undefined }))
vi.mock('@/features/pendingWrites/hooks', async (importOriginal) => {
  const real = await importOriginal<typeof import('@/features/pendingWrites/hooks')>()
  return {
    ...real,
    useStoredQueueSettled: () => {
      const settled = real.useStoredQueueSettled()
      return queueOverride.settled ?? settled
    },
  }
})

// counts sign-outs and can hold one, for the page's own clean-up and the interceptor's alike
const signOutGate = vi.hoisted(() => ({ hold: null as Promise<void> | null, calls: 0 }))
vi.mock('@/features/auth/localSignOut', async (importOriginal) => {
  const real = await importOriginal<typeof import('@/features/auth/localSignOut')>()
  return {
    ...real,
    signOutLocally: async (...args: Parameters<typeof real.signOutLocally>) => {
      signOutGate.calls++
      if (signOutGate.hold) {
        await signOutGate.hold
      }
      return real.signOutLocally(...args)
    },
  }
})

const CLEANUP_KEY = 'econumo.deletionCleanup'
const partner = { id: 'u2', avatar: 'pets:sky', name: 'Partner' }
const assign = vi.fn()
const downloads: string[] = []

function renderPage(
  user: Record<string, unknown> = fixtureUser,
  queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } }),
) {
  queryClient.setQueryData(queryKeys.user, user)
  const router = createMemoryRouter(
    [
      { path: '/settings/profile/delete-data', element: <DeleteDataPage /> },
      { path: '/settings/profile', element: <div>Profile page</div> },
    ],
    { initialEntries: ['/settings/profile/delete-data'] },
  )
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return { queryClient }
}

function okEnvelope() {
  return HttpResponse.json({ success: true, message: '', data: {} })
}

interface ExportLog {
  calls: string[]
}

function exportHandler(log: ExportLog, status = 200) {
  return http.get('*/api/v1/transaction/export-transaction-list', ({ request }) => {
    log.calls.push(new URL(request.url).searchParams.get('accountId') ?? '')
    if (status !== 200) {
      return HttpResponse.json({ success: false, message: 'boom', code: 0, exceptionType: 'x' }, { status })
    }
    return new HttpResponse('transaction_id\n', { headers: { 'Content-Type': 'text/csv; charset=UTF-8' } })
  })
}

async function goToCode(user: ReturnType<typeof userEvent.setup>, withPassword = true) {
  if (withPassword) {
    await user.type(screen.getByLabelText('Password'), 'secret123')
  }
  await user.click(screen.getByRole('button', { name: 'Send confirmation code' }))
  await screen.findByLabelText('Confirmation code')
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  onlineManager.setOnline(true)
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  URL.createObjectURL = vi.fn(() => 'blob:mock')
  URL.revokeObjectURL = vi.fn()
  downloads.length = 0
  vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) {
    downloads.push(this.download)
  })
  assign.mockClear()
  queueOverride.settled = undefined
  signOutGate.hold = null
  signOutGate.calls = 0
  Object.defineProperty(window, 'location', { value: { ...window.location, assign }, writable: true })
})

afterEach(() => {
  onlineManager.setOnline(true)
  vi.restoreAllMocks()
})

describe('the request phase', () => {
  it('asks for the password, then POSTs it and shows the code phase', async () => {
    let body: unknown
    const log: ExportLog = { calls: [] }
    server.use(
      ...coreHandlers(),
      exportHandler(log),
      http.post('*/api/v1/user/request-user-deletion', async ({ request }) => {
        body = await request.json()
        return okEnvelope()
      }),
    )
    const user = userEvent.setup()
    renderPage()
    expect(screen.getByText(/This deletes your accounts with their transactions/)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Send confirmation code' }))
    expect(await screen.findByText('Required field')).toBeInTheDocument()
    expect(body).toBeUndefined()

    await goToCode(user)
    expect(body).toEqual({ password: 'secret123' })
    expect(screen.getByText('We sent a confirmation code to your email.')).toBeInTheDocument()
  })

  it('a wrong password shows the field error', async () => {
    server.use(
      ...coreHandlers(),
      http.post('*/api/v1/user/request-user-deletion', () =>
        HttpResponse.json({ success: false, message: 'Invalid', code: 400, errors: { password: ['Wrong password'] } }, { status: 400 }),
      ),
    )
    const user = userEvent.setup()
    renderPage()
    await user.type(screen.getByLabelText('Password'), 'nope')
    await user.click(screen.getByRole('button', { name: 'Send confirmation code' }))
    expect(await screen.findByText('Wrong password')).toBeInTheDocument()
    expect(screen.queryByLabelText('Confirmation code')).not.toBeInTheDocument()
  })

  it('a passwordless user is asked for no password and sends an empty one', async () => {
    let body: unknown
    server.use(
      ...coreHandlers(),
      exportHandler({ calls: [] }),
      http.post('*/api/v1/user/request-user-deletion', async ({ request }) => {
        body = await request.json()
        return okEnvelope()
      }),
    )
    const user = userEvent.setup()
    renderPage({ ...fixtureUser, hasPassword: false })
    expect(screen.queryByLabelText('Password')).not.toBeInTheDocument()
    await goToCode(user, false)
    expect(body).toEqual({ password: '' })
  })

  it('asks for no password where the signed-in backend has password sign-in off', async () => {
    window.econumoConfig = { PASSWORD_LOGIN: false }
    let body: unknown
    server.use(
      ...coreHandlers(),
      exportHandler({ calls: [] }),
      http.post('*/api/v1/user/request-user-deletion', async ({ request }) => {
        body = await request.json()
        return okEnvelope()
      }),
    )
    const user = userEvent.setup()
    renderPage({ ...fixtureUser, hasPassword: true })
    expect(screen.queryByLabelText('Password')).not.toBeInTheDocument()
    await goToCode(user, false)
    expect(body).toEqual({ password: '' })
  })

  it('says nothing about a subscription, even with billing configured', () => {
    window.econumoConfig = { BILLING_URL: 'https://billing.example.test' }
    server.use(...coreHandlers())
    renderPage()
    expect(screen.queryByText(/subscription/i)).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Open billing' })).not.toBeInTheDocument()
  })
})

describe('the download', () => {
  it('runs once on the code phase, for the accounts the user owns only', async () => {
    const log: ExportLog = { calls: [] }
    const withShared = [...fixtureAccounts, { ...fixtureAccounts[0], id: 'a-shared', name: 'Partner wallet', owner: partner }]
    server.use(
      ...coreHandlers({ accounts: withShared }),
      exportHandler(log),
      http.post('*/api/v1/user/request-user-deletion', () => okEnvelope()),
    )
    const user = userEvent.setup()
    renderPage()
    expect(log.calls).toEqual([])
    await goToCode(user)
    expect(await screen.findByText(/Your transactions were downloaded/)).toBeInTheDocument()
    expect(log.calls).toEqual(['a1,a2,a3,a-hidden'])
    expect(downloads).toEqual([expect.stringMatching(/^transactions-\d{4}-\d{2}-\d{2}\.csv$/)])
  })

  it('waits for the account list before downloading', async () => {
    const log: ExportLog = { calls: [] }
    let releaseAccounts: () => void = () => {}
    const accountsReady = new Promise<void>((resolve) => {
      releaseAccounts = resolve
    })
    server.use(
      http.get('*/api/v1/account/get-account-list', async () => {
        await accountsReady
        return HttpResponse.json({ success: true, message: '', data: { items: fixtureAccounts } })
      }),
      ...coreHandlers(),
      exportHandler(log),
      http.post('*/api/v1/user/request-user-deletion', () => okEnvelope()),
    )
    const user = userEvent.setup()
    renderPage()
    await goToCode(user)
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(log.calls).toEqual([])
    expect(screen.getByRole('button', { name: 'Delete my data' })).toBeDisabled()
    releaseAccounts()
    await waitFor(() => expect(log.calls).toEqual(['a1,a2,a3,a-hidden']))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Delete my data' })).toBeEnabled())
  })

  it('keeps Confirm disabled while the download runs', async () => {
    let releaseExport: () => void = () => {}
    const exportReady = new Promise<void>((resolve) => {
      releaseExport = resolve
    })
    server.use(
      ...coreHandlers(),
      http.get('*/api/v1/transaction/export-transaction-list', async () => {
        await exportReady
        return new HttpResponse('transaction_id\n', { headers: { 'Content-Type': 'text/csv; charset=UTF-8' } })
      }),
      http.post('*/api/v1/user/request-user-deletion', () => okEnvelope()),
    )
    const user = userEvent.setup()
    renderPage()
    await goToCode(user)
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(screen.getByRole('button', { name: 'Delete my data' })).toBeDisabled()
    releaseExport()
    expect(await screen.findByText(/Your transactions were downloaded/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Delete my data' })).toBeEnabled()
  })

  it('waits for the stored queue before downloading, with Confirm disabled', async () => {
    queueOverride.settled = false
    const log: ExportLog = { calls: [] }
    server.use(...coreHandlers(), exportHandler(log), http.post('*/api/v1/user/request-user-deletion', () => okEnvelope()))
    const user = userEvent.setup()
    renderPage()
    await goToCode(user)
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(log.calls).toEqual([])
    expect(screen.getByRole('button', { name: 'Delete my data' })).toBeDisabled()

    queueOverride.settled = undefined
    // any re-render reads the hook again
    act(() => onlineManager.setOnline(false))
    act(() => onlineManager.setOnline(true))
    await waitFor(() => expect(log.calls).toEqual(['a1,a2,a3,a-hidden']))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Delete my data' })).toBeEnabled())
  })

  it('does not download while a change waits to be sent', async () => {
    setToken('tok', 'u1')
    await loadPendingWrites('u1')
    await mutate((ops) => addChange(ops, { kind: 'limit', budgetId: 'b', elementId: 'e', period: '2026-11-01', amount: '1' }, { key: 'k', seq: 1 }))
    const log: ExportLog = { calls: [] }
    server.use(...coreHandlers(), exportHandler(log), http.post('*/api/v1/user/request-user-deletion', () => okEnvelope()))
    const user = userEvent.setup()
    renderPage()
    await goToCode(user)
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(log.calls).toEqual([])
  })

  it('a failed account list counts as a failed download: Confirm stays enabled and Retry reloads the accounts', async () => {
    const log: ExportLog = { calls: [] }
    let accountCalls = 0
    let accountsFail = true
    server.use(
      http.get('*/api/v1/account/get-account-list', () => {
        accountCalls++
        if (accountsFail) {
          return HttpResponse.json({ success: false, message: 'boom', code: 0, exceptionType: 'x' }, { status: 500 })
        }
        return HttpResponse.json({ success: true, message: '', data: { items: fixtureAccounts } })
      }),
      ...coreHandlers(),
      exportHandler(log),
      http.post('*/api/v1/user/request-user-deletion', () => okEnvelope()),
    )
    const user = userEvent.setup()
    renderPage()
    await goToCode(user)
    expect(await screen.findByText('The download of your transactions failed.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Delete my data' })).toBeEnabled()
    expect(log.calls).toEqual([])

    accountsFail = false
    const before = accountCalls
    await user.click(screen.getByRole('button', { name: 'Try the download again' }))
    expect(await screen.findByText(/Your transactions were downloaded/)).toBeInTheDocument()
    expect(accountCalls).toBeGreaterThan(before)
    expect(log.calls).toEqual(['a1,a2,a3,a-hidden'])
  })

  it('a failed download offers a retry and leaves Confirm enabled', async () => {
    const log: ExportLog = { calls: [] }
    server.use(
      ...coreHandlers(),
      exportHandler(log, 500),
      http.post('*/api/v1/user/request-user-deletion', () => okEnvelope()),
    )
    const user = userEvent.setup()
    renderPage()
    await goToCode(user)
    expect(await screen.findByText('The download of your transactions failed.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Delete my data' })).toBeEnabled()

    server.use(exportHandler(log))
    await user.click(screen.getByRole('button', { name: 'Try the download again' }))
    expect(await screen.findByText(/Your transactions were downloaded/)).toBeInTheDocument()
    expect(log.calls).toHaveLength(2)
  })

  it('a failed account refetch over cached accounts is a failed download, not a stuck page', async () => {
    const log: ExportLog = { calls: [] }
    server.use(
      http.get('*/api/v1/account/get-account-list', () =>
        HttpResponse.json({ success: false, message: 'boom', code: 0, exceptionType: 'x' }, { status: 500 })),
      ...coreHandlers(),
      exportHandler(log),
      http.post('*/api/v1/user/request-user-deletion', () => okEnvelope()),
    )
    const user = userEvent.setup()
    const { queryClient } = renderPage()
    queryClient.setQueryData(queryKeys.accounts, fixtureAccounts)
    await goToCode(user)
    expect(await screen.findByText('The download of your transactions failed.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Try the download again' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Delete my data' })).toBeEnabled()
    expect(log.calls).toEqual([])
  })

  it('an unreachable account list ends as a failed download under the app\'s own retry policy', async () => {
    const log: ExportLog = { calls: [] }
    server.use(
      http.get('*/api/v1/account/get-account-list', () => HttpResponse.error()),
      ...coreHandlers(),
      exportHandler(log),
      http.post('*/api/v1/user/request-user-deletion', () => okEnvelope()),
    )
    const user = userEvent.setup()
    renderPage(fixtureUser, createAppQueryClient())
    await goToCode(user)
    expect(await screen.findByText('The download of your transactions failed.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Try the download again' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Delete my data' })).toBeEnabled()
    expect(log.calls).toEqual([])
  })

  it('fetches the export\'s account list with a 30-second timeout', async () => {
    server.use(...coreHandlers(), exportHandler({ calls: [] }), http.post('*/api/v1/user/request-user-deletion', () => okEnvelope()))
    const get = vi.spyOn(api, 'get')
    const user = userEvent.setup()
    renderPage()
    await goToCode(user)
    expect(await screen.findByText(/Your transactions were downloaded/)).toBeInTheDocument()
    const listCall = get.mock.calls.find(([url]) => String(url).includes('get-account-list'))
    expect(listCall?.[1]).toMatchObject({ timeout: 30_000 })
  })

  it('exports the freshly fetched account list, not the cached one', async () => {
    const log: ExportLog = { calls: [] }
    const fresh = [...fixtureAccounts, { ...fixtureAccounts[0], id: 'a-new', name: 'Made on another device' }]
    server.use(...coreHandlers({ accounts: fresh }), exportHandler(log), http.post('*/api/v1/user/request-user-deletion', () => okEnvelope()))
    const user = userEvent.setup()
    const { queryClient } = renderPage()
    queryClient.setQueryData(queryKeys.accounts, fixtureAccounts)
    await goToCode(user)
    expect(await screen.findByText(/Your transactions were downloaded/)).toBeInTheDocument()
    expect(log.calls).toHaveLength(1)
    expect(log.calls[0].split(',').sort()).toEqual(['a-hidden', 'a-new', 'a1', 'a2', 'a3'])
  })

  it('gives the download a five-minute timeout', async () => {
    server.use(...coreHandlers(), exportHandler({ calls: [] }), http.post('*/api/v1/user/request-user-deletion', () => okEnvelope()))
    const get = vi.spyOn(api, 'get')
    const user = userEvent.setup()
    renderPage()
    await goToCode(user)
    expect(await screen.findByText(/Your transactions were downloaded/)).toBeInTheDocument()
    const exportCall = get.mock.calls.find(([url]) => String(url).includes('export-transaction-list'))
    expect(exportCall?.[1]).toMatchObject({ timeout: 5 * 60_000 })
  })

  it('makes no request for a user who owns no account', async () => {
    const log: ExportLog = { calls: [] }
    server.use(
      ...coreHandlers({ accounts: [{ ...fixtureAccounts[0], id: 'a-shared', name: 'Partner wallet', owner: partner }] }),
      exportHandler(log),
      http.post('*/api/v1/user/request-user-deletion', () => okEnvelope()),
    )
    const user = userEvent.setup()
    renderPage()
    await goToCode(user)
    expect(await screen.findByText('You have no accounts of your own to export.')).toBeInTheDocument()
    expect(log.calls).toEqual([])
  })
})

describe('the confirm', () => {
  function codePhaseHandlers() {
    return [...coreHandlers(), exportHandler({ calls: [] }), http.post('*/api/v1/user/request-user-deletion', () => okEnvelope())]
  }

  it('POSTs the code, clears the local data and goes to the sign-in page', async () => {
    let body: unknown
    let flagWhenSent: string | null = null
    server.use(
      ...codePhaseHandlers(),
      http.post('*/api/v1/user/confirm-user-deletion', async ({ request }) => {
        body = await request.json()
        flagWhenSent = localStorage.getItem(CLEANUP_KEY)
        return okEnvelope()
      }),
    )
    setToken('tok', 'u1')
    localStorage.setItem('econumo.query-cache', '{"stale":"finances"}')
    const user = userEvent.setup()
    renderPage()
    await goToCode(user)
    await screen.findByText(/Your transactions were downloaded/)
    await user.type(screen.getByLabelText('Confirmation code'), '482913')
    await user.click(screen.getByRole('button', { name: 'Delete my data' }))

    await waitFor(() => expect(assign).toHaveBeenCalledWith('/login?reason=deletion-scheduled'))
    expect(body).toEqual({ code: '482913' })
    expect(Number(flagWhenSent)).toBeGreaterThan(0)
    expect(localStorage.getItem('token')).toBeNull()
    expect(localStorage.getItem('econumo.query-cache')).toBeNull()
    expect(localStorage.getItem(CLEANUP_KEY)).toBeNull()
  })

  it('a refused code shows the server message, stays on the code phase and drops the flag', async () => {
    server.use(
      ...codePhaseHandlers(),
      http.post('*/api/v1/user/confirm-user-deletion', () =>
        HttpResponse.json({ success: false, message: 'The confirmation code is not valid.', code: 400, errors: {} }, { status: 400 }),
      ),
    )
    setToken('tok', 'u1')
    const user = userEvent.setup()
    renderPage()
    await goToCode(user)
    await screen.findByText(/Your transactions were downloaded/)
    await user.type(screen.getByLabelText('Confirmation code'), '999999')
    await user.click(screen.getByRole('button', { name: 'Delete my data' }))
    expect(await screen.findByText('The confirmation code is not valid.')).toBeInTheDocument()
    expect(screen.getByLabelText('Confirmation code')).toBeInTheDocument()
    expect(localStorage.getItem(CLEANUP_KEY)).toBeNull()
    expect(localStorage.getItem('token')).toBe('tok')
    expect(assign).not.toHaveBeenCalled()
  })

  it('is disabled while a change waits to be sent', async () => {
    setToken('tok', 'u1')
    await loadPendingWrites('u1')
    await mutate((ops) => addChange(ops, { kind: 'limit', budgetId: 'b', elementId: 'e', period: '2026-11-01', amount: '1' }, { key: 'k', seq: 1 }))
    server.use(...codePhaseHandlers())
    const user = userEvent.setup()
    renderPage()
    await goToCode(user)
    expect(screen.getByText(/Some of your changes are still waiting to be sent/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Delete my data' })).toBeDisabled()
  })

  it('is enabled with the queue empty', async () => {
    setToken('tok', 'u1')
    await loadPendingWrites('u1')
    server.use(...codePhaseHandlers())
    const user = userEvent.setup()
    renderPage()
    await goToCode(user)
    expect(screen.queryByText(/Some of your changes are still waiting to be sent/)).not.toBeInTheDocument()
    await waitFor(() => expect(screen.getByRole('button', { name: 'Delete my data' })).toBeEnabled())
  })

  it('a refused change does not block Confirm', async () => {
    setToken('tok', 'u1')
    await loadPendingWrites('u1')
    await mutate((ops) => settleRefused(
      addChange(ops, { kind: 'limit', budgetId: 'b', elementId: 'e', period: '2026-11-01', amount: '1' }, { key: 'k', seq: 1 }), 'k', 'refused'))
    server.use(...codePhaseHandlers())
    const user = userEvent.setup()
    renderPage()
    await goToCode(user)
    await waitFor(() => expect(screen.getByRole('button', { name: 'Delete my data' })).toBeEnabled())
  })

  it('a 401 on a first attempt is an ordinary expired session: nothing is wiped', async () => {
    setToken('tok', 'u1')
    await loadPendingWrites('u1')
    await mutate((ops) => settleRefused(
      addChange(ops, { kind: 'limit', budgetId: 'b', elementId: 'e', period: '2026-11-01', amount: '1' }, { key: 'k', seq: 1 }), 'k', 'refused'))
    localStorage.setItem('econumo.query-cache', '{"stale":"finances"}')
    server.use(
      ...codePhaseHandlers(),
      http.post('*/api/v1/user/confirm-user-deletion', () =>
        HttpResponse.json({ success: false, message: 'Invalid access token', code: 0, errors: {} }, { status: 401 }),
      ),
    )
    const user = userEvent.setup()
    renderPage()
    await goToCode(user)
    await screen.findByText(/Your transactions were downloaded/)
    await user.type(screen.getByLabelText('Confirmation code'), '482913')
    await user.click(screen.getByRole('button', { name: 'Delete my data' }))

    await waitFor(() => expect(assign).toHaveBeenCalledWith('/login?reason=expired'))
    expect(assign).not.toHaveBeenCalledWith('/login?reason=deletion-scheduled')
    expect(localStorage.getItem('token')).toBeNull()
    expect(localStorage.getItem(CLEANUP_KEY)).toBeNull()
    expect(localStorage.getItem('econumo.query-cache')).toBe('{"stale":"finances"}')
    expect(getOps()).toHaveLength(1)
  })

  it('a 401 on another request while the confirm is in flight wipes nothing; the confirm decides', async () => {
    setToken('tok', 'u1')
    await loadPendingWrites('u1')
    await mutate((ops) => settleRefused(
      addChange(ops, { kind: 'limit', budgetId: 'b', elementId: 'e', period: '2026-11-01', amount: '1' }, { key: 'k', seq: 1 }), 'k', 'refused'))
    let releaseConfirm: () => void = () => {}
    const confirmHeld = new Promise<void>((resolve) => {
      releaseConfirm = resolve
    })
    let confirmArrived = false
    server.use(
      ...codePhaseHandlers(),
      http.post('*/api/v1/user/confirm-user-deletion', async () => {
        confirmArrived = true
        await confirmHeld
        return HttpResponse.json({ success: false, message: 'Invalid access token', code: 0, errors: {} }, { status: 401 })
      }),
      http.get('*/api/v1/secure', () =>
        HttpResponse.json({ success: false, message: 'Invalid access token', code: 0, errors: {} }, { status: 401 }),
      ),
    )
    const user = userEvent.setup()
    renderPage()
    await goToCode(user)
    await screen.findByText(/Your transactions were downloaded/)
    await user.type(screen.getByLabelText('Confirmation code'), '482913')
    await user.click(screen.getByRole('button', { name: 'Delete my data' }))
    await waitFor(() => expect(confirmArrived).toBe(true))

    await expect(api.get(apiUrl('/api/v1/secure'))).rejects.toThrow()
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(assign).not.toHaveBeenCalled()
    expect(getOps()).toHaveLength(1)

    releaseConfirm()
    await waitFor(() => expect(assign).toHaveBeenCalledWith('/login?reason=expired'))
    expect(assign).not.toHaveBeenCalledWith('/login?reason=deletion-scheduled')
    expect(getOps()).toHaveLength(1)
  })

  describe('the in-flight marker is cleared once the confirm settles', () => {
    // a 401 elsewhere with a fresh flag finishes the clean-up only when no confirm is in flight
    async function siblingUnauthorizedRunsCleanup() {
      server.use(http.get('*/api/v1/secure', () =>
        HttpResponse.json({ success: false, message: 'Invalid access token', code: 0, errors: {} }, { status: 401 })))
      localStorage.setItem(CLEANUP_KEY, String(Date.now()))
      assign.mockClear()
      await expect(api.get(apiUrl('/api/v1/secure'))).rejects.toThrow()
      await waitFor(() => expect(assign).toHaveBeenCalledWith('/login?reason=deletion-scheduled'))
    }

    async function confirmWith(response: () => Response) {
      server.use(...codePhaseHandlers(), http.post('*/api/v1/user/confirm-user-deletion', response))
      setToken('tok', 'u1')
      const user = userEvent.setup()
      renderPage()
      await goToCode(user)
      await screen.findByText(/Your transactions were downloaded/)
      await user.type(screen.getByLabelText('Confirmation code'), '482913')
      await user.click(screen.getByRole('button', { name: 'Delete my data' }))
    }

    it('after a success, only once the page finished its own clean-up', async () => {
      let releaseSignOut: () => void = () => {}
      signOutGate.hold = new Promise<void>((resolve) => {
        releaseSignOut = resolve
      })
      server.use(http.get('*/api/v1/secure', () =>
        HttpResponse.json({ success: false, message: 'Invalid access token', code: 0, errors: {} }, { status: 401 })))
      await confirmWith(() => okEnvelope())
      await waitFor(() => expect(signOutGate.calls).toBe(1))
      // a sibling 401 while the page signs out must not start a second clean-up
      await expect(api.get(apiUrl('/api/v1/secure'))).rejects.toThrow()
      await new Promise((resolve) => setTimeout(resolve, 20))
      expect(signOutGate.calls).toBe(1)

      signOutGate.hold = null
      releaseSignOut()
      await waitFor(() => expect(assign).toHaveBeenCalledWith('/login?reason=deletion-scheduled'))
      expect(signOutGate.calls).toBe(1)
      setToken('tok', 'u1')
      await siblingUnauthorizedRunsCleanup()
    })

    it('after a timed-out confirm, which keeps the flag', async () => {
      const post = api.post.bind(api)
      let confirmConfig: unknown
      vi.spyOn(api, 'post').mockImplementation((url, data, config) => {
        if (String(url).includes('confirm-user-deletion')) {
          confirmConfig = config
          return Promise.reject(new AxiosError('timeout of 60000ms exceeded', AxiosError.ECONNABORTED))
        }
        return post(url, data, config)
      })
      await confirmWith(() => okEnvelope())
      expect(await screen.findByText(/timeout|Something went wrong|try again/i)).toBeInTheDocument()
      expect(confirmConfig).toMatchObject({ timeout: 60_000 })
      expect(Number(localStorage.getItem(CLEANUP_KEY))).toBeGreaterThan(0)
      expect(assign).not.toHaveBeenCalled()
      await siblingUnauthorizedRunsCleanup()
    })

    it('after a refused code', async () => {
      await confirmWith(() =>
        HttpResponse.json({ success: false, message: 'The confirmation code is not valid.', code: 400, errors: {} }, { status: 400 }))
      expect(await screen.findByText('The confirmation code is not valid.')).toBeInTheDocument()
      await siblingUnauthorizedRunsCleanup()
    })

    it('after a network error', async () => {
      await confirmWith(() => HttpResponse.error())
      await waitFor(() => expect(screen.getByRole('button', { name: 'Delete my data' })).toBeEnabled())
      await siblingUnauthorizedRunsCleanup()
    })
  })

  it('a 502 keeps the flag: the mark may have committed behind the proxy', async () => {
    server.use(
      ...codePhaseHandlers(),
      http.post('*/api/v1/user/confirm-user-deletion', () =>
        HttpResponse.json({ success: false, message: 'Bad gateway', code: 0, exceptionType: 'x' }, { status: 502 })),
    )
    setToken('tok', 'u1')
    const user = userEvent.setup()
    renderPage()
    await goToCode(user)
    await screen.findByText(/Your transactions were downloaded/)
    await user.type(screen.getByLabelText('Confirmation code'), '482913')
    await user.click(screen.getByRole('button', { name: 'Delete my data' }))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Delete my data' })).toBeEnabled())
    expect(Number(localStorage.getItem(CLEANUP_KEY))).toBeGreaterThan(0)
    expect(assign).not.toHaveBeenCalled()
  })

  it('a 401 after an attempt that got no response finishes the deletion clean-up', async () => {
    setToken('tok', 'u1')
    await loadPendingWrites('u1')
    await mutate((ops) => settleRefused(
      addChange(ops, { kind: 'limit', budgetId: 'b', elementId: 'e', period: '2026-11-01', amount: '1' }, { key: 'k', seq: 1 }), 'k', 'refused'))
    let attempts = 0
    server.use(
      ...codePhaseHandlers(),
      http.post('*/api/v1/user/confirm-user-deletion', () => {
        attempts++
        if (attempts === 1) {
          return HttpResponse.error()
        }
        return HttpResponse.json({ success: false, message: 'Invalid access token', code: 0, errors: {} }, { status: 401 })
      }),
    )
    const user = userEvent.setup()
    renderPage()
    await goToCode(user)
    await screen.findByText(/Your transactions were downloaded/)
    await user.type(screen.getByLabelText('Confirmation code'), '482913')
    await user.click(screen.getByRole('button', { name: 'Delete my data' }))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Delete my data' })).toBeEnabled())
    expect(Number(localStorage.getItem(CLEANUP_KEY))).toBeGreaterThan(0)
    expect(assign).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: 'Delete my data' }))
    await waitFor(() => expect(assign).toHaveBeenCalledWith('/login?reason=deletion-scheduled'))
    expect(attempts).toBe(2)
    expect(localStorage.getItem('token')).toBeNull()
    expect(localStorage.getItem(CLEANUP_KEY)).toBeNull()
    await waitFor(() => expect(getOps()).toHaveLength(0))
  })
})

it('offline, both submit buttons are disabled', async () => {
  server.use(...coreHandlers(), exportHandler({ calls: [] }), http.post('*/api/v1/user/request-user-deletion', () => okEnvelope()))
  const user = userEvent.setup()
  renderPage()
  act(() => onlineManager.setOnline(false))
  expect(screen.getByRole('button', { name: 'Send confirmation code' })).toBeDisabled()
  act(() => onlineManager.setOnline(true))
  await goToCode(user)
  act(() => onlineManager.setOnline(false))
  expect(screen.getByRole('button', { name: 'Delete my data' })).toBeDisabled()
})

describe('the sign-in page after a deletion request', () => {
  function renderSignIn() {
    server.use(http.get('*/api/v1/oauth/get-provider-list', () => HttpResponse.json({ success: true, message: '', data: [] })))
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const router = createMemoryRouter([{ path: '/login', element: <LoginPage /> }], { initialEntries: ['/login?reason=deletion-scheduled'] })
    render(
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} />
      </QueryClientProvider>,
    )
  }

  it('says in how many days the data will be deleted', async () => {
    window.econumoConfig = { USER_DELETION_DAYS: 30 }
    renderSignIn()
    expect(await screen.findByText('Your data will be deleted in 30 days. Sign in before then to keep it.')).toBeInTheDocument()
  })

  it('uses the singular for one day', async () => {
    window.econumoConfig = { USER_DELETION_DAYS: 1 }
    renderSignIn()
    expect(await screen.findByText('Your data will be deleted in 1 day. Sign in before then to keep it.')).toBeInTheDocument()
  })

  it('promises no date when the operator deletes by hand', async () => {
    window.econumoConfig = { USER_DELETION_DAYS: 0 }
    renderSignIn()
    expect(
      await screen.findByText('Your data will be deleted after a manual review of your request. Sign in before then to keep it.'),
    ).toBeInTheDocument()
    expect(screen.queryByText(/in 0 days/)).not.toBeInTheDocument()
  })

  it('promises nothing about timing when the page talks to another server', async () => {
    window.econumoConfig = { ALLOW_CUSTOM_API: true, USER_DELETION_DAYS: 30 }
    selfHosted(true)
    backendHost('https://other.example.test')
    renderSignIn()
    expect(await screen.findByText('Your data is scheduled for deletion. Sign in to keep it.')).toBeInTheDocument()
    expect(screen.queryByText(/30 days/)).not.toBeInTheDocument()
  })
})
