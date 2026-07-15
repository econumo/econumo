import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser, fixtureUsd, fixtureEur } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import { CurrenciesPage } from './CurrenciesPage'

const fixturePts = { id: 'cur-pts', code: 'PTS', name: 'Points', symbol: 'pt', fractionDigits: 0, scope: 'own', isArchived: 0, isHidden: 0 }
const fixtureGbp = { id: 'cur-gbp', code: 'GBP', name: 'Pound', symbol: '£', fractionDigits: 2, scope: 'global', isArchived: 0, isHidden: 1 }

const defaultRates = [
  { currencyId: 'cur-usd', baseCurrencyId: 'cur-usd', rate: '1', updatedAt: '2026-07-01 00:00:00' },
  { currencyId: 'cur-eur', baseCurrencyId: 'cur-usd', rate: '0.9', updatedAt: '2026-07-01 00:00:00' },
  { currencyId: 'cur-pts', baseCurrencyId: 'cur-usd', rate: '3.5', updatedAt: '2026-07-01 00:00:00' },
]

function mockViewport() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter([{ path: '/x', element: <CurrenciesPage /> }], { initialEntries: ['/x'] })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return queryClient
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  server.use(
    ...coreHandlers({
      currencies: [fixtureUsd, fixtureEur, fixturePts, fixtureGbp],
      rates: defaultRates,
    }),
  )
  mockViewport()
})

it('renders My currencies and Global currencies sections; own customs show name+code+rate label', async () => {
  renderPage()
  expect(await screen.findByText('My currencies')).toBeInTheDocument()
  expect(screen.getByText('Global currencies')).toBeInTheDocument()
  expect(await screen.findByText('Points')).toBeInTheDocument()
  expect(screen.getByText('PTS · pt')).toBeInTheDocument()
  expect(screen.getByText('1 USD = 3.5 PTS')).toBeInTheDocument()
})

it('create flow: uuidv7 id + uppercased code posted, list invalidated', async () => {
  let body: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/currency/create-currency', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({
        success: true, message: '',
        data: { item: { id: body.id, code: body.code, name: body.name, symbol: 'pt', fractionDigits: 0, scope: 'own', isArchived: 0, isHidden: 0 } },
      })
    }),
  )
  const user = userEvent.setup()
  const queryClient = renderPage()
  const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
  await screen.findByText('Points')
  await user.click(screen.getByRole('button', { name: /Create currency/ }))
  await user.type(await screen.findByLabelText('Code'), 'pts2')
  await user.type(screen.getByLabelText('Name'), 'Second Points')
  await user.type(screen.getByLabelText('Exchange rate'), '3.5')
  await user.click(screen.getByRole('button', { name: 'Create' }))
  await waitFor(() => expect(body).toBeDefined())
  expect(body!.id).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i)
  expect(body!.code).toBe('PTS')
  expect(body!.name).toBe('Second Points')
  expect(body!.rate).toBe('3.5')
  await waitFor(() => expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: queryKeys.currencies }))
})

it('archive toggle on an own custom posts archive-currency', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/currency/archive-currency', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await screen.findByText('Points')
  await user.click(screen.getByRole('switch', { name: 'archive Points' }))
  await waitFor(() => expect(body).toEqual({ id: 'cur-pts' }))
})

it('delete flow surfaces server refusal text', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/currency/delete-currency', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json(
        { success: false, message: 'Currency is in use and cannot be deleted', code: 400, errors: {} },
        { status: 400 },
      )
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await screen.findByText('Points')
  await user.click(screen.getByRole('button', { name: 'actions Points' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Delete' }))
  await screen.findByText('Delete currency?')
  await user.click(screen.getByRole('button', { name: 'Delete' }))
  await waitFor(() => expect(body).toEqual({ id: 'cur-pts' }))
  expect(await screen.findByText('Currency is in use and cannot be deleted')).toBeInTheDocument()
})

it('hide/show switch on globals posts hide-currency / show-currency', async () => {
  let hideBody: unknown
  let showBody: unknown
  server.use(
    http.post('*/api/v1/currency/hide-currency', async ({ request }) => {
      hideBody = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
    http.post('*/api/v1/currency/show-currency', async ({ request }) => {
      showBody = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await screen.findByText('Euro')
  // EUR is visible (isHidden 0), not base, not profile currency -> togglable, hides it
  await user.click(screen.getByRole('switch', { name: 'show Euro' }))
  await waitFor(() => expect(hideBody).toEqual({ id: 'cur-eur' }))
  // GBP starts hidden -> toggling shows it
  await user.click(screen.getByRole('switch', { name: 'show Pound' }))
  await waitFor(() => expect(showBody).toEqual({ id: 'cur-gbp' }))
})

it("base currency row's visibility switch is disabled", async () => {
  renderPage()
  await screen.findByText('US Dollar')
  const sw = screen.getByRole('switch', { name: 'show US Dollar' })
  expect(sw).toBeDisabled()
  expect(sw).toHaveAttribute('title', 'The base currency is always visible')
})

it("profile currency row's visibility switch is disabled", async () => {
  server.use(
    ...coreHandlers({
      currencies: [fixtureUsd, fixtureEur, fixturePts, fixtureGbp],
      rates: defaultRates,
      user: { ...fixtureUser, options: fixtureUser.options.map((o) => (o.name === 'currency_id' ? { ...o, value: 'cur-eur' } : o)) },
    }),
  )
  renderPage()
  await screen.findByText('Euro')
  const sw = screen.getByRole('switch', { name: 'show Euro' })
  expect(sw).toBeDisabled()
  expect(sw).toHaveAttribute('title', 'Your profile currency is always visible')
})

it('set-rate dialog surfaces a server 400 inline and stays open', async () => {
  server.use(
    http.post('*/api/v1/currency/set-currency-rate', () =>
      HttpResponse.json(
        { success: false, message: 'Rate must be a positive number', code: 400, errors: {} },
        { status: 400 },
      ),
    ),
  )
  const user = userEvent.setup()
  renderPage()
  await screen.findByText('Points')
  await user.click(screen.getByRole('button', { name: 'actions Points' }))
  const menu = await screen.findByRole('menu')
  await user.click(within(menu).getByRole('menuitem', { name: 'Set exchange rate' }))
  await screen.findByText('Set exchange rate')
  await user.type(screen.getByLabelText('Exchange rate'), '-1')
  await user.click(screen.getByRole('button', { name: 'Save rate' }))
  expect(await screen.findByText('Rate must be a positive number')).toBeInTheDocument()
  expect(screen.getByText('Set exchange rate')).toBeInTheDocument()
})

it('set-rate dialog posts {currencyId, rate, date?}', async () => {
  let body: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/currency/set-currency-rate', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await screen.findByText('Points')
  await user.click(screen.getByRole('button', { name: 'actions Points' }))
  const menu = await screen.findByRole('menu')
  await user.click(within(menu).getByRole('menuitem', { name: 'Set exchange rate' }))
  await screen.findByText('Set exchange rate')
  await user.type(screen.getByLabelText('Exchange rate'), '4.2')
  await user.click(screen.getByRole('button', { name: 'Save rate' }))
  await waitFor(() => expect(body).toBeDefined())
  expect(body).toEqual({ currencyId: 'cur-pts', rate: '4.2' })
})
