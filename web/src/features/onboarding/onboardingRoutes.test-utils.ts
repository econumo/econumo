import { http, HttpResponse } from 'msw'
import { fixtureUser } from '@/test/fixtures'

// The Day 0 step routes (finish-import and the three step saves) as the server answers them: each marks its step
// in the onboarding option and answers {user, <key->id map>, budgetId}; a replay
// of an operation id or a call for a step already done writes nothing and
// answers with the current state.

type Step = 'import' | 'categories' | 'accounts' | 'plan'

export interface StepServer {
  calls: { path: string; body: any }[]
  option: string
  budgetId: string
  // makes the next call of the step fail: a 500, or a response lost after the server committed
  failNext: Partial<Record<Step, 'http500' | 'lost'>>
}

export const userWith = (option: string, base: typeof fixtureUser = fixtureUser) => ({
  ...base,
  options: base.options.map((o) => (o.name === 'onboarding' ? { ...o, value: option } : o)),
})

const markStep = (option: string, step: Step) =>
  [...option.split(',').filter((k) => k !== '' && k !== 'started' && k !== step && k !== `!${step}`), step].join(',')

export function stepRoutes(init: { option?: string; budgetId?: string; base?: typeof fixtureUser } = {}) {
  const srv: StepServer = { calls: [], option: init.option ?? '', budgetId: init.budgetId ?? '', failNext: {} }
  const seen = new Map<string, Record<string, string>>()
  let n = 0
  const handle = (step: Step, path: string, rowsOf: (body: any) => { key: string }[], prefix: string, mapKey?: 'categories' | 'accounts') =>
    http.post(`*/api/v1/onboarding/${path}`, async ({ request }) => {
      const body = (await request.json()) as any
      srv.calls.push({ path, body })
      const fault = srv.failNext[step]
      if (fault === 'http500') {
        delete srv.failNext[step]
        return HttpResponse.json({ success: false, message: 'boom', code: 0, exceptionType: 'Error' }, { status: 500 })
      }
      const done = srv.option.split(',').includes(step)
      let ids = seen.get(body.id)
      if (!ids && !done) {
        ids = Object.fromEntries(rowsOf(body).map((r) => [r.key, `${prefix}-${++n}`]))
        seen.set(body.id, ids)
        srv.option = markStep(srv.option, step)
        if (step !== 'plan' && !srv.budgetId) srv.budgetId = 'b-onb'
        if (step === 'plan' && body.budgetId) srv.budgetId = body.budgetId
      }
      if (fault === 'lost') {
        delete srv.failNext[step]
        return HttpResponse.error()
      }
      const data: Record<string, unknown> = { user: userWith(srv.option, init.base), budgetId: srv.budgetId }
      if (mapKey) data[mapKey] = ids ?? {}
      return HttpResponse.json({ success: true, message: '', data })
    })
  const handlers = [
    handle('import', 'finish-import', () => [], 'imp'),
    handle('categories', 'save-categories', (b) => b.categories ?? [], 'cat', 'categories'),
    handle('accounts', 'save-accounts', (b) => b.accounts ?? [], 'acc', 'accounts'),
    handle('plan', 'save-plan', () => [], 'plan'),
  ]
  return { srv, handlers }
}
