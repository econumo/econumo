import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { IDBFactory } from 'fake-indexeddb'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { createKey, encryptCredential } from '@/lib/importCrypto'
import { SimpleFINPage } from './SimpleFINPage'

vi.mock('@/hooks/useIsCompact', () => ({ useIsCompact: () => false }))
vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))

const ACCESS_URL = 'https://u:p@bridge.example/simplefin'
const source = (over: Record<string, unknown> = {}) => ({
  id: 's2', provider: 'simplefin', name: 'SimpleFIN', status: 'active', createdAt: '2026-09-01 00:00:00',
  lastSyncedAt: '', credentialCiphertext: '', cards: [], ...over,
})
const run = (over: Record<string, unknown> = {}) => ({
  id: 'r1', sourceId: 's2', provider: 'simplefin', status: 'completed', trigger: 'manual',
  importedCount: 2, matchedCount: 1, amountsUpdatedCount: 0, queuedCount: 0, skippedCount: 0, failedCount: 0,
  errors: [], startedAt: '2026-09-07 10:00:00', finishedAt: '2026-09-07 10:00:02', ...over,
})

function renderPage(data: Record<string, unknown>) {
  server.use(...coreHandlers(data))
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter([{ path: '/settings/simplefin', element: <SimpleFINPage /> }], { initialEntries: ['/settings/simplefin'] })
  render(<QueryClientProvider client={queryClient}><RouterProvider router={router} /></QueryClientProvider>)
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  globalThis.indexedDB = new IDBFactory()
})

it('first connect: creates a key, claims the token, and stores only ciphertext', async () => {
  const posted: Record<string, unknown> = {}
  server.use(
    http.post('*/api/v1/import/set-credential-key', async ({ request }) => {
      posted.key = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { ...(posted.key as object), updatedAt: '2026-09-07 10:00:00' } })
    }),
    http.post('*/api/v1/import/claim-setup-token', async ({ request }) => {
      posted.claim = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { accessUrl: ACCESS_URL } })
    }),
    http.post('*/api/v1/import/create-source', async ({ request }) => {
      posted.source = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { item: source({ credentialCiphertext: (posted.source as { credentialCiphertext: string }).credentialCiphertext }) } })
    }),
    http.post('*/api/v1/import/list-external-accounts', () => HttpResponse.json({ success: true, message: '', data: { items: [] } })),
  )
  renderPage({ importSources: [] })
  const user = userEvent.setup()
  await user.type(await screen.findByLabelText('Setup token'), 'tok-1')
  await user.type(screen.getByLabelText('Passphrase'), 'correct horse')
  await user.type(screen.getByPlaceholderText('Repeat passphrase'), 'correct horse')
  await user.click(screen.getByRole('button', { name: 'Connect' }))
  await waitFor(() => expect(posted.source).toBeDefined(), { timeout: 15_000 })
  expect(posted.claim).toEqual({ setupToken: 'tok-1' })
  expect((posted.key as { kdf: string }).kdf).toContain('PBKDF2-SHA256')
  const src = posted.source as Record<string, string>
  expect(src.provider).toBe('simplefin')
  expect(src.credentialCiphertext).toMatch(/^v1:/)
  expect(JSON.stringify(posted)).not.toContain(ACCESS_URL)
  expect(await screen.findByRole('button', { name: 'Sync now' })).toBeInTheDocument()
}, 20_000)

it('locked device: wrong passphrase is rejected, right one unlocks and lists bridge accounts', async () => {
  const wrapped = await createKey('pw')
  const ciphertext = await encryptCredential(ACCESS_URL)
  globalThis.indexedDB = new IDBFactory()  // "another device": server has the key, this one does not
  let listBody: unknown
  server.use(http.post('*/api/v1/import/list-external-accounts', async ({ request }) => {
    listBody = await request.json()
    return HttpResponse.json({ success: true, message: '', data: { items: [
      { externalAccountId: 'ACT-CHK', externalName: 'Checking', externalCurrency: 'USD', balance: '100', orgName: 'Bank', state: 'unmapped', accountId: '' },
    ] } })
  }))
  renderPage({ importSources: [source({ credentialCiphertext: ciphertext })], importCredentialKey: { ...wrapped, updatedAt: '2026-09-07 10:00:00' } })
  const user = userEvent.setup()
  await user.type(await screen.findByPlaceholderText('Passphrase'), 'nope')
  await user.click(screen.getByRole('button', { name: 'Unlock' }))
  expect(await screen.findByRole('alert', {}, { timeout: 15_000 })).toHaveTextContent('Wrong passphrase.')
  await user.clear(screen.getByPlaceholderText('Passphrase'))
  await user.type(screen.getByPlaceholderText('Passphrase'), 'pw')
  await user.click(screen.getByRole('button', { name: 'Unlock' }))
  expect(await screen.findByText('Checking', {}, { timeout: 15_000 })).toBeInTheDocument()
  expect(listBody).toEqual({ sourceId: 's2', accessUrl: ACCESS_URL })
}, 40_000)

it('stale device: a key reset elsewhere asks for the new passphrase instead of a reconnect', async () => {
  await createKey('old')  // this device unlocked under the previous passphrase
  const thisDevice = globalThis.indexedDB
  globalThis.indexedDB = new IDBFactory()
  const replacement = await createKey('new')  // ...then another device reset the key
  const ciphertext = await encryptCredential(ACCESS_URL)
  globalThis.indexedDB = thisDevice
  renderPage({ importSources: [source({ credentialCiphertext: ciphertext })], importCredentialKey: { ...replacement, updatedAt: '2026-09-08 10:00:00' } })
  expect(await screen.findByText(/passphrase was changed on another device/)).toBeInTheDocument()
  expect(screen.queryByPlaceholderText('aHR0cHM6Ly9…')).not.toBeInTheDocument()
  const user = userEvent.setup()
  await user.type(screen.getByPlaceholderText('Passphrase'), 'new')
  await user.click(screen.getByRole('button', { name: 'Unlock' }))
  expect(await screen.findByRole('button', { name: 'Sync now' }, { timeout: 15_000 })).toBeInTheDocument()
}, 40_000)

it('unlocked device: Sync now posts the decrypted access URL and shows the run summary', async () => {
  const wrapped = await createKey('pw')
  const ciphertext = await encryptCredential(ACCESS_URL)
  let syncBody: unknown
  server.use(
    http.post('*/api/v1/import/list-external-accounts', () => HttpResponse.json({ success: true, message: '', data: { items: [] } })),
    http.post('*/api/v1/import/sync-source', async ({ request }) => {
      syncBody = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { run: run({ status: 'partial', failedCount: 1, errors: [{ externalAccountId: 'ACT-SAV', message: 'unsupported currency' }] }), accounts: [] } })
    }),
  )
  renderPage({
    importSources: [source({ credentialCiphertext: ciphertext, lastSyncedAt: '2026-09-05 08:00:00' })],
    importCredentialKey: { ...wrapped, updatedAt: '2026-09-07 10:00:00' },
    importRuns: [],
  })
  // register AFTER renderPage's own coreHandlers() call so this overrides its
  // default empty run list (msw resolves the most recently added handler first)
  server.use(http.get('*/api/v1/import/get-run-list', () => HttpResponse.json({ success: true, message: '', data: { items: [
    run({ status: 'partial', failedCount: 1, errors: [{ externalAccountId: 'ACT-SAV', message: 'unsupported currency' }] }),
  ] } })))
  const user = userEvent.setup()
  expect(await screen.findByText('Last synced 2026-09-05 08:00:00')).toBeInTheDocument()
  await user.click(await screen.findByRole('button', { name: 'Sync now' }))
  await waitFor(() => expect(syncBody).toEqual({ sourceId: 's2', accessUrl: ACCESS_URL, startDate: '2026-09-02' }))
  expect(await screen.findByText('Completed with errors')).toBeInTheDocument()
  expect(screen.getByText(/ACT-SAV: unsupported currency/)).toBeInTheDocument()
})
