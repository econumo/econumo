import { afterEach, expect, it, vi } from 'vitest'
import { copyText } from './clipboard'

afterEach(() => {
  vi.unstubAllGlobals()
  Reflect.deleteProperty(document, 'execCommand')
})

function stubExecCommand(result: boolean) {
  const execCommand = vi.fn(() => result)
  Object.defineProperty(document, 'execCommand', { value: execCommand, configurable: true })
  return execCommand
}

it('uses the async clipboard API when it is available', async () => {
  const writeText = vi.fn().mockResolvedValue(undefined)
  vi.stubGlobal('navigator', { ...window.navigator, clipboard: { writeText } })
  await expect(copyText('eco_pat_value')).resolves.toBe(true)
  expect(writeText).toHaveBeenCalledWith('eco_pat_value')
})

// An insecure context (plain http on a LAN IP) has no navigator.clipboard at all.
it('falls back to execCommand when navigator.clipboard is missing', async () => {
  vi.stubGlobal('navigator', { ...window.navigator, clipboard: undefined })
  const execCommand = stubExecCommand(true)
  await expect(copyText('eco_pat_value')).resolves.toBe(true)
  expect(execCommand).toHaveBeenCalledWith('copy')
  expect(document.querySelector('textarea')).toBeNull()
})

it('falls back to execCommand when writeText rejects', async () => {
  const writeText = vi.fn().mockRejectedValue(new DOMException('denied'))
  vi.stubGlobal('navigator', { ...window.navigator, clipboard: { writeText } })
  stubExecCommand(true)
  await expect(copyText('eco_pat_value')).resolves.toBe(true)
})

it('reports failure when neither path works', async () => {
  vi.stubGlobal('navigator', { ...window.navigator, clipboard: undefined })
  stubExecCommand(false)
  await expect(copyText('eco_pat_value')).resolves.toBe(false)
  expect(document.querySelector('textarea')).toBeNull()
})
