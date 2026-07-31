import { isNewerVersion } from './version'

it.each([
  ['v1.0.2', 'v1.0.1', true],
  ['v1.1.0', 'v1.0.9', true],
  ['v2.0.0', 'v1.9.9', true],
  ['v1.0.2', 'v1.0.2', false],
  ['v1.0.1', 'v1.0.2', false],
  ['v1.0.10', 'v1.0.9', true],
  ['v1.0.2', 'dev', false],
  ['dev', 'v1.0.2', false],
  ['', 'v1.0.2', false],
  ['v1.0.2', '', false],
  ['1.0.3', 'v1.0.2', false],
])('isNewerVersion(%s, %s) -> %s', (latest, current, expected) => {
  expect(isNewerVersion(latest, current)).toBe(expected)
})

it('dismissed-update version round-trips through localStorage', async () => {
  const { getDismissedUpdateVersion, setDismissedUpdateVersion } = await import('./version')
  localStorage.removeItem('econumo.dismissed-update-version')
  expect(getDismissedUpdateVersion()).toBeNull()
  setDismissedUpdateVersion('v1.0.2')
  expect(getDismissedUpdateVersion()).toBe('v1.0.2')
})
