import { render, screen, waitFor, within } from '@testing-library/react'
import { delay, http, HttpResponse } from 'msw'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { readMultipart, server } from '@/test/msw'
import { coreHandlers, fixtureEur, fixtureUsd } from '@/test/fixtures'
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
  expect(await screen.findByRole('heading', { name: 'Which account are these transactions from?' })).toBeInTheDocument()
  expect(currentStep()).toHaveAccessibleName(/^Account/)
  expect(trackEvent).toHaveBeenCalledWith(METRICS.TRANSACTION_IMPORT_OPEN, { withFile: true })
})

it('the step list is locked until a file is loaded', async () => {
  renderWizard()
  expect(await screen.findByRole('button', { name: /^Amount/ })).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Continue' })).toBeDisabled()
})

it('picking a file in the dialog moves on to the Account step', async () => {
  renderWizard()
  const user = userEvent.setup()
  await user.upload(await screen.findByLabelText('Choose a file'), csv(BANK))
  expect(await screen.findByRole('heading', { name: 'Which account are these transactions from?' })).toBeInTheDocument()
})

it('any step opens from the step list, and Back and Continue walk the steps in order', async () => {
  renderWizard({ initialFile: csv(BANK) })
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  await user.click(screen.getByRole('button', { name: /^Amount/ }))
  expect(screen.getByRole('heading', { name: 'How much was each payment?' })).toBeInTheDocument()
  expect(currentStep()).toHaveAccessibleName(/^Amount/)
  await user.click(screen.getByRole('button', { name: 'Back' }))
  expect(screen.getByRole('heading', { name: 'When did each payment happen?' })).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(screen.getByRole('heading', { name: 'How are the payments sorted?' })).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(screen.getByRole('heading', { name: 'What was each payment for?' })).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(screen.getByRole('heading', { name: 'Who was paid, or who paid you?' })).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(screen.getByRole('heading', { name: 'Here is what will be added' })).toBeInTheDocument()
})

it('lists Category between Amount and Description, and Payee after Description', async () => {
  renderWizard({ initialFile: csv('Booking date,Amount,Details,Trip\n2026-01-02,-5.50,Coffee,Rome\n') })
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  const titles = screen.getAllByRole('button', { name: /^(Account|Date|Amount|Category|Description|Payee|Tags|Check and import)/ })
  const order = ['Account', 'Date', 'Amount', 'Category', 'Description', 'Payee', 'Tags', 'Check and import']
  expect(titles.map((b) => b.textContent)).toEqual(order.map((title) => expect.stringMatching(new RegExp(`^\\d*${title}`))))
})

it('a file with no category or payee answers both calmly; they never need attention', async () => {
  renderWizard({ initialFile: csv(BANK) })
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  expect(screen.getByRole('button', { name: 'Category' })).toHaveAccessibleDescription('None')
  expect(screen.getByRole('button', { name: 'Payee' })).toHaveAccessibleDescription('None')
  await user.click(screen.getByRole('button', { name: 'Category' }))
  expect(screen.getByText('Your file has no categories. You can sort transactions into categories later.')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Continue' })).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Payee' }))
  expect(screen.getByText('Your file doesn’t say who was paid. You can add payees later.')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Continue' })).toBeInTheDocument()
})

it('picking the amount as the category moves it: Amount loses it and needs an answer, Import waits', async () => {
  renderWizard({ initialFile: csv(EXPORT, 'econumo.csv') })
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Here is what will be added' })
  await user.click(screen.getByRole('button', { name: 'Category' }))
  await user.click(screen.getByRole('button', { name: 'Change' }))
  await user.click(screen.getByRole('button', { name: 'Show all' }))
  await user.click(screen.getByRole('radio', { name: /from “amount”/ }))
  expect(screen.getByRole('button', { name: /^Amount.*needs your attention/ })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: /^Category/ })).toHaveAccessibleDescription('-4.5 · 1000')
  await user.click(screen.getByRole('button', { name: /^Check/ }))
  expect(screen.getByRole('button', { name: /^Import/ })).toBeDisabled()
})

it('picking the date as the description moves it: Date loses it and needs an answer', async () => {
  renderWizard({ initialFile: csv(BANK) })
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  await user.click(screen.getByRole('button', { name: 'Description' }))
  await user.click(screen.getByRole('button', { name: 'Change' }))
  await user.click(screen.getByRole('button', { name: 'Show all' }))
  await user.click(screen.getByRole('radio', { name: /from “Booking date”/ }))
  expect(screen.getByRole('button', { name: /^Date.*needs your attention/ })).toBeInTheDocument()
})

it('picking the description as the date moves it: Description is left empty', async () => {
  renderWizard({ initialFile: csv(BANK) })
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  await user.click(screen.getByRole('button', { name: 'Date' }))
  await user.click(screen.getByRole('button', { name: 'Change' }))
  await user.click(screen.getByRole('button', { name: 'Show all' }))
  await user.click(screen.getByRole('radio', { name: /from “Details”/ }))
  expect(screen.getByRole('button', { name: /^Description/ })).toHaveAccessibleDescription('No description')
})

it('Category and Payee left as None never block Import', async () => {
  renderWizard({ initialFile: csv(EXPORT, 'econumo.csv') })
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Here is what will be added' })
  expect(screen.getByRole('button', { name: 'Category' })).toHaveAccessibleDescription('Food · Salary')
  expect(screen.getByRole('button', { name: 'Payee' })).toHaveAccessibleDescription('Cafe')
  for (const step of ['Category', 'Payee']) {
    await user.click(screen.getByRole('button', { name: step }))
    await user.click(screen.getByRole('button', { name: 'Change' }))
    await user.click(screen.getByRole('radio', { name: step === 'Category' ? 'The file has no category' : 'The file has no payee' }))
    expect(screen.getByRole('button', { name: step })).toHaveAccessibleDescription('None')
  }
  await user.click(screen.getByRole('button', { name: /^Check/ }))
  expect(await screen.findByRole('button', { name: 'Import 2 transactions' })).toBeEnabled()
})

it('a guessed step opens on its answer with Continue in the footer; Change puts the choices in its place', async () => {
  renderWizard({ initialFile: csv(BANK) })
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  await user.click(screen.getByRole('button', { name: 'Date' }))
  expect(screen.getByText('Jan 2, 2026 · Jan 3, 2026')).toBeInTheDocument()
  expect(screen.getByText('from “Booking date”')).toBeInTheDocument()
  expect(screen.queryByRole('radio')).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Continue' })).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Change' }))
  expect(trackEvent).toHaveBeenCalledWith(METRICS.TRANSACTION_IMPORT_CHANGE_ANSWER, { step: 'date' })
  expect(screen.getByRole('radiogroup', { name: 'Which of these is the date?' })).toBeInTheDocument()
  expect(screen.getByRole('radio', { name: /from “Booking date”/ })).toBeChecked()
  // the answer row gives way to the list
  expect(screen.queryByText('from “Booking date”', { selector: 'div' })).not.toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Change' })).not.toBeInTheDocument()
  // the list stays open after leaving the step and coming back
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Back' }))
  expect(screen.getByRole('radiogroup', { name: 'Which of these is the date?' })).toBeInTheDocument()
})

it('a step with nothing guessed opens on its question, and picking an answer shows it', async () => {
  renderWizard({ initialFile: csv('Day,Amount,Details\n2026-01-02,-5.50,Coffee\n2026-01-03,1.00,x\n') })
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  await user.click(screen.getByRole('button', { name: /^Date/ }))
  expect(screen.getByRole('radiogroup', { name: 'Which of these is the date?' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Continue' })).toBeInTheDocument()
  await user.click(screen.getByRole('radio', { name: /from “Day”/ }))
  expect(screen.queryByRole('radiogroup', { name: 'Which of these is the date?' })).not.toBeInTheDocument()
  expect(screen.getByText('from “Day”')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Continue' })).toBeInTheDocument()
})

const marker = (name: RegExp) => screen.getByRole('button', { name }).querySelector('span.rounded-full')

it('a guessed step keeps its number until it is seen and left; a missing answer keeps its mark; Import is not held back', async () => {
  renderWizard({ initialFile: csv('Booking date,Amount,Details\n2026-01-02,-5.50,Coffee\n2026-01-03,100.00,Salary\n') })
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  expect(marker(/^Date/)).toHaveTextContent('2')
  expect(marker(/^Date/)?.querySelector('svg')).toBeNull()
  expect(screen.getByRole('button', { name: /^Date/ })).toHaveAccessibleDescription(/^Jan 2, 2026/)
  expect(screen.getByRole('button', { name: /^Account.*needs your attention/ })).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: /^Date/ }))
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(marker(/^Date/)?.querySelector('svg.lucide-check')).not.toBeNull()
  expect(marker(/^Description/)).toHaveTextContent(/\d/)
  // left the account step without an answer: the mark stays
  expect(screen.getByRole('button', { name: /^Account.*needs your attention/ })).toBeInTheDocument()
})

it('steps never opened stay numbered and do not hold back Import', async () => {
  renderWizard({ initialFile: csv(EXPORT, 'econumo.csv') })
  await screen.findByRole('heading', { name: 'Here is what will be added' })
  for (const name of [/^Account/, /^Date/, /^Amount/]) expect(marker(name)?.querySelector('svg')).toBeNull()
  expect(await screen.findByRole('button', { name: 'Import 2 transactions' })).toBeEnabled()
})

it('a step that opened on its question shows the answer once a guess arrives', async () => {
  let reply: () => void = () => {}
  const replied = new Promise<void>((resolve) => (reply = resolve))
  server.use(
    http.get('*/api/v1/transaction/suggest-import-columns', async () => {
      await replied
      return HttpResponse.json({ success: true, message: '', data: { roles: { payee: 'Who' } } })
    }),
  )
  renderWizard({ initialFile: csv('Booking date,Amount,Details,Who\n2026-01-02,-5.50,Coffee,Cafe Roma\n2026-01-03,100.00,Salary,ACME Ltd\n') })
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  await user.click(screen.getByRole('button', { name: /^Payee/ }))
  expect(screen.getByRole('radiogroup', { name: 'Which of these says who was paid?' })).toBeInTheDocument()
  reply()
  await waitFor(() => expect(screen.queryByRole('radiogroup', { name: 'Which of these says who was paid?' })).not.toBeInTheDocument())
  expect(screen.getByText('from “Who”')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Change' })).toBeInTheDocument()
})

it('money in and money out keep the list open after the first pick and fold it after the second', async () => {
  renderWizard({ initialFile: csv('Booking date,Foo,Bar,Details\n2026-01-02,,5.50,Coffee\n2026-01-03,100.00,,Salary\n') })
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  await user.click(screen.getByRole('button', { name: /^Amount/ }))
  expect(screen.getByRole('button', { name: 'Continue' })).toBeInTheDocument()
  await user.click(screen.getByRole('radio', { name: 'Money in and money out' }))
  const moneyIn = () => screen.getByRole('radiogroup', { name: 'Which of these is money in?' })
  const moneyOut = () => screen.getByRole('radiogroup', { name: 'Which of these is money out?' })
  await user.click(within(moneyIn()).getByRole('radio', { name: /from “Foo”/ }))
  expect(moneyOut()).toBeInTheDocument()
  await user.click(within(moneyOut()).getByRole('radio', { name: /from “Bar”/ }))
  expect(screen.queryByRole('radiogroup', { name: 'Which of these is money out?' })).not.toBeInTheDocument()
  expect(screen.getByText('from “Foo” · from “Bar”')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Continue' })).toBeInTheDocument()
})

it('Tags is left out when every column is already used', async () => {
  renderWizard({ initialFile: csv(BANK) })
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  expect(screen.queryByRole('button', { name: /^Tags/ })).not.toBeInTheDocument()
})

it('Tags is offered when the file has a column no step uses', async () => {
  renderWizard({ initialFile: csv('Booking date,Amount,Details,Merchant,Trip\n2026-01-02,-5.50,Coffee,Cafe,Rome\n') })
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  expect(screen.getByRole('button', { name: /^Tags/ })).toBeInTheDocument()
})

it('Tags is left out when Category and Payee read the only columns left', async () => {
  renderWizard({ initialFile: csv('Booking date,Amount,Details,Merchant\n2026-01-02,-5.50,Coffee,Cafe\n') })
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  expect(screen.getByRole('button', { name: 'Payee' })).toHaveAccessibleDescription('Cafe')
  expect(screen.queryByRole('button', { name: /^Tags/ })).not.toBeInTheDocument()
})

it('marks an unanswered account and a date column that leaves rows unread', async () => {
  renderWizard({ initialFile: csv('Booking date,Amount,Details\n2026-01-02,-5.50,Coffee\nnot a day,1.00,x\n') })
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  expect(screen.getByRole('button', { name: /^Account.*needs your attention/ })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: /^Date.*needs your attention/ })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Amount' })).toBeInTheDocument()
})

it('with no date chosen yet, Amount still answers with the amounts the file reads', async () => {
  renderWizard({ initialFile: csv('Day,Amount,Details\n2026-01-02,-5.50,Coffee\n2026-01-03,1.00,x\n') })
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  expect(screen.getByRole('button', { name: 'Amount' })).toHaveAccessibleDescription('−5.50 · +1.00')
  await user.click(screen.getByRole('button', { name: 'Amount' }))
  const items = screen.getAllByRole('listitem')
  expect(items.map((item) => item.textContent)).toEqual(expect.arrayContaining([expect.stringMatching(/^−5\.5/), expect.stringMatching(/^\+1/)]))
  expect(screen.getByRole('button', { name: 'Continue' })).toBeInTheDocument()
})

it('choosing a different date column on its step clears the warning', async () => {
  renderWizard({ initialFile: csv('Day,Amount,Details\n2026-01-02,-5.50,Coffee\n2026-01-03,1.00,x\n') })
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  expect(screen.getByRole('button', { name: /^Date.*needs your attention/ })).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: /^Date/ }))
  await user.click(screen.getByRole('radio', { name: /from “Day”/ }))
  expect(screen.getByRole('button', { name: 'Date' })).toBeInTheDocument()
})

it('rows that will be skipped never mark a step; the step says so quietly', async () => {
  const days = Array.from({ length: 9 }, (_, i) => `2026-01-0${i + 1},-1.00,x`).join('\n')
  renderWizard({ initialFile: csv(`Date,Amount,Details\n${days}\nsoon,1.00,y\n`) })
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  expect(screen.getByRole('button', { name: 'Date' })).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Date' }))
  expect(screen.getByText('1 row has no readable date and will be skipped')).toBeInTheDocument()
})

it('each step shows its answer in plain words under its name', async () => {
  renderWizard({ initialFile: csv(BANK) })
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  expect(screen.getByRole('button', { name: 'Date' })).toHaveAccessibleDescription('Jan 2, 2026')
  expect(screen.getByRole('button', { name: 'Amount' })).toHaveAccessibleDescription('−5.50 · +100.00')
  expect(screen.getByRole('button', { name: 'Description' })).toHaveAccessibleDescription('Coffee')
  expect(screen.getByRole('button', { name: /^Check/ })).toHaveAccessibleDescription('2 to import · 0 left out')
  expect(screen.getByText('2 transactions ready · 0 left out')).toBeInTheDocument()
})

it('a column given on the Payee step carries a “used for payee” tag on Description', async () => {
  renderWizard({ initialFile: csv('Booking date,Amount,Details,Note\n2026-01-02,-5.50,Coffee and a long walk home,Cafe\n') })
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  await user.click(screen.getByRole('button', { name: /^Payee/ }))
  await user.click(screen.getByRole('radio', { name: 'Cafe from “Note”' }))
  expect(screen.getByRole('button', { name: 'Payee' })).toHaveAccessibleDescription('Cafe')
  await user.click(screen.getByRole('button', { name: 'Description' }))
  await user.click(screen.getByRole('button', { name: 'Change' }))
  // used elsewhere, so not offered until Show all
  expect(screen.queryByRole('radio', { name: 'Cafe from “Note”' })).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Show all' }))
  expect(screen.getByRole('radio', { name: 'Cafe from “Note”' })).toHaveAccessibleDescription('used for payee')
  expect(screen.getByRole('radio', { name: /from “Details”/ })).toBeChecked()
})

it('an Econumo export goes straight to Check', async () => {
  renderWizard({ initialFile: csv(EXPORT, 'econumo.csv') })
  expect(await screen.findByRole('heading', { name: "Here is what will be added" })).toBeInTheDocument()
  expect(currentStep()).toHaveAccessibleName(/^Check/)
})

it('an Econumo export’s Account step answers with the accounts the file names', async () => {
  renderWizard({ initialFile: csv(EXPORT, 'econumo.csv') })
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Here is what will be added' })
  expect(screen.getByRole('button', { name: 'Account' })).toHaveAccessibleDescription('From Cash and 1 more account')
  await user.click(screen.getByRole('button', { name: 'Account' }))
  const list = screen.getByRole('list', { name: 'Accounts in your file' })
  expect(list).toHaveAccessibleName('Accounts in your file')
  expect(list).toHaveTextContent('Cash · 1 transaction')
  expect(list).toHaveTextContent('Bank · 1 transaction')
  expect(screen.queryByRole('radio', { name: /from “account_name”/ })).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Continue' })).toBeInTheDocument()
})

it('closing on the Date step reports the step it was closed on', async () => {
  const onClose = vi.fn()
  renderWizard({ initialFile: csv(BANK), onClose })
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
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

it('on a phone the step list comes first; a step opens over it and the arrow goes back', async () => {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: q === '(max-width: 639px)', media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  renderWizard({ initialFile: csv(BANK) })
  const user = userEvent.setup()
  expect(await screen.findByText('We read your file and filled in what we could. Tap a step to change it.')).toBeInTheDocument()
  expect(screen.queryByRole('heading', { name: 'Which account are these transactions from?' })).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Amount' }))
  expect(screen.getByRole('heading', { name: 'How much was each payment?' })).toBeInTheDocument()
  expect(screen.getByText('Step 3 of 7 · Amount')).toBeInTheDocument()
  expect(screen.queryByRole('navigation')).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'All steps' }))
  await user.click(screen.getByRole('button', { name: 'Check 2 transactions' }))
  expect(screen.getByRole('heading', { name: 'Here is what will be added' })).toBeInTheDocument()
})

it('on a phone, before a file is read, the dialog asks for one', async () => {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: q === '(max-width: 639px)', media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  renderWizard()
  expect(await screen.findByRole('heading', { name: 'Choose a file from your bank or another app' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Close' })).toBeInTheDocument()
})

const toCheckWithNewAccount = async (user: ReturnType<typeof userEvent.setup>) => {
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
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

it('an Econumo export makes each new account in the currency its rows name', async () => {
  const made: Record<string, unknown>[] = []
  const cad = { ...fixtureUsd, id: 'cur-cad', code: 'CAD', name: 'Canadian Dollar', symbol: 'C$' }
  server.use(
    ...coreHandlers({ currencies: [fixtureUsd, fixtureEur, cad] }),
    http.post('*/api/v1/account/create-account', async ({ request }) => {
      const body = (await request.json()) as Record<string, unknown>
      made.push(body)
      return HttpResponse.json({ success: true, message: '', data: { item: { id: `made-${made.length}`, folderId: 'f1' }, transaction: null } })
    }),
    http.post('*/api/v1/transaction/import-transaction-list', () =>
      HttpResponse.json({ success: true, message: '', data: { imported: 2, skipped: 0, errors: {} } }),
    ),
  )
  renderWizard({
    initialFile: csv(
      'transaction_id,account_name,account_currency,category,description,tag,labels,payee,amount,date\n' +
        'a1,WS Cash,CAD,Interest,,,,WealthSimple,-4.5,2026-11-08 14:05:09\n' +
        'a2,Cash,USD,Strata,,,,ICBC,100,2026-11-07 09:00:00\n',
    ),
  })
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Import 2 transactions' }))
  await waitFor(() => expect(made).toHaveLength(1))
  // "Cash" is the user's own account; only WS Cash is made, in CAD
  expect(made[0]).toMatchObject({ name: 'WS Cash', currencyId: 'cur-cad' })
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

// a new file or a reopened dialog forgets the accounts this one made, so the lists must know them
it('a failed account create reads the accounts and folders again', async () => {
  const reads = { accounts: 0, folders: 0 }
  const count = ({ request }: { request: Request }) => {
    if (request.url.includes('/get-account-list')) reads.accounts++
    if (request.url.includes('/get-folder-list')) reads.folders++
  }
  server.events.on('request:start', count)
  onTestFinished(() => server.events.removeListener('request:start', count))
  server.use(
    http.post('*/api/v1/account/create-account', () =>
      HttpResponse.json({ success: false, message: 'Nope', code: 400, errors: {} }, { status: 400 }),
    ),
  )
  renderWizard({ initialFile: csv(BANK) })
  const user = userEvent.setup()
  await toCheckWithNewAccount(user)
  const before = { ...reads }
  await user.click(await screen.findByRole('button', { name: 'Import 2 transactions' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('Nope')
  expect(reads.accounts).toBeGreaterThan(before.accounts)
  expect(reads.folders).toBeGreaterThan(before.folders)
})
