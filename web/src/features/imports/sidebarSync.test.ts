import { QueryClient } from '@tanstack/react-query'
import * as importsApi from '@/api/imports'
import { loadStoredKey } from '@/lib/importCrypto'
import { notePullSync, SIDEBAR_SYNC_INTERVAL_MS, syncPullSourcesIfDue } from './sidebarSync'

vi.mock('@/lib/importCrypto', () => ({
  loadStoredKey: vi.fn(async () => ({ tag: 'wrapped' })),
  decryptCredential: vi.fn(async () => 'https://u:p@bridge.example/simplefin'),
}))
vi.mock('@/api/imports', () => ({
  getImportSourceList: vi.fn(),
  getImportCredentialKey: vi.fn(),
  syncImportSource: vi.fn(),
}))

const bank = { id: 's1', provider: 'simplefin', name: 'Bank', credentialCiphertext: 'v1:iv:ct', lastSyncedAt: '', cards: [] }
const wallet = { id: 's2', provider: 'apple-wallet', name: 'iPhone', credentialCiphertext: '', lastSyncedAt: '', cards: [] }
const run = { trigger: 'manual', status: 'completed', importedCount: 2, matchedCount: 1, amountsUpdatedCount: 0 }

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = { IMPORT_SIMPLEFIN: true }
  Object.defineProperty(window, 'isSecureContext', { value: true, configurable: true })
  vi.mocked(importsApi.getImportSourceList).mockResolvedValue([bank, wallet] as never)
  vi.mocked(importsApi.getImportCredentialKey).mockResolvedValue({ wrappedDataKey: 'wrapped', kdf: '' } as never)
  vi.mocked(importsApi.syncImportSource).mockReset().mockResolvedValue({ run, accounts: [] } as never)
  vi.mocked(loadStoredKey).mockResolvedValue({ tag: 'wrapped' } as never)
})

it('syncs every connected SimpleFIN source, then waits ten minutes before the next one', async () => {
  const now = Date.now()
  expect(await syncPullSourcesIfDue(new QueryClient(), now)).toEqual({ imported: 2, matched: 1, failed: 0 })
  expect(importsApi.syncImportSource).toHaveBeenCalledTimes(1)
  expect(vi.mocked(importsApi.syncImportSource).mock.calls[0][0]).toBe('s1')
  expect(await syncPullSourcesIfDue(new QueryClient(), now + SIDEBAR_SYNC_INTERVAL_MS - 1000)).toBeNull()
  expect(importsApi.syncImportSource).toHaveBeenCalledTimes(1)
  expect(await syncPullSourcesIfDue(new QueryClient(), now + SIDEBAR_SYNC_INTERVAL_MS + 1000)).not.toBeNull()
  expect(importsApi.syncImportSource).toHaveBeenCalledTimes(2)
})

it('a sync from the SimpleFIN page counts too', async () => {
  notePullSync(Date.now())
  expect(await syncPullSourcesIfDue(new QueryClient())).toBeNull()
  expect(importsApi.syncImportSource).not.toHaveBeenCalled()
})

it('does nothing while the key is locked on this device, and does not start the wait', async () => {
  vi.mocked(loadStoredKey).mockResolvedValue(null)
  expect(await syncPullSourcesIfDue(new QueryClient())).toBeNull()
  vi.mocked(loadStoredKey).mockResolvedValue({ tag: 'wrapped' } as never)
  expect(await syncPullSourcesIfDue(new QueryClient())).not.toBeNull()
})

it('does nothing with SimpleFIN off, on a plain-http page or without a SimpleFIN source', async () => {
  window.econumoConfig = { IMPORT_SIMPLEFIN: false }
  expect(await syncPullSourcesIfDue(new QueryClient())).toBeNull()
  window.econumoConfig = { IMPORT_SIMPLEFIN: true }
  Object.defineProperty(window, 'isSecureContext', { value: false, configurable: true })
  expect(await syncPullSourcesIfDue(new QueryClient())).toBeNull()
  Object.defineProperty(window, 'isSecureContext', { value: true, configurable: true })
  vi.mocked(importsApi.getImportSourceList).mockResolvedValue([wallet] as never)
  expect(await syncPullSourcesIfDue(new QueryClient())).toBeNull()
  expect(importsApi.syncImportSource).not.toHaveBeenCalled()
})

it('a failed sync counts as failed and still starts the wait', async () => {
  vi.mocked(importsApi.syncImportSource).mockRejectedValue(new Error('bridge down'))
  expect(await syncPullSourcesIfDue(new QueryClient())).toEqual({ imported: 0, matched: 0, failed: 1 })
  expect(await syncPullSourcesIfDue(new QueryClient())).toBeNull()
})
