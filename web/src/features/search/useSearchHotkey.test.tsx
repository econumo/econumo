import { act, fireEvent, renderHook } from '@testing-library/react'
import { useUiStore } from '@/app/uiStore'
import { METRICS, trackEvent } from '@/lib/metrics'
import { useSearchHotkey } from './useSearchHotkey'

vi.mock('@/lib/metrics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/metrics')>()
  return { ...actual, trackEvent: vi.fn() }
})

beforeEach(() => {
  useUiStore.setState({ searchOpen: false })
  vi.mocked(trackEvent).mockClear()
})

it('Ctrl+K and ⌘K open search', () => {
  renderHook(() => useSearchHotkey())
  fireEvent.keyDown(window, { key: 'k', ctrlKey: true })
  expect(useUiStore.getState().searchOpen).toBe(true)
  act(() => useUiStore.getState().closeSearch())
  fireEvent.keyDown(window, { key: 'K', metaKey: true })
  expect(useUiStore.getState().searchOpen).toBe(true)
})

it('a bare K or another chord does nothing', () => {
  renderHook(() => useSearchHotkey())
  fireEvent.keyDown(window, { key: 'k' })
  fireEvent.keyDown(window, { key: 'j', ctrlKey: true })
  expect(useUiStore.getState().searchOpen).toBe(false)
})

it('is ignored while another dialog is open', () => {
  renderHook(() => useSearchHotkey())
  const d = document.createElement('div')
  d.setAttribute('role', 'dialog')
  document.body.append(d)
  fireEvent.keyDown(window, { key: 'k', ctrlKey: true })
  expect(useUiStore.getState().searchOpen).toBe(false)
  d.remove()
})

it('fires the open metric once, and not again while already open', () => {
  renderHook(() => useSearchHotkey())
  fireEvent.keyDown(window, { key: 'k', ctrlKey: true })
  fireEvent.keyDown(window, { key: 'k', ctrlKey: true })
  expect(trackEvent).toHaveBeenCalledTimes(1)
  expect(trackEvent).toHaveBeenCalledWith(METRICS.GLOBAL_SEARCH_OPEN)
})

it('stops listening on unmount', () => {
  const { unmount } = renderHook(() => useSearchHotkey())
  unmount()
  fireEvent.keyDown(window, { key: 'k', ctrlKey: true })
  expect(useUiStore.getState().searchOpen).toBe(false)
})
