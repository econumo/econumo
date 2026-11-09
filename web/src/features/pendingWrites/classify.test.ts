import { describe, expect, it } from 'vitest'
import { classifyWriteError } from './classify'

const envelope = { success: false, message: 'Nope', code: 0, errors: {} }
const html = '<html><body>Bad gateway</body></html>'

function respond(status: number, data: unknown) {
  return { isAxiosError: true, response: { status, data } }
}

describe('classifyWriteError', () => {
  it.each([
    ['no response', { isAxiosError: true, code: 'ERR_NETWORK' }, 'wait'],
    ['a timeout', { isAxiosError: true, code: 'ECONNABORTED' }, 'wait'],
    ['409 with envelope', respond(409, envelope), 'done'],
    ['400 with envelope', respond(400, envelope), 'refused'],
    ['402 with envelope', respond(402, envelope), 'refused'],
    ['429 with envelope', respond(429, envelope), 'wait'],
    ['500 with envelope', respond(500, envelope), 'retry'],
    ['502 with HTML', respond(502, html), 'wait'],
    ['404 with HTML', respond(404, html), 'wait'],
    ['409 with HTML', respond(409, html), 'wait'],
    ['an empty body', respond(400, null), 'wait'],
    ['401 with envelope', respond(401, envelope), 'pause'],
    ['401 with HTML', respond(401, html), 'pause'],
  ])('%s', (_name, err, expected) => {
    expect(classifyWriteError(err)).toBe(expected)
  })
})
