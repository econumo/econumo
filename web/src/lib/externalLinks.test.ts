import { afterEach, expect, it, vi } from 'vitest'
import { installExternalLinkInterceptor } from './externalLinks'

afterEach(() => {
  delete (window as { Capacitor?: unknown }).Capacitor
  document.body.innerHTML = ''
})

function clickAnchor(href: string): MouseEvent {
  const a = document.createElement('a')
  a.href = href
  document.body.appendChild(a)
  const event = new MouseEvent('click', { bubbles: true, cancelable: true })
  a.dispatchEvent(event)
  return event
}

it('sends absolute http(s) links to the native browser', () => {
  const open = vi.fn(async () => {})
  window.Capacitor = { isNativePlatform: () => true, Plugins: { Browser: { open } } }
  installExternalLinkInterceptor()
  const event = clickAnchor('https://econumo.com/docs')
  expect(open).toHaveBeenCalledWith({ url: 'https://econumo.com/docs' })
  expect(event.defaultPrevented).toBe(true)
})

it('leaves in-app relative links alone', () => {
  const open = vi.fn(async () => {})
  window.Capacitor = { isNativePlatform: () => true, Plugins: { Browser: { open } } }
  installExternalLinkInterceptor()
  const event = clickAnchor('/settings')
  expect(open).not.toHaveBeenCalled()
  expect(event.defaultPrevented).toBe(false)
})

it('installs nothing on the web', () => {
  installExternalLinkInterceptor()
  // Use a fragment URL which doesn't trigger document navigation in jsdom
  const event = clickAnchor('#external-section')
  expect(event.defaultPrevented).toBe(false)
})
