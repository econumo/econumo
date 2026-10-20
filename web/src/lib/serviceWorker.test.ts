import { refreshServiceWorkerCache } from './serviceWorker'

function stubWorker(answer: boolean) {
  const postMessage = vi.fn((_msg: unknown, [port]: MessagePort[]) => {
    if (answer) port.postMessage({ ok: true })
  })
  const update = vi.fn().mockResolvedValue(undefined)
  Object.defineProperty(navigator, 'serviceWorker', {
    configurable: true,
    value: { getRegistration: vi.fn().mockResolvedValue({ active: { postMessage }, update }) },
  })
  return { postMessage, update }
}

beforeEach(() => {
  vi.stubEnv('PROD', true)
  delete window.Capacitor
})

afterEach(() => {
  vi.unstubAllEnvs()
  vi.useRealTimers()
  Reflect.deleteProperty(navigator, 'serviceWorker')
})

it('asks the worker to refresh its cache and waits for the answer', async () => {
  const { postMessage, update } = stubWorker(true)
  await refreshServiceWorkerCache()
  expect(postMessage).toHaveBeenCalledWith({ type: 'refresh' }, expect.any(Array))
  expect(update).toHaveBeenCalled()
})

it('stops waiting for a worker that never answers', async () => {
  vi.useFakeTimers()
  stubWorker(false)
  const done = refreshServiceWorkerCache()
  await vi.advanceTimersByTimeAsync(60_000)
  await expect(done).resolves.toBeUndefined()
})

it('does nothing in the native app', async () => {
  const { postMessage } = stubWorker(true)
  window.Capacitor = { isNativePlatform: () => true }
  await refreshServiceWorkerCache()
  expect(postMessage).not.toHaveBeenCalled()
})
