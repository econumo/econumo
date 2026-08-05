import { afterEach, expect, it, vi } from 'vitest'
import { downloadBlob } from './download'

afterEach(() => {
  delete (window as { Capacitor?: unknown }).Capacitor
})

it('writes the file and opens the share sheet in app mode', async () => {
  const writeFile = vi.fn(async () => ({ uri: 'file:///cache/transactions.csv' }))
  const share = vi.fn(async () => ({}))
  window.Capacitor = {
    isNativePlatform: () => true,
    Plugins: { Filesystem: { writeFile }, Share: { share } },
  }
  downloadBlob(new Blob(['a,b\n1,2'], { type: 'text/csv' }), 'transactions.csv')
  await vi.waitFor(() => expect(share).toHaveBeenCalled())
  expect(writeFile).toHaveBeenCalledWith({
    path: 'transactions.csv',
    data: expect.any(String),
    directory: 'CACHE',
  })
  expect(share).toHaveBeenCalledWith({ url: 'file:///cache/transactions.csv' })
})

it('swallows a share-sheet cancellation rejection', async () => {
  const writeFile = vi.fn(async () => ({ uri: 'file:///cache/transactions.csv' }))
  const share = vi.fn(async () => {
    throw new Error('cancelled')
  })
  window.Capacitor = {
    isNativePlatform: () => true,
    Plugins: { Filesystem: { writeFile }, Share: { share } },
  }
  const onUnhandledRejection = vi.fn()
  window.addEventListener('unhandledrejection', onUnhandledRejection)
  downloadBlob(new Blob(['a,b\n1,2'], { type: 'text/csv' }), 'transactions.csv')
  await vi.waitFor(() => expect(writeFile).toHaveBeenCalled())
  await vi.waitFor(() => expect(share).toHaveBeenCalled())
  await new Promise((resolve) => setTimeout(resolve, 0))
  window.removeEventListener('unhandledrejection', onUnhandledRejection)
  expect(onUnhandledRejection).not.toHaveBeenCalled()
})

it('keeps the anchor download on the web', () => {
  const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
  downloadBlob(new Blob(['x']), 'x.csv')
  expect(click).toHaveBeenCalled()
  click.mockRestore()
})
