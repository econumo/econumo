import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { server } from '@/test/msw'
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
  expect(screen.getByRole('heading', { name: 'Which column has the date?' })).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Next' }))
  await user.click(screen.getByRole('button', { name: 'Next' }))
  expect(screen.getByRole('heading', { name: 'Which column describes each transaction?' })).toBeInTheDocument()
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
