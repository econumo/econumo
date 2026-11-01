import type { ReactNode } from 'react'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { useClassificationMenu } from './classificationMenu'

const SHARE = 'Share as trip page'
const stamp = { createdAt: '2026-01-01 00:00:00', updatedAt: '2026-01-01 00:00:00' }

function setup() {
  server.use(
    ...coreHandlers({
      tags: [
        { id: 'own-tag', ownerUserId: 'u1', name: 'trip', icon: 'flight', position: 0, isArchived: 0, ...stamp },
        { id: 'shared-tag', ownerUserId: 'someone-else', name: 'shared', icon: 'flight', position: 1, isArchived: 0, ...stamp },
      ],
    }),
  )
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>
  return renderHook(() => useClassificationMenu('b1'), { wrapper })
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = { SHARE_URL: 'https://share.example.test' }
})

it('a tag line offers sharing as a trip page, enabled for own and shared tags', async () => {
  const { result } = setup()
  await waitFor(() => {
    const own = result.current.actionsFor({ kind: 'tag', id: 'own-tag' }).find((a) => a.label === SHARE)
    expect(own).toBeDefined()
    expect(own?.disabled).toBeFalsy()
  })
  const open = vi.spyOn(window, 'open').mockReturnValue(null)
  const shared = result.current.actionsFor({ kind: 'tag', id: 'shared-tag' }).find((a) => a.label === SHARE)
  expect(shared?.disabled).toBeFalsy()
  shared?.onSelect()
  expect(open).toHaveBeenCalledWith('https://share.example.test/new?kind=trip&tag=shared-tag', '_blank', 'noopener,noreferrer')
  open.mockRestore()
})

it('a category line never offers it', async () => {
  const { result } = setup()
  expect(result.current.actionsFor({ kind: 'category', id: 'c1' }).map((a) => a.label)).not.toContain(SHARE)
})

it('is absent without a share url', async () => {
  window.econumoConfig = { SHARE_URL: '' }
  const { result } = setup()
  expect(result.current.actionsFor({ kind: 'tag', id: 'own-tag' }).map((a) => a.label)).not.toContain(SHARE)
})
