import { render, screen, waitFor, within } from '@testing-library/react'
import { delay, http, HttpResponse } from 'msw'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { readMultipart, server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { METRICS, trackEvent } from '@/lib/metrics'
import { ImportWizard } from './ImportWizard'

vi.mock('@/lib/metrics', async (importOriginal) => ({ ...(await importOriginal<typeof import('@/lib/metrics')>()), trackEvent: vi.fn() }))

const BANK = 'Booking date,Amount,Details\n2026-01-02,-5.50,Coffee\n2026-01-03,100.00,Salary\n'
const EXPORT =
  'transaction_id,account_name,account_currency,category,description,tag,labels,payee,amount,date\n' +
  'a1,Cash,USD,Food,Lunch,,,Cafe,-4.5,2026-03-04 14:05:09\n' +
  'a2,Bank,EUR,Salary,,,,,1000,2026-03-05 09:00:00\n'

const csv = (text: string, name = 'statement.csv') => new File([text], name, { type: 'text/csv' })

function renderWizard(props: { initialFile?: File; onClose?: () => void } = {}) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <ImportWizard open onClose={props.onClose ?? (() => {})} onComplete={vi.fn()} initialFile={props.initialFile} />
    </QueryClientProvider>,
  )
}

const currentStep = () => screen.getByRole('navigation').querySelector('[aria-current="step"]')

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  server.use(...coreHandlers())
  vi.mocked(trackEvent).mockClear()
})

it('a bank statement given on open lands on the Account step', async () => {
  renderWizard({ initialFile: csv(BANK) })
  expect(await screen.findByRole('heading', { name: 'Which account is this statement from?' })).toBeInTheDocument()
  expect(currentStep()).toHaveAccessibleName(/^Account/)
  expect(trackEvent).toHaveBeenCalledWith(METRICS.TRANSACTION_IMPORT_OPEN, { withFile: true })
})

it('the step bar is locked until a file is loaded', async () => {
  renderWizard()
  expect(await screen.findByRole('button', { name: /^Amount/ })).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Next' })).toBeDisabled()
})

it('picking a file in the dialog moves on to the Account step', async () => {
  renderWizard()
  const user = userEvent.setup()
  await user.upload(await screen.findByLabelText('Choose a file'), csv(BANK))
  expect(await screen.findByRole('heading', { name: 'Which account is this statement from?' })).toBeInTheDocument()
})

it('any step opens from the step bar, and Back and Next walk the steps in order', async () => {
  renderWizard({ initialFile: csv(BANK) })
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Which account is this statement from?' })
  await user.click(screen.getByRole('button', { name: /^Amount/ }))
  expect(screen.getByRole('heading', { name: 'How does the file show amounts?' })).toBeInTheDocument()
  expect(currentStep()).toHaveAccessibleName(/^Amount/)
  await user.click(screen.getByRole('button', { name: 'Back' }))
  expect(screen.getByRole('heading', { name: 'Tap the column with the date' })).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Next' }))
  await user.click(screen.getByRole('button', { name: 'Next' }))
  expect(screen.getByRole('heading', { name: 'Tap the column that describes each transaction' })).toBeInTheDocument()
})

it('Extra details is left out when every column is already used', async () => {
  renderWizard({ initialFile: csv(BANK) })
  await screen.findByRole('heading', { name: 'Which account is this statement from?' })
  expect(screen.queryByRole('button', { name: /^Extra details/ })).not.toBeInTheDocument()
})

it('Extra details is offered when the file has a column no step uses', async () => {
  renderWizard({ initialFile: csv('Booking date,Amount,Details,Merchant\n2026-01-02,-5.50,Coffee,Cafe\n') })
  await screen.findByRole('heading', { name: 'Which account is this statement from?' })
  expect(screen.getByRole('button', { name: /^Extra details/ })).toBeInTheDocument()
})

it('marks an unanswered account and a date column that leaves rows unread', async () => {
  renderWizard({ initialFile: csv('Booking date,Amount,Details\n2026-01-02,-5.50,Coffee\nnot a day,1.00,x\n') })
  await screen.findByRole('heading', { name: 'Which account is this statement from?' })
  expect(screen.getByRole('button', { name: /^Account.*needs your attention/ })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: /^Date.*needs your attention/ })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Amount' })).toBeInTheDocument()
})

it('choosing a different date column on its step clears the warning', async () => {
  renderWizard({ initialFile: csv('Day,Amount,Details\n2026-01-02,-5.50,Coffee\n2026-01-03,1.00,x\n') })
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Which account is this statement from?' })
  expect(screen.getByRole('button', { name: /^Date.*needs your attention/ })).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: /^Date/ }))
  await user.click(screen.getByRole('radio', { name: /^Day/ }))
  expect(screen.getByRole('button', { name: 'Date' })).toBeInTheDocument()
})

it('rows that will be skipped never mark a step; the step says so quietly', async () => {
  const days = Array.from({ length: 9 }, (_, i) => `2026-01-0${i + 1},-1.00,x`).join('\n')
  renderWizard({ initialFile: csv(`Date,Amount,Details\n${days}\nsoon,1.00,y\n`) })
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Which account is this statement from?' })
  expect(screen.getByRole('button', { name: 'Date' })).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Date' }))
  expect(screen.getByText('1 row has no readable date and will be skipped')).toBeInTheDocument()
})

it('a column given in Extra details is a strip on Description that opens Extra details', async () => {
  renderWizard({ initialFile: csv('Booking date,Amount,Details,Merchant\n2026-01-02,-5.50,Coffee,Cafe\n') })
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Which account is this statement from?' })
  await user.click(screen.getByRole('button', { name: 'Extra details' }))
  await user.click(screen.getByRole('radio', { name: 'Merchant' }))
  await user.click(within(screen.getByRole('radiogroup', { name: 'Use “Merchant” as' })).getByRole('radio', { name: 'Payee' }))
  await user.click(screen.getByRole('button', { name: 'Description' }))
  await user.click(await screen.findByRole('button', { name: 'Payee — tap to change' }))
  expect(screen.getByRole('heading', { name: 'Anything else to bring over?' })).toBeInTheDocument()
})

it('an Econumo export goes straight to Check', async () => {
  renderWizard({ initialFile: csv(EXPORT, 'econumo.csv') })
  expect(await screen.findByRole('heading', { name: "Here's what will be imported" })).toBeInTheDocument()
  expect(currentStep()).toHaveAccessibleName(/^Check/)
})

it('closing on the Date step reports the step it was closed on', async () => {
  const onClose = vi.fn()
  renderWizard({ initialFile: csv(BANK), onClose })
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Which account is this statement from?' })
  await user.click(screen.getByRole('button', { name: /^Date/ }))
  await user.keyboard('{Escape}')
  expect(trackEvent).toHaveBeenCalledWith(METRICS.TRANSACTION_IMPORT_CLOSE, { step: 'date' })
  expect(onClose).toHaveBeenCalled()
})

it('a file over 10 MB says so and stays on Your file', async () => {
  renderWizard()
  const user = userEvent.setup()
  await user.upload(await screen.findByLabelText('Choose a file'), new File([new Uint8Array(10485761)], 'big.csv', { type: 'text/csv' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('This file is larger than 10 MB')
  expect(currentStep()).toHaveAccessibleName(/^Your file/)
  expect(screen.getByRole('button', { name: /^Account/ })).toBeDisabled()
})

it('an empty file says so', async () => {
  renderWizard()
  const user = userEvent.setup()
  await user.upload(await screen.findByLabelText('Choose a file'), csv('Date,Amount\n'))
  expect(await screen.findByRole('alert')).toHaveTextContent('This file is empty')
})

it('on a phone the bar shows numbered steps and the current step name', async () => {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: q === '(max-width: 639px)', media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  renderWizard({ initialFile: csv(BANK) })
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Which account is this statement from?' })
  const nav = screen.getByRole('navigation')
  const shownName = () => nav.querySelector('p')
  expect(shownName()).toHaveTextContent('Account')
  expect(within(nav).getByRole('button', { name: /^Amount/ })).toHaveTextContent('4')
  await user.click(within(nav).getByRole('button', { name: /^Amount/ }))
  await waitFor(() => expect(shownName()).toHaveTextContent('Amount'))
})

const toCheckWithNewAccount = async (user: ReturnType<typeof userEvent.setup>) => {
  await screen.findByRole('heading', { name: 'Which account is this statement from?' })
  await user.click(screen.getByRole('radio', { name: 'A new account' }))
  await user.click(screen.getByRole('button', { name: /^Check/ }))
}

// the server invents a folder only for a user who has none and refuses a blank one otherwise
it('a new account goes into the user’s first folder and the rows into the account the server made', async () => {
  let created: Record<string, unknown> | null = null
  let accountId: string | undefined
  server.use(
    http.post('*/api/v1/account/create-account', async ({ request }) => {
      created = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ success: true, message: '', data: { item: { id: 'made-by-server', folderId: 'f1' }, transaction: null } })
    }),
    http.post('*/api/v1/transaction/import-transaction-list', async ({ request }) => {
      accountId = (await readMultipart(request)).accountId
      return HttpResponse.json({ success: true, message: '', data: { imported: 2, skipped: 0, errors: {} } })
    }),
  )
  renderWizard({ initialFile: csv(BANK) })
  const user = userEvent.setup()
  await toCheckWithNewAccount(user)
  await user.click(await screen.findByRole('button', { name: 'Import 2 transactions' }))
  await waitFor(() => expect(accountId).toBe('made-by-server'))
  expect(created).toMatchObject({ name: 'statement', folderId: 'f1' })
  expect(created!.id).not.toBe('made-by-server')
})

it('a new account never goes into a hidden folder', async () => {
  let created: Record<string, unknown> | null = null
  server.use(
    http.get('*/api/v1/account/get-folder-list', () =>
      HttpResponse.json({
        success: true,
        message: '',
        data: { items: [{ id: 'f-hidden', name: 'Hidden', position: 0, isVisible: 0 }, { id: 'f2', name: 'Savings', position: 1, isVisible: 1 }] },
      }),
    ),
    http.post('*/api/v1/account/create-account', async ({ request }) => {
      created = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ success: true, message: '', data: { item: { id: 'made-by-server', folderId: 'f2' }, transaction: null } })
    }),
    http.post('*/api/v1/transaction/import-transaction-list', () =>
      HttpResponse.json({ success: true, message: '', data: { imported: 2, skipped: 0, errors: {} } }),
    ),
  )
  renderWizard({ initialFile: csv(BANK) })
  const user = userEvent.setup()
  await toCheckWithNewAccount(user)
  await user.click(await screen.findByRole('button', { name: 'Import 2 transactions' }))
  await waitFor(() => expect(created).toMatchObject({ folderId: 'f2' }))
})

it('Import waits until the user’s folders are known', async () => {
  server.use(http.get('*/api/v1/account/get-folder-list', async () => {
    await delay('infinite')
    return HttpResponse.json({})
  }))
  renderWizard({ initialFile: csv(BANK) })
  const user = userEvent.setup()
  await toCheckWithNewAccount(user)
  expect(await screen.findByRole('button', { name: 'Import 2 transactions' })).toBeDisabled()
})
