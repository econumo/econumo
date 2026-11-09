import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider, onlineManager } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { queryKeys } from '@/app/queryKeys'
import type { ImportCredentialKeyDto } from '@/api/dto/imports'
import { useImportKey } from './useImportKey'

vi.mock('@/lib/importCrypto', () => ({ loadStoredKey: vi.fn(async () => null) }))

it('offline, the key stored on this device is still read', async () => {
  onlineManager.setOnline(false)
  onTestFinished(() => onlineManager.setOnline(true))
  const client = new QueryClient()
  const wrapped = { wrappedDataKey: 'wrapped' } as ImportCredentialKeyDto
  client.setQueryData(queryKeys.importCredentialKey, wrapped)
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>
  const { result } = renderHook(() => useImportKey(), { wrapper })
  await waitFor(() => expect(result.current.state.status).toBe('locked'))
})
