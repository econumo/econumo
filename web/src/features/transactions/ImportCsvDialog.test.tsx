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
  const { unmount } = render(
    <QueryClientProvider client={queryClient}>
      <ImportCsvDialog open onClose={() => {}} onComplete={onComplete} />
    </QueryClientProvider>,
  )
  return { queryClient, onComplete, unmount }
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


// the column pickers are the app's searchable picker; '—' is its clear row
async function pickColumn(user: ReturnType<typeof userEvent.setup>, field: string | RegExp, option: string | RegExp) {
  await user.click(screen.getByRole('combobox', { name: field }))
  await user.click(await screen.findByRole('option', { name: option }))
}

it('parsing a file reveals the mapping UI with auto-detected columns and samples', async () => {
  renderDialog()
  expect(screen.getByText('Maximum file size: 10 MB')).toBeInTheDocument()
  await uploadCsv()
  expect(await screen.findByText(/Map the columns from your CSV file/)).toBeInTheDocument()
  expect(screen.getByText('import.csv')).toBeInTheDocument()

  // auto-detected columns, each read with a sample value from the file
  expect(screen.getByRole('combobox', { name: 'Account' })).toHaveValue('Account ("Cash")')
  expect((screen.getByRole('combobox', { name: 'Date' }) as HTMLInputElement).value).toMatch(/^Date/)
  expect((screen.getByRole('combobox', { name: 'Amount' }) as HTMLInputElement).value).toMatch(/^Amount/)
  expect(screen.getByRole('button', { name: 'Import' })).toBeEnabled()
})

it('parses an initial file without a pick in the dialog', async () => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <ImportCsvDialog open onClose={() => {}} onComplete={vi.fn()} initialFile={new File([CSV], 'dropped.csv', { type: 'text/csv' })} />
    </QueryClientProvider>,
  )
  expect(await screen.findByText(/Map the columns from your CSV file/)).toBeInTheDocument()
  expect(screen.getByText('dropped.csv')).toBeInTheDocument()
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
  // Import requires an account pick now
  expect(screen.getByRole('button', { name: 'Import' })).toBeDisabled()
  await user.click(await screen.findByRole('combobox', { name: 'Account' }))
  await user.click(await screen.findByRole('option', { name: /Cash \(/ }))
  expect(screen.getByRole('button', { name: 'Import' })).toBeEnabled()
})

it('dual amount mode requires both inflow and outflow columns', async () => {
  renderDialog()
  const user = await uploadCsv('Account,Date,In,Out\nCash,2026-01-02,,5\n')
  await waitFor(() => expect(screen.getByRole('combobox', { name: 'Amount (Inflow)' })).toHaveValue('In'))
  expect(screen.getByRole('combobox', { name: 'Amount (Outflow)' })).toHaveValue('Out ("5")')
  expect(screen.getByRole('button', { name: 'Import' })).toBeEnabled()
  await pickColumn(user, 'Amount (Inflow)', '—')
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

  // not auto-detected, so unmapped -> countNewLabels short-circuits to 0 -> no preview
  expect(screen.getByRole('combobox', { name: /reporting tags/i })).toHaveValue('')
  expect(screen.queryByRole('status')).not.toBeInTheDocument()

  await pickColumn(user, /reporting tags/i, /^Labels/)
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
  await pickColumn(user, /reporting tags/i, /^Labels/)
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
  await user.click(screen.getByRole('combobox', { name: 'Account' }))
  await user.click(await screen.findByRole('option', { name: /Shared \(/ }))

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
  await pickColumn(user, /reporting tags/i, /^Labels/)
  expect(await screen.findByRole('status')).toHaveTextContent('This import will create 1 new tag')
})

function snapshotHandlers(items: { accountId: string; balance: string; openingCorrectionId: string; fresh: boolean }[], log: string[] = []) {
  server.use(
    http.get('*/api/v1/account/get-balance-snapshot', () => {
      log.push('snapshot')
      return HttpResponse.json({ success: true, message: '', data: { takenAt: '2026-11-07 10:00:00', items } })
    }),
    http.post('*/api/v1/transaction/import-transaction-list', () => {
      log.push('import')
      return HttpResponse.json({ success: true, message: '', data: { imported: 2, skipped: 0, errors: {} } })
    }),
    http.post('*/api/v1/account/restore-balance', async ({ request }) => {
      log.push(`restore:${JSON.stringify(await request.json())}`)
      return HttpResponse.json({ success: true, message: '', data: { item: fixtureAccounts[0], restored: true } })
    }),
  )
  return log
}

it('keep balance box is ticked for a fresh account and unticked for a used one', async () => {
  snapshotHandlers([{ accountId: 'a1', balance: '100.5', openingCorrectionId: 'c1', fresh: true }, { accountId: 'a2', balance: '2000', openingCorrectionId: '', fresh: true }])
  const first = renderDialog()
  await uploadCsv()
  expect(await screen.findByRole('checkbox', { name: /Keep current balance/ })).toBeChecked()
  first.unmount()

  snapshotHandlers([{ accountId: 'a1', balance: '100.5', openingCorrectionId: 'c1', fresh: true }, { accountId: 'a2', balance: '2000', openingCorrectionId: '', fresh: false }])
  renderDialog()
  await uploadCsv()
  expect(await screen.findByRole('checkbox', { name: /Keep current balance/ })).not.toBeChecked()
})

it('ticked submit runs snapshot, import, restore and reports the kept balances', async () => {
  const log = snapshotHandlers([{ accountId: 'a1', balance: '100.5', openingCorrectionId: 'c1', fresh: true }, { accountId: 'a2', balance: '2000', openingCorrectionId: '', fresh: true }])
  const { onComplete } = renderDialog()
  const user = await uploadCsv()
  await screen.findByRole('checkbox', { name: /Keep current balance/ })
  await user.click(screen.getByRole('button', { name: 'Import' }))
  await waitFor(() => expect(onComplete).toHaveBeenCalled())
  // preview snapshot, fresh snapshot, import, restores (a trailing refetch from the invalidation may follow)
  expect(log.slice(0, 5)).toEqual([
    'snapshot',
    'snapshot',
    'import',
    'restore:{"accountId":"a1","balance":"100.5","openingCorrectionId":"c1","since":"2026-11-07 10:00:00"}',
    'restore:{"accountId":"a2","balance":"2000","openingCorrectionId":"","since":"2026-11-07 10:00:00"}',
  ])
  expect(onComplete.mock.calls[0][0].kept).toHaveLength(2)
})

it('unticked submit never restores', async () => {
  const log = snapshotHandlers([{ accountId: 'a1', balance: '100.5', openingCorrectionId: 'c1', fresh: false }])
  const { onComplete } = renderDialog()
  const user = await uploadCsv()
  await screen.findByRole('checkbox', { name: /Keep current balance/ })
  const before = log.length
  await user.click(screen.getByRole('button', { name: 'Import' }))
  await waitFor(() => expect(onComplete).toHaveBeenCalledWith({ imported: 2, failed: 0, errors: [] }))
  expect(log.slice(before).filter((l) => l !== 'snapshot')).toEqual(['import'])
})

it('keep balance box is hidden when no account is mapped or the snapshot has no items', async () => {
  snapshotHandlers([])
  const first = renderDialog()
  await uploadCsv()
  await screen.findByText(/Map the columns/)
  await waitFor(() => expect(screen.getByRole('button', { name: 'Import' })).toBeEnabled())
  expect(screen.queryByRole('checkbox', { name: /Keep current balance/ })).not.toBeInTheDocument()
  first.unmount()

  const log = snapshotHandlers([{ accountId: 'a1', balance: '1', openingCorrectionId: '', fresh: true }])
  renderDialog()
  const user = await uploadCsv()
  await screen.findByRole('checkbox', { name: /Keep current balance/ })
  await pickColumn(user, 'Account', '—')
  await waitFor(() => expect(screen.queryByRole('checkbox', { name: /Keep current balance/ })).not.toBeInTheDocument())
  expect(log.length).toBeGreaterThan(0)
})

it('keeps Import disabled while the balance snapshot is loading', async () => {
  let release: () => void = () => {}
  const gate = new Promise<void>((resolve) => { release = resolve })
  snapshotHandlers([{ accountId: 'a1', balance: '1', openingCorrectionId: '', fresh: true }])
  server.use(
    http.get('*/api/v1/account/get-balance-snapshot', async () => {
      await gate
      return HttpResponse.json({ success: true, message: '', data: { takenAt: '2026-11-07 10:00:00', items: [{ accountId: 'a1', balance: '1', openingCorrectionId: '', fresh: true }] } })
    }),
  )
  renderDialog()
  await uploadCsv()
  await screen.findByText(/Map the columns/)
  expect(screen.getByRole('button', { name: 'Import' })).toBeDisabled()
  release()
  expect(await screen.findByRole('checkbox', { name: /Keep current balance/ })).toBeChecked()
  expect(screen.getByRole('button', { name: 'Import' })).toBeEnabled()
})

it('keeps the box on screen while a changed target reloads the snapshot', async () => {
  const user = await (async () => {
    snapshotHandlers([{ accountId: 'a1', balance: '1', openingCorrectionId: '', fresh: true }])
    renderDialog()
    return uploadCsv()
  })()
  await screen.findByRole('checkbox', { name: /Keep current balance/ })
  let release: () => void = () => {}
  const gate = new Promise<void>((resolve) => { release = resolve })
  server.use(
    http.get('*/api/v1/account/get-balance-snapshot', async () => {
      await gate
      return HttpResponse.json({ success: true, message: '', data: { takenAt: '2026-11-07 10:00:00', items: [{ accountId: 'a1', balance: '1', openingCorrectionId: '', fresh: true }] } })
    }),
  )
  await user.click(screen.getByRole('button', { name: 'toggle Account mode' }))
  await user.click(screen.getByRole('combobox', { name: 'Account' }))
  await user.click(await screen.findByRole('option', { name: /Cash \(/ }))
  expect(screen.getByRole('checkbox', { name: /Keep current balance/ })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Import' })).toBeDisabled()
  release()
  await waitFor(() => expect(screen.getByRole('button', { name: 'Import' })).toBeEnabled())
})

it('a failed snapshot says the balance cannot be kept and still allows a plain import', async () => {
  const log = snapshotHandlers([])
  server.use(http.get('*/api/v1/account/get-balance-snapshot', () => HttpResponse.json({ success: false, message: 'boom', code: 0, exceptionType: 'x' }, { status: 500 })))
  const { onComplete } = renderDialog()
  const user = await uploadCsv()
  expect(await screen.findByText(/Couldn't check the current balances/)).toBeInTheDocument()
  expect(screen.queryByRole('checkbox', { name: /Keep current balance/ })).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Import' }))
  await waitFor(() => expect(onComplete).toHaveBeenCalledWith({ imported: 2, failed: 0, errors: [] }))
  expect(log).toEqual(['import'])
})
