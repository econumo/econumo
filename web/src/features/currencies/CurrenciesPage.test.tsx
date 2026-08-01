import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser, fixtureUsd, fixtureEur } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import { CurrenciesPage } from './CurrenciesPage'

const fixturePts = { id: 'cur-pts', code: 'PTS', name: 'Points', symbol: 'pt', fractionDigits: 0, scope: 'own', isHidden: 0 }
const fixtureGbp = { id: 'cur-gbp', code: 'GBP', name: 'Pound', symbol: '£', fractionDigits: 2, scope: 'global', isHidden: 1 }

const defaultRates = [
  { currencyId: 'cur-usd', baseCurrencyId: 'cur-usd', rate: '1', updatedAt: '2026-07-01 00:00:00' },
  { currencyId: 'cur-eur', baseCurrencyId: 'cur-usd', rate: '0.9', updatedAt: '2026-07-01 00:00:00' },
  { currencyId: 'cur-pts', baseCurrencyId: 'cur-usd', rate: '3.5', updatedAt: '2026-07-01 00:00:00' },
]

function mockViewport(compact = false) {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: q.includes('max-width') ? compact : false,
    media: q,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
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
        data: { item: { id: body.id, code: body.code, name: body.name, symbol: 'pt', fractionDigits: 0, scope: 'own', isHidden: 0 } },
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

it('own custom rows carry the same visibility switch: posts hide-currency / show-currency', async () => {
  let hideBody: unknown
  let showBody: unknown
  server.use(
    ...coreHandlers({
      currencies: [fixtureUsd, fixtureEur, fixturePts, { ...fixturePts, id: 'cur-old', code: 'OLD', name: 'Old points', isHidden: 1 }, fixtureGbp],
      rates: defaultRates,
    }),
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
  await screen.findByText('Points')
  await user.click(screen.getByRole('switch', { name: 'show Points' }))
  await waitFor(() => expect(hideBody).toEqual({ id: 'cur-pts' }))
  await user.click(screen.getByRole('switch', { name: 'show Old points' }))
  await waitFor(() => expect(showBody).toEqual({ id: 'cur-old' }))
})

it("an own custom that is the profile currency has its switch disabled", async () => {
  server.use(
    ...coreHandlers({
      currencies: [fixtureUsd, fixtureEur, fixturePts, fixtureGbp],
      rates: defaultRates,
      user: { ...fixtureUser, options: fixtureUser.options.map((o) => (o.name === 'currency_id' ? { ...o, value: 'cur-pts' } : o)) },
    }),
  )
  renderPage()
  await screen.findByText('Points')
  const sw = screen.getByRole('switch', { name: 'show Points' })
  expect(sw).toBeDisabled()
  expect(sw).toHaveAttribute('title', 'Your profile currency is always visible')
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

it('edit dialog prefills the fixed rate and posts it with update-currency', async () => {
  let body: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/currency/update-currency', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  const queryClient = renderPage()
  const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
  await screen.findByText('Points')
  await user.click(screen.getByRole('button', { name: 'actions Points' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit' }))
  const rateInput = await screen.findByLabelText('Exchange rate')
  // prefilled from the rates query (cur-pts fixed rate 3.5)
  expect(rateInput).toHaveValue('3.5')
  await user.clear(rateInput)
  await user.type(rateInput, '20')
  await user.click(screen.getByRole('button', { name: 'Update' }))
  await waitFor(() => expect(body).toBeDefined())
  expect(body).toEqual({ id: 'cur-pts', name: 'Points', symbol: 'pt', fractionDigits: 0, rate: '20' })
  // the fixed rate rides update-currency: BOTH the list and the rates view refresh
  await waitFor(() => expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: queryKeys.currencyRates }))
})

it('own rows get a kebab with Edit / Set rate / Delete; global rows get none', async () => {
  const user = userEvent.setup()
  renderPage()
  await screen.findByText('Points')
  expect(screen.getByRole('button', { name: 'actions Points' })).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'actions Pound' })).toBeNull()
  expect(screen.queryByRole('button', { name: 'actions US Dollar' })).toBeNull()
  await user.click(screen.getByRole('button', { name: 'actions Points' }))
  const menu = await screen.findByRole('menu')
  expect(within(menu).getByRole('menuitem', { name: 'Edit' })).toBeInTheDocument()
  expect(within(menu).queryByRole('menuitem', { name: 'Set exchange rate' })).toBeNull()
  expect(within(menu).getByRole('menuitem', { name: 'Delete' })).toBeInTheDocument()
})

it('compact: tapping an own row opens the action sheet; global rows do not react', async () => {
  mockViewport(true)
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByText('Points'))
  const sheet = await screen.findByRole('dialog')
  expect(within(sheet).getByRole('button', { name: 'Edit' })).toBeInTheDocument()
  expect(within(sheet).queryByRole('button', { name: 'Set exchange rate' })).toBeNull()
  expect(within(sheet).getByRole('button', { name: 'Delete' })).toBeInTheDocument()
  await user.keyboard('{Escape}')
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  await user.click(screen.getByText('Pound'))
  expect(screen.queryByRole('dialog')).toBeNull()
})

it('no active-only filter: hidden currencies are always listed, switch off', async () => {
  server.use(
    ...coreHandlers({
      currencies: [fixtureUsd, fixtureEur, { ...fixturePts, isHidden: 1 }, fixtureGbp],
      rates: defaultRates,
    }),
  )
  renderPage()
  expect(await screen.findByText('Points')).toBeInTheDocument()
  expect(screen.getByRole('switch', { name: 'show Points' })).not.toBeChecked()
  expect(screen.getByText('Pound')).toBeInTheDocument()
  expect(screen.queryByRole('switch', { name: 'Active only' })).toBeNull()
})

it('the rate is required: an empty rate blocks submit with a validation message', async () => {
  let called = false
  server.use(
    http.post('*/api/v1/currency/update-currency', () => {
      called = true
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await screen.findByText('Points')
  await user.click(screen.getByRole('button', { name: 'actions Points' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit' }))
  const rateInput = await screen.findByLabelText('Exchange rate')
  await user.clear(rateInput)
  await user.click(screen.getByRole('button', { name: 'Update' }))
  expect(await screen.findAllByText('Required field')).not.toHaveLength(0)
  expect(called).toBe(false)
})
