import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureAccounts, fixtureLabels, fixtureUsd } from '@/test/fixtures'
import { api } from '@/api/client'
import { ImportCsvDialog } from './ImportCsvDialog'

const CSV = 'Account,Date,Amount,Category,Description\nCash,2026-01-02,-5.5,Food,coffee\nBank,2026-01-03,100,Salary,pay\n'

function renderDialog(onComplete = vi.fn()) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <ImportCsvDialog open onClose={() => {}} onComplete={onComplete} />
    </QueryClientProvider>,
  )
  return { queryClient, onComplete }
}

async function uploadCsv(text = CSV, name = 'import.csv') {
  const user = userEvent.setup()
  const input = await screen.findByLabelText('CSV File')
  await user.upload(input, new File([text], name, { type: 'text/csv' }))
  return user
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  server.use(...coreHandlers())
})

afterEach(() => {
  // a leftover api.post mock from spyOnImportPost would otherwise silently
  // survive into the next test and make its pass depend on run order
  vi.restoreAllMocks()
})

it('parsing a file reveals the mapping UI with auto-detected columns and samples', async () => {
  renderDialog()
  expect(screen.getByText('Maximum file size: 10 MB')).toBeInTheDocument()
  await uploadCsv()
  expect(await screen.findByText(/Map the columns from your CSV file/)).toBeInTheDocument()
  expect(screen.getByText('import.csv')).toBeInTheDocument()

  const accountSelect = screen.getByLabelText('Account') as HTMLSelectElement
  expect(accountSelect.value).toBe('Account')
  const dateSelect = screen.getByLabelText('Date') as HTMLSelectElement
  expect(dateSelect.value).toBe('Date')
  const amountSelect = screen.getByLabelText('Amount') as HTMLSelectElement
  expect(amountSelect.value).toBe('Amount')
  // sample values decorate the options
  expect(screen.getAllByRole('option', { name: 'Account ("Cash")' }).length).toBeGreaterThan(0)
  expect(screen.getByRole('button', { name: 'Import' })).toBeEnabled()
})

it('rejects files above 10 MB', async () => {
  renderDialog()
  const user = userEvent.setup()
  const input = await screen.findByLabelText('CSV File')
  const big = new File([new Uint8Array(10485761)], 'big.csv', { type: 'text/csv' })
  await user.upload(input, big)
  expect(await screen.findByText('Maximum file size: 10 MB')).toBeInTheDocument()
  expect(screen.queryByText(/Map the columns/)).not.toBeInTheDocument()
})

it('switching account to an existing account shows writable accounts only', async () => {
  renderDialog()
  const user = await uploadCsv()
  await user.click(screen.getByRole('button', { name: 'toggle Account mode' }))
  const select = (await screen.findByLabelText('Account')) as HTMLSelectElement
  const labels = [...select.options].map((o) => o.textContent)
  expect(labels.some((l) => l?.startsWith('Cash ('))).toBe(true)
  // Import requires an account pick now
  expect(screen.getByRole('button', { name: 'Import' })).toBeDisabled()
  await user.selectOptions(select, 'a1')
  expect(screen.getByRole('button', { name: 'Import' })).toBeEnabled()
})

it('dual amount mode requires both inflow and outflow columns', async () => {
  renderDialog()
  const user = await uploadCsv('Account,Date,In,Out\nCash,2026-01-02,,5\n')
  await waitFor(() => expect((screen.getByLabelText('Amount (Inflow)') as HTMLSelectElement).value).toBe('In'))
  expect((screen.getByLabelText('Amount (Outflow)') as HTMLSelectElement).value).toBe('Out')
  expect(screen.getByRole('button', { name: 'Import' })).toBeEnabled()
  await user.selectOptions(screen.getByLabelText('Amount (Inflow)'), '')
  expect(screen.getByRole('button', { name: 'Import' })).toBeDisabled()
})

it('happy path: import posts once, invalidates queries, and reports the result', async () => {
  let posts = 0
  server.use(
    http.post('*/api/v1/transaction/import-transaction-list', () => {
      posts += 1
      return HttpResponse.json({ success: true, message: '', data: { imported: 2, skipped: 0, errors: {} } })
    }),
  )
  const onComplete = vi.fn()
  const { queryClient } = renderDialog(onComplete)
  const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
  const user = await uploadCsv()
  await screen.findByText(/Map the columns/)
  await user.click(screen.getByRole('button', { name: 'Import' }))
  await waitFor(() => expect(onComplete).toHaveBeenCalledWith({ imported: 2, failed: 0, errors: [] }))
  expect(posts).toBe(1)
  expect(invalidateSpy).toHaveBeenCalled()
})

// jsdom multipart bodies don't survive the MSW interceptor (no Blob.stream),
// so inspect the FormData at the axios boundary instead of over the wire —
// same technique as src/api/transaction.test.ts's importTransactionList test.
// This still exercises the real FormData serialization (mapping JSON, every
// form field) that buildImportPayload's output goes through on its way to
// the network, unlike mocking the API function itself.
function spyOnImportPost(imported = 1) {
  const postSpy = vi
    .spyOn(api, 'post')
    .mockResolvedValue({ data: { success: true, message: '', data: { imported, skipped: 0, errors: {} } } })
  const body = () => postSpy.mock.calls[0]?.[1] as FormData | undefined
  return {
    mapping: () => {
      const raw = body()?.get('mapping')
      return raw ? (JSON.parse(raw as string) as Record<string, string | null>) : null
    },
    fields: () => {
      const form = body()
      if (!form) return {}
      const out: Record<string, string> = {}
      for (const [key, value] of form.entries()) {
        if (key === 'file' || key === 'mapping') continue
        out[key] = value as string
      }
      return out
    },
  }
}

it('unmapped labels column defaults labelsSeparator to ";" and sends mapping.labels as null', async () => {
  const captured = spyOnImportPost()
  renderDialog()
  const user = await uploadCsv()
  await screen.findByText(/Map the columns/)
  await user.click(screen.getByRole('button', { name: 'Import' }))
  await waitFor(() => expect(captured.mapping()).not.toBeNull())
  expect(captured.mapping()!.labels).toBeNull()
  expect(captured.fields().labelsSeparator).toBe(';')
})

it('mapping the labels column previews the new-label count and sends the chosen separator', async () => {
  const captured = spyOnImportPost()
  renderDialog()
  const user = await uploadCsv('Account,Date,Amount,Labels\nCash,2026-01-02,-5.5,Kid A;Kid B\nBank,2026-01-03,100,Kid A\n')
  await screen.findByText(/Map the columns/)

  const labelsColumnSelect = screen.getByRole('combobox', { name: /reporting tags/i }) as HTMLSelectElement
  await user.selectOptions(labelsColumnSelect, '')
  // unmapped -> countNewLabels short-circuits to 0 -> no preview
  expect(screen.queryByRole('status')).not.toBeInTheDocument()

  await user.selectOptions(labelsColumnSelect, 'Labels')
  // "Kid A" and "Kid B" are both new (the fixture owner only has "health") ->
  // countNewLabels returns 2, which is what flips the preview on
  await screen.findByRole('status')

  const separatorButton = screen.getByRole('button', { name: /separator/i })
  await user.click(separatorButton)
  await user.click(screen.getByRole('button', { name: '|' }))
  await user.click(screen.getByRole('button', { name: /save/i }))

  await user.click(screen.getByRole('button', { name: 'Import' }))
  await waitFor(() => expect(captured.mapping()).not.toBeNull())
  expect(captured.mapping()!.labels).toBe('Labels')
  expect(captured.fields().labelsSeparator).toBe('|')
})

it('a custom separator overrides the presets', async () => {
  const captured = spyOnImportPost()
  renderDialog()
  const user = await uploadCsv('Account,Date,Amount,Labels\nCash,2026-01-02,-5.5,Kid A#Kid B\n')
  await screen.findByText(/Map the columns/)
  await user.selectOptions(screen.getByRole('combobox', { name: /reporting tags/i }), 'Labels')
  await user.click(screen.getByRole('button', { name: /separator/i }))
  await user.type(screen.getByRole('textbox', { name: /custom/i }), '#')
  await user.click(screen.getByRole('button', { name: /save/i }))
  await user.click(screen.getByRole('button', { name: 'Import' }))
  await waitFor(() => expect(captured.fields().labelsSeparator).toBe('#'))
})

it('existing-mode label picker is owner-scoped, excludes archived labels, and sends labelIds comma-joined', async () => {
  const archivedLabel = {
    id: 'label-old', ownerUserId: 'u1', name: 'retired', icon: 'sell', position: 1, isArchived: 1,
    createdAt: '2026-01-01 00:00:00', updatedAt: '2026-01-01 00:00:00',
  }
  server.use(...coreHandlers({ labels: [...fixtureLabels, archivedLabel] }))

  const captured = spyOnImportPost()
  renderDialog()
  const user = await uploadCsv()
  await screen.findByText(/Map the columns/)

  await user.click(screen.getByRole('button', { name: /toggle.*reporting tags.*mode/i }))
  // "health" (u1, unarchived) is offered; "retired" (u1, archived) is not
  const chip = await screen.findByRole('checkbox', { name: 'health' })
  expect(screen.queryByRole('checkbox', { name: 'retired' })).not.toBeInTheDocument()
  expect(chip).toHaveAttribute('aria-checked', 'false')
  await user.click(chip)
  expect(chip).toHaveAttribute('aria-checked', 'true')

  await user.click(screen.getByRole('button', { name: 'Import' }))
  await waitFor(() => expect(captured.fields().labelIds).toBe('label1'))
})

it('switching the target account owner clears a fixed label pick, so it never leaks to the new owner (Vue parity with category/payee/tag)', async () => {
  const otherOwner = { id: 'u2', avatar: 'pets:sky', name: 'Partner' }
  const sharedAccount = {
    id: 'a-shared', owner: otherOwner, folderId: null, name: 'Shared', position: 4,
    currency: fixtureUsd, balance: '50', type: 1 as const, icon: 'group',
    sharedAccess: [{ user: { id: 'u1', avatar: 'face:emerald', name: 'Ada' }, role: 'admin' as const, isAccepted: 1 as const }],
  }
  const otherLabel = { id: 'label2', ownerUserId: 'u2', name: 'shared-label', icon: 'sell', position: 1, isArchived: 0, createdAt: '2026-01-01 00:00:00', updatedAt: '2026-01-01 00:00:00' }
  server.use(...coreHandlers({ accounts: [...fixtureAccounts, sharedAccount], labels: [...fixtureLabels, otherLabel] }))

  const captured = spyOnImportPost()
  renderDialog()
  const user = await uploadCsv()
  await screen.findByText(/Map the columns/)

  await user.click(screen.getByRole('button', { name: /toggle.*reporting tags.*mode/i }))
  const healthChip = await screen.findByRole('checkbox', { name: 'health' })
  await user.click(healthChip)
  expect(healthChip).toHaveAttribute('aria-checked', 'true')

  await user.click(screen.getByRole('button', { name: 'toggle Account mode' }))
  await user.selectOptions(screen.getByLabelText('Account'), 'a-shared')

  // the picker now only offers the new owner's labels, and the stale pick
  // from the previous owner is gone rather than silently carried over
  await waitFor(() => expect(screen.queryByRole('checkbox', { name: 'health' })).not.toBeInTheDocument())
  expect(await screen.findByRole('checkbox', { name: 'shared-label' })).toHaveAttribute('aria-checked', 'false')

  await user.click(screen.getByRole('button', { name: 'Import' }))
  // without the clearing effect, buildImportPayload would still send the
  // previous owner's "label1" here even though it is invisible to u2
  await waitFor(() => expect(captured.mapping()).not.toBeNull())
  expect(captured.fields().labelIds).toBeUndefined()
})

it('the new-tag count skips names an existing BUDGET tag already holds', async () => {
  renderDialog()
  // "vacation" is a budget tag (fixtureTags), "health" a reporting tag
  // (fixtureLabels); both kinds share one namespace, so only "fresh" is new
  const user = await uploadCsv('Account,Date,Amount,Labels\nCash,2026-01-02,-5.5,vacation;health;fresh\n')
  await screen.findByText(/Map the columns/)
  await user.selectOptions(screen.getByRole('combobox', { name: /reporting tags/i }), 'Labels')
  expect(await screen.findByRole('status')).toHaveTextContent('This import will create 1 new tag')
})
