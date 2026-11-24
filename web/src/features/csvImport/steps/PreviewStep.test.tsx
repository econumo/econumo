import { useState } from 'react'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider, onlineManager } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { readMultipart, server } from '@/test/msw'
import { api } from '@/api/client'
import { coreHandlers } from '@/test/fixtures'
import { METRICS, trackEvent } from '@/lib/metrics'
import { cancelAiRefresh } from '@/features/onboarding/finishImport'
import { ImportResultDialog } from '@/features/transactions/ImportResultDialog'
import { ImportWizard, type RetryFailedChunks } from '../ImportWizard'
import type { ImportResult } from '../upload'

vi.mock('@/lib/metrics', async (importOriginal) => ({ ...(await importOriginal<typeof import('@/lib/metrics')>()), trackEvent: vi.fn() }))

const ok = (data: unknown) => HttpResponse.json({ success: true, message: '', data })
const csv = (text: string, name = 'statement.csv') => new File([text], name, { type: 'text/csv' })

// The chunk files as handed to the HTTP client, in send order. Node 26's fetch sends a jsdom File
// as an empty multipart part, so a handler cannot read the rows off the wire.
function captureUploads(): File[] {
  const files: File[] = []
  const post = api.post.bind(api)
  const spy = vi.spyOn(api, 'post').mockImplementation((url: string, body?: unknown, config?: Parameters<typeof api.post>[2]) => {
    if (body instanceof FormData) files.push(body.get('file') as File)
    return post(url, body, config)
  })
  onTestFinished(() => spy.mockRestore())
  return files
}

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
  await screen.findByRole('heading', { name: /Which account|Here is what/ })
  await user.click(screen.getByRole('button', { name: /^Check/ }))
  return screen.findByRole('heading', { name: "Here is what will be added" })
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

const listItems = () => within(screen.getByRole('list', { name: 'Transactions to add' })).getAllByRole('listitem')
const problemRow = (text: string) => listItems().find((li) => li.textContent?.includes(text)) as HTMLElement

it('the list shows each transaction the way it will be imported, read with a decimal comma', async () => {
  renderWizard('Datum;Betrag;Text\n2026-01-02;-4,50;Kaffee\n2026-01-03;100,00;Lohn\n')
  const user = userEvent.setup()
  await openCheck(user)
  const amount = screen.getByRole('button', { name: /^−4\.50/ })
  expect(amount).not.toHaveClass('text-income')
  expect(screen.getByRole('button', { name: /^\+100\.00/ })).toHaveClass('text-income')
  expect(screen.getByRole('button', { name: 'Kaffee' }).closest('li')).toHaveAttribute('title', '2026-01-02;-4,50;Kaffee')
  expect(screen.getByRole('button', { name: 'Jan 2, 2026' })).toHaveTextContent('Jan 2')
  expect(screen.getByText('Transactions').nextSibling).toHaveTextContent('2')
  expect(screen.getByText('Money in').nextSibling).toHaveTextContent('100.00 $')
  expect(screen.getByText('Money out').nextSibling).toHaveTextContent('−4.50 $')
  expect(screen.getByText('Jan 2, 2026 to Jan 3, 2026')).toBeInTheDocument()
  expect(importButton()).toHaveTextContent('Import 2 transactions')
})

it('rows that will be left out sit in the list in file order, with the reason and the line from the file', async () => {
  renderWizard(PROBLEMS)
  const user = userEvent.setup()
  await openCheck(user)
  expect(screen.getByText('2 will be skipped (No readable date: 1, No readable amount: 1)')).toBeInTheDocument()
  const items = listItems()
  expect(items).toHaveLength(9)
  expect(items[2]).toHaveTextContent('Row 4: Cake')
  expect(within(items[2]).getByText('No readable date')).toBeInTheDocument()
  expect(within(items[2]).getByText('someday,-2.00,Cake')).toBeInTheDocument()
  expect(items[5]).toHaveTextContent('Row 7: Nothing')
  expect(within(items[5]).getByText('No readable amount')).toBeInTheDocument()
})

it('a long file shows its first rows and every row that will be left out; the rest on request', async () => {
  const good = Array.from({ length: 150 }, (_, i) => `${GOOD_DATES[i % GOOD_DATES.length]},-1.00,Row${i}`)
  renderWizard('Date,Amount,Details\n' + good.join('\n') + '\nsomeday,-2.00,Late\n')
  const user = userEvent.setup()
  await openCheck(user)
  expect(listItems()).toHaveLength(101)
  expect(problemRow('Late')).toHaveTextContent('No readable date')
  await user.click(screen.getByRole('button', { name: 'Show 50 more rows' }))
  expect(listItems()).toHaveLength(151)
})

it('typing a date on a row without one brings it into the import', async () => {
  renderWizard(PROBLEMS)
  const user = userEvent.setup()
  await openCheck(user)
  expect(importButton()).toHaveTextContent('Import 7 transactions')
  fireEvent.change(screen.getByLabelText('Date for this row'), { target: { value: '2026-01-03' } })
  expect(importButton()).toHaveTextContent('Import 8 transactions')
  expect(screen.getByRole('button', { name: 'Cake' })).toBeInTheDocument()
})

it('typing an amount on a row without one brings it into the import', async () => {
  renderWizard(PROBLEMS)
  const user = userEvent.setup()
  await openCheck(user)
  const input = screen.getByLabelText('Amount for this row')
  await user.type(input, 'abc')
  expect(screen.getByText("Can't read this amount")).toBeInTheDocument()
  await user.type(input, '{Enter}')
  expect(importButton()).toHaveTextContent('Import 7 transactions')
  await user.clear(input)
  // read like the file's amounts: a point for decimals, commas group thousands
  await user.type(input, '-1,234.50{Enter}')
  expect(importButton()).toHaveTextContent('Import 8 transactions')
  expect(screen.getByRole('button', { name: /^−1,234\.50/ })).toBeInTheDocument()
})

it('Skip this row keeps the row out, and Don’t skip offers it again', async () => {
  renderWizard(PROBLEMS)
  const user = userEvent.setup()
  await openCheck(user)
  await user.click(within(problemRow('Cake')).getByRole('button', { name: 'Skip this row' }))
  expect(within(problemRow('Cake')).getByText('Skipped by you')).toBeInTheDocument()
  expect(importButton()).toHaveTextContent('Import 7 transactions')
  await user.click(within(problemRow('Cake')).getByRole('button', { name: "Don't skip" }))
  expect(screen.getByLabelText('Date for this row')).toBeInTheDocument()
})

it('Something looks wrong? goes back to the Date step', async () => {
  renderWizard(PROBLEMS)
  const user = userEvent.setup()
  await openCheck(user)
  await user.click(screen.getByRole('button', { name: 'Something looks wrong?' }))
  expect(await screen.findByRole('heading', { name: 'When did each payment happen?' })).toBeInTheDocument()
  expect(trackEvent).toHaveBeenCalledWith(METRICS.TRANSACTION_IMPORT_LOOKS_WRONG)
})

it('a value in the list opens its step', async () => {
  renderWizard(PROBLEMS)
  const user = userEvent.setup()
  await openCheck(user)
  await user.click(screen.getByRole('button', { name: /^−5\.50/ }))
  expect(await screen.findByRole('heading', { name: 'How much was each payment?' })).toBeInTheDocument()
})

it('importing into a new account creates it once, then imports into it', async () => {
  const created: { id: string; name: string }[] = []
  const posted: { accountId: string | undefined; file: string }[] = []
  const uploads = captureUploads()
  server.use(
    http.post('*/api/v1/account/create-account', async ({ request }) => {
      const body = (await request.json()) as { id: string; name: string }
      created.push(body)
      return ok({ item: { id: body.id } })
    }),
    http.post('*/api/v1/transaction/import-transaction-list', async ({ request }) => {
      const form = await readMultipart(request)
      posted.push({ accountId: form.accountId, file: await uploads[posted.length].text() })
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

it('while the import runs, the steps and their answers are locked', async () => {
  let release!: () => void
  const gate = new Promise<void>((resolve) => (release = resolve))
  server.use(
    http.post('*/api/v1/account/create-account', async ({ request }) => ok({ item: { id: ((await request.json()) as { id: string }).id } })),
    http.post('*/api/v1/transaction/import-transaction-list', async () => {
      await gate
      return ok({ imported: 7, skipped: 0, errors: {} })
    }),
  )
  renderWizard(PROBLEMS)
  const user = userEvent.setup()
  await openCheck(user)
  expect(screen.getByRole('button', { name: 'Close' })).toBeInTheDocument()
  await user.click(importButton())
  await waitFor(() => expect(screen.getByRole('button', { name: /^Date/ })).toBeDisabled())
  expect(screen.getByRole('button', { name: 'Back' })).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Something looks wrong?' })).toBeDisabled()
  expect(screen.getByRole('button', { name: /^−5\.50/ })).toBeDisabled()
  expect(screen.queryByRole('button', { name: 'Close' })).not.toBeInTheDocument()
  await user.keyboard('{Escape}')
  expect(screen.getByRole('heading', { name: "Here is what will be added" })).toBeInTheDocument()
  release()
  expect(await screen.findByText('7 transactions imported')).toBeInTheDocument()
})

it('a chunk that failed is offered again, and only that chunk is sent', async () => {
  const lines = Array.from({ length: 600 }, (_, i) => `${GOOD_DATES[i % GOOD_DATES.length]},-${i + 1}.00`)
  const sent: number[] = []
  let calls = 0
  const uploads = captureUploads()
  server.use(
    http.post('*/api/v1/account/create-account', async ({ request }) => ok({ item: { id: ((await request.json()) as { id: string }).id } })),
    http.post('*/api/v1/transaction/import-transaction-list', async () => {
      calls++
      const rows = (await uploads[calls - 1].text()).trim().split('\n').length - 1
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
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  await user.click(screen.getByRole('button', { name: /^Date/ }))
  // opening the choices answers nothing; picking one does
  await user.click(screen.getByRole('button', { name: 'Change' }))
  await user.click(screen.getByRole('radio', { name: /from “Value date”/ }))
  release()
  await waitFor(() => expect(trackEvent).toHaveBeenCalledWith(METRICS.TRANSACTION_IMPORT_AI_GUESS))
  expect(screen.getByText('from “Value date”')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Description' }))
  expect(screen.getByText('Coffee · Salary')).toBeInTheDocument()
  expect(screen.getByText('from “Info”')).toBeInTheDocument()
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
  await screen.findByRole('heading', { name: "Here is what will be added" })
  expect(screen.getByText('Accounts in this file')).toBeInTheDocument()
  expect(screen.getByRole('combobox', { name: 'Currency for Wallet' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Mar 4, 2026 14:05:09' })).toHaveTextContent('Mar 4')
  expect(screen.getByRole('button', { name: 'Lunch' }).closest('li')).toHaveTextContent('Wallet · Cafe · Food')
  expect(screen.getByRole('button', { name: /^−4\.50/ })).toBeInTheDocument()
  expect(asked).not.toHaveBeenCalled()
})

it('rows going to accounts in different currencies are not added up', async () => {
  renderWizard(
    'transaction_id,account_name,account_currency,category,description,tag,labels,payee,amount,date\n' +
      'a1,Wallet,EUR,Food,Lunch,,,Cafe,-4.5,2026-03-04 14:05:09\n' +
      'a2,Purse,USD,Food,Tea,,,Cafe,-2,2026-03-05 10:00:00\n',
  )
  await screen.findByRole('heading', { name: "Here is what will be added" })
  expect(screen.getByText('Mar 4, 2026 to Mar 5, 2026')).toBeInTheDocument()
  expect(screen.getByText('Transactions').nextSibling).toHaveTextContent('2')
  expect(screen.queryByText(/Money in/)).not.toBeInTheDocument()
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
  expect(within(screen.getByRole('dialog')).getByRole('button', { name: /^Account.*needs your attention/ })).toHaveAccessibleDescription('Not chosen')
})
