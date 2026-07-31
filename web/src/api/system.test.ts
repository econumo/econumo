import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { getUpdateInfo } from './system'

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('returns the update info from the envelope', async () => {
  server.use(
    http.get('*/api/v1/system/get-update-info', () =>
      HttpResponse.json({ success: true, message: '', data: { version: 'v1.2.3', url: 'https://econumo.com/releases/v1.2.3' } }),
    ),
  )
  await expect(getUpdateInfo()).resolves.toEqual({ version: 'v1.2.3', url: 'https://econumo.com/releases/v1.2.3' })
})

// A reverse proxy that blocks the path (observed: nginx 403 HTML in front of a
// live instance) must read as "no update known" — a rejection here paints the
// sync icon amber and keeps it spinning through retry cycles forever.
it('resolves null when the endpoint is blocked by a proxy', async () => {
  server.use(
    http.get('*/api/v1/system/get-update-info', () =>
      HttpResponse.html('<html><body>403 Forbidden</body></html>', { status: 403 }),
    ),
  )
  await expect(getUpdateInfo()).resolves.toBeNull()
})

it('resolves null when the response is not the JSON envelope', async () => {
  server.use(
    http.get('*/api/v1/system/get-update-info', () => HttpResponse.html('<html>captive portal</html>')),
  )
  await expect(getUpdateInfo()).resolves.toBeNull()
})
