import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router'
import { RouterPage } from '@/app/router-pages'
import i18n from '@/app/i18n'
import { addChange } from '@/features/pendingWrites/queue'
import { loadPendingWrites, mutate, resetPendingWritesForTests } from '@/features/pendingWrites/store'
import { resetDbForTests } from '@/lib/db'
import { setToken } from '@/lib/storage'
import { resetTransactionStoreForTests } from '@/lib/transactionStore'
import { IDBFactory } from 'fake-indexeddb'
import { pluralPick } from '@/lib/plural'
import { LogoutEscapeButton } from './LogoutEscapeButton'

const t = (key: string) => i18n.t(key)

beforeEach(() => {
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches: false, media: query, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

function renderEscape(placement?: 'screen' | 'container') {
  render(
    <MemoryRouter initialEntries={['/loading']}>
      <Routes>
        <Route path="/loading" element={<LogoutEscapeButton placement={placement} />} />
        <Route path={RouterPage.LOGOUT} element={<p>signing out</p>} />
      </Routes>
    </MemoryRouter>,
  )
}

const link = () => screen.getByRole('link', { name: t('settings.page.logout') })

async function queueOne() {
  await loadPendingWrites('u1')
  await mutate((ops) => addChange(ops, { kind: 'limit', budgetId: 'b', elementId: 'e', period: '2026-11-01', amount: '1' }, { key: 'k', seq: 1 }))
}

it.each(['screen', 'container'] as const)('with nothing unsent, placed on the %s, it goes straight to sign-out', async (placement) => {
  renderEscape(placement)
  await userEvent.click(link())
  expect(await screen.findByText('signing out')).toBeInTheDocument()
})

it('with unsent changes it says what would be lost before signing out', async () => {
  await queueOne()
  renderEscape()
  await userEvent.click(link())

  expect(screen.queryByText('signing out')).not.toBeInTheDocument()
  const warning = pluralPick(t('settings.logout.pending_warning'), 1, i18n.language)
  expect(await screen.findByText(new RegExp(warning.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')))).toBeInTheDocument()

  await userEvent.click(screen.getByRole('button', { name: t('auth.sign_out.action.logout') }))
  expect(await screen.findByText('signing out')).toBeInTheDocument()
})

it('staying keeps the user on the loader', async () => {
  await queueOne()
  renderEscape()
  await userEvent.click(link())
  expect(screen.queryByRole('link', { name: t('settings.page.logout') })).not.toBeInTheDocument()
  await userEvent.click(await screen.findByRole('button', { name: t('auth.sign_out.action.cancel') }))
  expect(screen.queryByText('signing out')).not.toBeInTheDocument()
  expect(await screen.findByRole('link', { name: t('settings.page.logout') })).toBeInTheDocument()
})

it('can be confirmed with no connection', async () => {
  const { onlineManager } = await import('@tanstack/react-query')
  await queueOne()
  renderEscape()
  onlineManager.setOnline(false)
  onTestFinished(() => onlineManager.setOnline(true))
  await userEvent.click(link())
  expect(await screen.findByRole('button', { name: t('auth.sign_out.action.logout') })).toBeEnabled()
})

describe('before the stored queue has loaded', () => {
  afterEach(() => localStorage.clear())

  const warning = () => pluralPick(t('settings.logout.pending_warning'), 1, i18n.language)
  const escaped = (text: string) => new RegExp(text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'))

  it('a reopened app with changes on disk still asks, and names them once they arrive', async () => {
    await queueOne()
    resetPendingWritesForTests()
    setToken('tok', 'u1')
    renderEscape()
    await userEvent.click(link())

    expect(screen.queryByText('signing out')).not.toBeInTheDocument()
    expect(await screen.findByText(escaped(warning()))).toBeInTheDocument()
    expect(screen.getByRole('button', { name: t('auth.sign_out.action.logout') })).toBeEnabled()
  })

  it('a database that never answers keeps the confirm disabled while waiting, then warns it cannot tell', async () => {
    globalThis.indexedDB = { open: () => ({}) } as unknown as IDBFactory
    resetDbForTests()
    resetTransactionStoreForTests()
    onTestFinished(() => {
      globalThis.indexedDB = new IDBFactory()
      resetDbForTests()
      resetTransactionStoreForTests()
    })
    setToken('tok', 'u1')
    renderEscape()
    await userEvent.click(link())

    expect(screen.queryByText('signing out')).not.toBeInTheDocument()
    expect(await screen.findByRole('button', { name: t('auth.sign_out.action.logout') })).toBeDisabled()
    await vi.waitFor(() => expect(screen.getByRole('button', { name: t('auth.sign_out.action.logout') })).toBeEnabled(), { timeout: 4000 })
    expect(screen.getByText(escaped(t('settings.logout.pending_unknown')))).toBeInTheDocument()
  })

  it('once the queue has loaded empty it goes straight to sign-out', async () => {
    setToken('tok', 'u1')
    await loadPendingWrites('u1')
    renderEscape()
    await userEvent.click(link())
    expect(await screen.findByText('signing out')).toBeInTheDocument()
  })
})
