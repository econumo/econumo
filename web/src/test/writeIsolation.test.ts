import { http, HttpResponse, delay } from 'msw'
import { onlineManager, QueryClient } from '@tanstack/react-query'
import { server } from '@/test/msw'
import * as budgetApi from '@/api/budget'
import { submitChange } from '@/features/pendingWrites/submit'
import { getOps } from '@/features/pendingWrites/store'

const change = { kind: 'limit', budgetId: 'b1', elementId: 'e1', period: '2026-07-01', amount: '132' } as const
let hung: Request | undefined

// A write left unanswered times out 10 s later and queues itself: the queue is
// module state, so it would land in whichever test is running by then.
it('a test can end with a write left unanswered', async () => {
  onlineManager.setOnline(true)
  server.use(http.post('*/api/v1/budget/set-limit', async ({ request }) => {
    hung = request
    await delay('infinite')
    return HttpResponse.json({ success: true, message: '', data: {} })
  }))
  void submitChange(new QueryClient(), change, (opts) => budgetApi.setLimit(change, opts))
  await vi.waitFor(() => expect(hung).toBeDefined())
})

it('its request ends with it, and nothing of it reaches the next test', async () => {
  expect(hung?.signal.aborted).toBe(true)
  expect(getOps()).toEqual([])
})
