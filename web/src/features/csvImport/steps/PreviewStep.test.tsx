import { useState } from 'react'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider, onlineManager } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { METRICS, trackEvent } from '@/lib/metrics'
import { cancelAiRefresh } from '@/features/onboarding/finishImport'
import { ImportResultDialog } from '@/features/transactions/ImportResultDialog'
import { ImportWizard, type RetryFailedChunks } from '../ImportWizard'
import type { ImportResult } from '../upload'

vi.mock('@/lib/metrics', async (importOriginal) => ({ ...(await importOriginal<typeof import('@/lib/metrics')>()), trackEvent: vi.fn() }))

const ok = (data: unknown) => HttpResponse.json({ success: true, message: '', data })
const csv = (text: string, name = 'statement.csv') => new File([text], name, { type: 'text/csv' })

const GOOD_DATES = ['2026-01-02', '2026-01-03', '2026-01-04', '2026-01-05', '2026-01-06', '2026-01-07', '2026-01-08', '2026-01-09']
// row 4 of the file (the third transaction) has no readable date, row 7 no amount
const PROBLEMS =
  'Date,Amount,Details\n' +
  '2026-01-02,-5.50,Coffee\n' +
  '2026-01-03,-3.00,Tea\n' +
  'someday,-2.00,Cake\n' +
  '2026-01-04,10.00,Refund\n' +
  '2026-01-05,-1.00,Gum\n' +
  '2026-01-06,,Nothing\n' +
  '2026-01-07,-4.00,Bus\n' +
  '2026-01-08,-6.00,Lunch\n' +
  '2026-01-09,-7.00,Dinner\n'

// The wizard with the result screen behind it, wired as Settings → Data wires them.
function Harness({ file }: { file: File }) {
  const [result, setResult] = useState<ImportResult | null>(null)
  const [retry, setRetry] = useState<RetryFailedChunks | null>(null)
  const [open, setOpen] = useState(true)
  return (
    <>
      <ImportWizard
        open={open}
        initialFile={file}
        onClose={() => setOpen(false)}
        onComplete={(r, again) => {
          setResult(r)
          setRetry(() => again ?? null)
        }}
      />
      <ImportResultDialog
        open={result !== null}
        result={result}
        onClose={() => setResult(null)}
        onRetry={retry ? async () => setResult(await retry()) : undefined}
      />
    </>
  )
}

function renderWizard(text: string) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <Harness file={csv(text)} />
    </QueryClientProvider>,
  )
}

async function openCheck(user: ReturnType<typeof userEvent.setup>) {
  await screen.findByRole('heading', { name: /Which account|Here's what/ })
  await user.click(screen.getByRole('button', { name: /^Check/ }))
  return screen.findByRole('heading', { name: "Here's what will be imported" })
}

const importButton = () => screen.getByRole('button', { name: /^Import \d+ transactions?$/ })

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  server.use(...coreHandlers({ accounts: [] }))
  vi.mocked(trackEvent).mockClear()
})

afterEach(() => {
  cancelAiRefresh()
  onlineManager.setOnline(true)
})

it('the first card shows the amount the way it will be imported, read with a decimal comma', async () => {
  renderWizard('Datum;Betrag;Text\n2026-01-02;-4,50;Kaffee\n2026-01-03;100,00;Lohn\n')
  const user = userEvent.setup()
  await openCheck(user)
  expect(screen.getByText('Transaction 1 of 2')).toBeInTheDocument()
  const amount = screen.getByRole('button', { name: /^−4\.50/ })
  expect(amount).toHaveClass('text-expense')
  expect(screen.getByRole('button', { name: 'Kaffee' })).toBeInTheDocument()
  expect(screen.getByText('2026-01-02;-4,50;Kaffee')).toBeInTheDocument()
  expect(screen.getByText(/^2 transactions, 2026-01-02 to 2026-01-03\. Money in 100\.00 \$, money out 4\.50 \$\.$/)).toBeInTheDocument()
  expect(importButton()).toHaveTextContent('Import 2 transactions')
})

it('Next and the right arrow key move through the rows', async () => {
  renderWizard(PROBLEMS)
  const user = userEvent.setup()
  await openCheck(user)
  await user.click(screen.getByRole('button', { name: 'Next' }))
  expect(screen.getByText('Transaction 2 of 9')).toBeInTheDocument()
  await user.keyboard('{ArrowRight}')
  expect(screen.getByText('Transaction 3 of 9')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Previous' }))
  expect(screen.getByText('Transaction 2 of 9')).toBeInTheDocument()
})

it('a swipe to the left shows the next row', async () => {
  renderWizard(PROBLEMS)
  const user = userEvent.setup()
  await openCheck(user)
  const card = screen.getByRole('button', { name: 'Coffee' }).closest('.touch-pan-y') as HTMLElement
  fireEvent.touchStart(card, { touches: [{ clientX: 200, clientY: 10 }], changedTouches: [{ clientX: 200, clientY: 10 }] })
  fireEvent.touchEnd(card, { touches: [], changedTouches: [{ clientX: 100, clientY: 12 }] })
  expect(screen.getByText('Transaction 2 of 9')).toBeInTheDocument()
})

it('Next problem jumps to the first row that will be skipped, and says why', async () => {
  renderWizard(PROBLEMS)
  const user = userEvent.setup()
  await openCheck(user)
  expect(screen.getByText('2 will be skipped (No readable date: 1, No readable amount: 1)')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Next problem' }))
  expect(screen.getByText('Transaction 3 of 9')).toBeInTheDocument()
  expect(screen.getByText('No readable date')).toBeInTheDocument()
  expect(screen.getByText('someday,-2.00,Cake')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Next problem' }))
  expect(screen.getByText('Transaction 6 of 9')).toBeInTheDocument()
  expect(screen.getByText('No readable amount')).toBeInTheDocument()
})

it('typing a date on a row without one brings it into the import', async () => {
  renderWizard(PROBLEMS)
  const user = userEvent.setup()
  await openCheck(user)
  expect(importButton()).toHaveTextContent('Import 7 transactions')
  await user.click(screen.getByRole('button', { name: 'Next problem' }))
  fireEvent.change(screen.getByLabelText('Date for this row'), { target: { value: '2026-01-03' } })
  expect(importButton()).toHaveTextContent('Import 8 transactions')
  expect(screen.getByRole('button', { name: 'Cake' })).toBeInTheDocument()
})

it('typing an amount on a row without one brings it into the import', async () => {
  renderWizard(PROBLEMS)
  const user = userEvent.setup()
  await openCheck(user)
  await user.click(screen.getByRole('button', { name: 'Next problem' }))
  await user.click(screen.getByRole('button', { name: 'Next problem' }))
  await user.type(screen.getByLabelText('Amount for this row'), '-2,25{Enter}')
  expect(importButton()).toHaveTextContent('Import 8 transactions')
  expect(screen.getByRole('button', { name: /^−2\.25/ })).toBeInTheDocument()
})

it('Skip this row keeps the row out, and Don’t skip offers it again', async () => {
  renderWizard(PROBLEMS)
  const user = userEvent.setup()
  await openCheck(user)
  await user.click(screen.getByRole('button', { name: 'Next problem' }))
  await user.click(screen.getByRole('button', { name: 'Skip this row' }))
  expect(screen.getByText('Skipped by you')).toBeInTheDocument()
  expect(importButton()).toHaveTextContent('Import 7 transactions')
  await user.click(screen.getByRole('button', { name: "Don't skip" }))
  expect(screen.getByLabelText('Date for this row')).toBeInTheDocument()
})

it('Something looks wrong? goes back to the Date step', async () => {
  renderWizard(PROBLEMS)
  const user = userEvent.setup()
  await openCheck(user)
  await user.click(screen.getByRole('button', { name: 'Something looks wrong?' }))
  expect(await screen.findByRole('heading', { name: 'Which column has the date?' })).toBeInTheDocument()
  expect(trackEvent).toHaveBeenCalledWith(METRICS.TRANSACTION_IMPORT_LOOKS_WRONG)
})

it('a value on the card opens its step', async () => {
  renderWizard(PROBLEMS)
  const user = userEvent.setup()
  await openCheck(user)
  await user.click(screen.getByRole('button', { name: /^−5\.50/ }))
  expect(await screen.findByRole('heading', { name: 'How does the file show amounts?' })).toBeInTheDocument()
})

it('importing into a new account creates it once, then imports into it', async () => {
  const created: { id: string; name: string }[] = []
  const posted: { accountId: FormDataEntryValue | null; file: string }[] = []
  server.use(
    http.post('*/api/v1/account/create-account', async ({ request }) => {
      const body = (await request.json()) as { id: string; name: string }
      created.push(body)
      return ok({ item: { id: body.id } })
    }),
    http.post('*/api/v1/transaction/import-transaction-list', async ({ request }) => {
      const form = await request.formData()
      posted.push({ accountId: form.get('accountId'), file: await (form.get('file') as File).text() })
      return ok({ imported: 7, skipped: 0, errors: {} })
    }),
  )
  renderWizard(PROBLEMS)
  const user = userEvent.setup()
  await openCheck(user)
  await user.click(importButton())
  expect(await screen.findByText('Import completed with errors')).toBeInTheDocument()
  expect(created).toHaveLength(1)
  expect(created[0].name).toBe('statement')
  expect(posted).toHaveLength(1)
  expect(posted[0].accountId).toBe(created[0].id)
  expect(posted[0].file.trim().split('\n')).toHaveLength(8)
  expect(screen.getByText('7 transactions imported')).toBeInTheDocument()
  expect(screen.getByText('No readable date')).toBeInTheDocument()
  expect(trackEvent).toHaveBeenCalledWith(METRICS.TRANSACTION_IMPORT, { imported: 7, failed: 2 })
})

it('when the new account cannot be made, nothing is sent and the dialog says why', async () => {
  const posted = vi.fn()
  server.use(
    http.post('*/api/v1/account/create-account', () =>
      HttpResponse.json({ success: false, message: 'Name taken', code: 400, errors: {} }, { status: 400 }),
    ),
    http.post('*/api/v1/transaction/import-transaction-list', () => {
      posted()
      return ok({ imported: 0, skipped: 0, errors: {} })
    }),
  )
  renderWizard(PROBLEMS)
  const user = userEvent.setup()
  await openCheck(user)
  await user.click(importButton())
  expect(await screen.findByRole('alert')).toHaveTextContent('Name taken')
  expect(posted).not.toHaveBeenCalled()
  expect(importButton()).toBeEnabled()
  expect(trackEvent).toHaveBeenCalledWith(METRICS.TRANSACTION_IMPORT_FAIL, { failed: 7 })
})

it('a chunk that failed is offered again, and only that chunk is sent', async () => {
  const lines = Array.from({ length: 600 }, (_, i) => `${GOOD_DATES[i % GOOD_DATES.length]},-${i + 1}.00`)
  const sent: number[] = []
  let calls = 0
  server.use(
    http.post('*/api/v1/account/create-account', async ({ request }) => ok({ item: { id: ((await request.json()) as { id: string }).id } })),
    http.post('*/api/v1/transaction/import-transaction-list', async ({ request }) => {
      calls++
      const rows = (await ((await request.formData()).get('file') as File).text()).trim().split('\n').length - 1
      if (calls === 2) return HttpResponse.json({ success: false, message: 'Server down', code: 0, exceptionType: 'x' }, { status: 500 })
      sent.push(rows)
      return ok({ imported: rows, skipped: 0, errors: {} })
    }),
  )
  renderWizard('Date,Amount\n' + lines.join('\n') + '\n')
  const user = userEvent.setup()
  await openCheck(user)
  await user.click(importButton())
  expect(await screen.findByText('500 transactions imported')).toBeInTheDocument()
  expect(trackEvent).toHaveBeenCalledWith(METRICS.TRANSACTION_IMPORT, { imported: 500, failed: 100 })
  await user.click(screen.getByRole('button', { name: 'Try the rest again' }))
  expect(await screen.findByText('600 transactions imported')).toBeInTheDocument()
  expect(sent).toEqual([500, 100])
  expect(screen.getByText('Import complete')).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Try the rest again' })).not.toBeInTheDocument()
})

it('an AI guess fills a step the user left alone but never one they answered', async () => {
  let release!: () => void
  const gate = new Promise<void>((resolve) => (release = resolve))
  let asked: unknown = null
  server.use(
    http.get('*/api/v1/transaction/suggest-import-columns', async ({ request }) => {
      asked = JSON.parse(new URL(request.url).searchParams.get('columns') ?? 'null')
      await gate
      return ok({ roles: { date: 'Booking date', description: 'Info' } })
    }),
  )
  renderWizard('Booking date,Value date,Amount,Info\n2026-01-02,2026-01-03,-5.50,Coffee\n2026-01-04,2026-01-05,100.00,Salary\n')
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Which account is this statement from?' })
  await user.click(screen.getByRole('button', { name: /^Date/ }))
  await user.click(screen.getByRole('radio', { name: /^Value date/ }))
  release()
  await waitFor(() => expect(trackEvent).toHaveBeenCalledWith(METRICS.TRANSACTION_IMPORT_AI_GUESS))
  expect(screen.getByRole('radio', { name: /^Value date/ })).toBeChecked()
  await user.click(screen.getByRole('button', { name: /^Description/ }))
  expect(screen.getByRole('radio', { name: /^Info/ })).toBeChecked()
  expect(asked).toEqual([
    { name: 'Booking date', type: 'date', negatives: false },
    { name: 'Value date', type: 'date', negatives: false },
    { name: 'Amount', type: 'number', negatives: true },
    { name: 'Info', type: 'text', negatives: false },
  ])
})

it('an Econumo export asks the AI nothing and shows its accounts on the Check step', async () => {
  const asked = vi.fn()
  server.use(http.get('*/api/v1/transaction/suggest-import-columns', () => {
    asked()
    return ok({ roles: {} })
  }))
  renderWizard(
    'transaction_id,account_name,account_currency,category,description,tag,labels,payee,amount,date\n' +
      'a1,Wallet,EUR,Food,Lunch,,,Cafe,-4.5,2026-03-04 14:05:09\n',
  )
  await screen.findByRole('heading', { name: "Here's what will be imported" })
  expect(screen.getByText('Accounts in this file')).toBeInTheDocument()
  expect(screen.getByRole('combobox', { name: 'Currency for Wallet' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: '2026-03-04 14:05:09' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: /^−4\.50/ })).toBeInTheDocument()
  expect(asked).not.toHaveBeenCalled()
})

it('offline, Import is disabled', async () => {
  renderWizard(PROBLEMS)
  const user = userEvent.setup()
  await openCheck(user)
  expect(importButton()).toBeEnabled()
  act(() => onlineManager.setOnline(false))
  expect(importButton()).toBeDisabled()
})

it('without an account answer, Import is disabled', async () => {
  server.use(...coreHandlers())
  renderWizard(PROBLEMS)
  const user = userEvent.setup()
  await openCheck(user)
  expect(importButton()).toBeDisabled()
  expect(within(screen.getByRole('dialog')).getByRole('button', { name: 'No account chosen' })).toBeInTheDocument()
})
