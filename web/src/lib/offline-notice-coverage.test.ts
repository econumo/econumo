import { describe, expect, it } from 'vitest'

// Every routed page whose writes all need the server says so offline with
// OfflineNotice, rendered by the page itself or by SettingsShell (which shows it
// unless the page passes editableOffline). The pages below show none, each for
// the reason given.
const NO_NOTICE: Record<string, string> = {
  HomePage: 'transactions can be added, edited and deleted offline',
  AccountPage: 'transactions can be added, edited and deleted offline',
  BudgetPage: 'Budget and Plan: limits can be changed offline',
  InboxPage: 'Try again on a failed write works offline (editableOffline)',
  ReportsPage: 'read-only even online',
  AppLockPage: 'local device setting, works offline',
  LoginPage: 'signed out',
  RegistrationPage: 'signed out',
  LogoutPage: 'signed out',
  OAuthCallbackPage: 'signed out, a provider redirect',
  NotFoundPage: 'no content',
}

// markers that put the notice on the page
const SHOWS_NOTICE = ['<OfflineNotice', '<SettingsShell', '<ClassificationList']

const sources = import.meta.glob('/src/**/*.tsx', { eager: true, query: '?raw', import: 'default' }) as Record<string, string>

function routedPages(): Map<string, string> {
  const routes = sources['/src/app/routes.tsx']
  const imports = new Map<string, string>()
  for (const m of routes.matchAll(/import \{ (\w+) \} from '@\/([^']+)'/g)) {
    imports.set(m[1], `/src/${m[2]}.tsx`)
  }
  const pages = new Map<string, string>()
  for (const m of routes.matchAll(/element: <(\w+)/g)) {
    const file = imports.get(m[1])
    if (file && !m[1].endsWith('Layout') && m[1] !== 'RequireAuth') {
      pages.set(m[1], file)
    }
  }
  return pages
}

describe('offline notice coverage', () => {
  const pages = routedPages()

  it('finds the routed pages', () => {
    expect(pages.size).toBeGreaterThan(30)
  })

  it('every routed page shows the offline notice or is listed with a reason', () => {
    const missing = [...pages]
      .filter(([name]) => !(name in NO_NOTICE))
      .filter(([, file]) => !SHOWS_NOTICE.some((marker) => sources[file]?.includes(marker)))
      .map(([name]) => name)
    expect(missing).toEqual([])
  })

  it('pages that stay editable offline do not show it', () => {
    for (const [name, file] of pages) {
      if (!(name in NO_NOTICE)) continue
      const content = sources[file] ?? ''
      expect(content, name).not.toContain('<OfflineNotice')
      if (content.includes('<SettingsShell')) {
        expect(content, `${name} uses SettingsShell, so it must pass editableOffline`).toContain('editableOffline')
      }
    }
  })

  it('every listed exemption is still a routed page', () => {
    for (const name of Object.keys(NO_NOTICE)) {
      expect(pages.has(name), name).toBe(true)
    }
  })

  it('pages under SettingsShell opt out only through editableOffline', () => {
    const optedOut = [...pages].filter(([, file]) => /editableOffline/.test(sources[file] ?? '')).map(([name]) => name)
    expect(optedOut.every((name) => name in NO_NOTICE)).toBe(true)
  })
})
