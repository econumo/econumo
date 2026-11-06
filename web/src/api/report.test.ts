import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import * as reportApi from './report'

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('get-net-worth-report sends budgetId/from/to and omits empty ones', async () => {
  const urls: URL[] = []
  server.use(
    http.get('*/api/v1/report/get-net-worth-report', ({ request }) => {
      urls.push(new URL(request.url))
      return HttpResponse.json({ success: true, message: '', data: { item: { currencyId: 'c', months: ['2026-01-01'], accounts: [] } } })
    }),
  )
  const report = await reportApi.getNetWorthReport({ budgetId: 'b1', from: '', to: '2026-06-01' })
  expect(report.months).toEqual(['2026-01-01'])
  expect(Object.fromEntries(urls[0].searchParams)).toEqual({ budgetId: 'b1', to: '2026-06-01' })
})
