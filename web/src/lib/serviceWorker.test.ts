import { refreshServiceWorkerCache, rememberRunningConfig } from './serviceWorker'

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

describe('rememberRunningConfig', () => {
  const put = vi.fn(async (_key: string, _response: Response) => {})
  const open = vi.fn(async (_name: string) => ({ put }))

  beforeEach(() => {
    put.mockClear()
    open.mockClear()
    vi.stubGlobal('caches', { open })
    stubWorker(true)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    Reflect.deleteProperty(window, 'econumoConfig')
  })

  it('stores the config this page booted with, as a script that restores it', async () => {
    window.econumoConfig = { VERSION: 'dev', VERSION_REF: 'abc' }
    await rememberRunningConfig()
    expect(open).toHaveBeenCalledWith('econumo-config')
    const [key, response] = put.mock.calls[0]
    expect(key).toBe('/econumo-config.js')
    expect(response.headers.get('content-type')).toContain('javascript')
    const restored: { econumoConfig?: unknown } = {}
    new Function('window', await response.text())(restored)
    expect(restored.econumoConfig).toEqual({ VERSION: 'dev', VERSION_REF: 'abc' })
  })

  it('stores nothing when the page got no config', async () => {
    Reflect.deleteProperty(window, 'econumoConfig')
    await rememberRunningConfig()
    expect(put).not.toHaveBeenCalled()
  })

  it('stores nothing in the native app', async () => {
    window.econumoConfig = { VERSION: 'dev' }
    window.Capacitor = { isNativePlatform: () => true }
    await rememberRunningConfig()
    expect(put).not.toHaveBeenCalled()
  })

  it('a failure to store it stays quiet', async () => {
    window.econumoConfig = { VERSION: 'dev' }
    open.mockRejectedValueOnce(new Error('quota'))
    await expect(rememberRunningConfig()).resolves.toBeUndefined()
  })
})
