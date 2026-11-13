import { expect, it, vi } from 'vitest'
import { downloadBlob } from './download'

it('downloads through an anchor', () => {
  const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
  downloadBlob(new Blob(['x']), 'x.csv')
  expect(click).toHaveBeenCalled()
  click.mockRestore()
})
