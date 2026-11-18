import { setupServer } from 'msw/node'

export const server = setupServer()

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

// The fields of a multipart body, file parts as their text. Request.formData() cannot be used in a
// handler: on newer Node, undici's multipart parser asserts its File is the global File, which
// jsdom replaces, so the parse throws inside MSW and the request goes unhandled.
export async function readMultipart(request: Request): Promise<Record<string, string>> {
  const boundary = /boundary=(?:"([^"]+)"|([^;]+))/.exec(request.headers.get('Content-Type') ?? '')
  if (!boundary) throw new Error('not a multipart body')
  const body = await request.text()
  const fields: Record<string, string> = {}
  for (const part of body.split(`--${boundary[1] ?? boundary[2]}`).slice(1, -1)) {
    const headerEnd = part.indexOf('\r\n\r\n')
    const name = /name="([^"]*)"/.exec(part.slice(0, headerEnd))
    if (name) fields[name[1]] = part.slice(headerEnd + 4, -2)
  }
  return fields
}
