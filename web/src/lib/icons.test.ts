import { expect, it } from 'vitest'
import { availableIcons, defaultAccountIcon, defaultCategoryIcon, defaultEnvelopeIcon } from './icons'

it('defaults new items to neutral catalogue icons: a star for categories and envelopes, a card for accounts', () => {
  expect(defaultCategoryIcon).toBe('star')
  expect(defaultEnvelopeIcon).toBe('star')
  expect(defaultAccountIcon).toBe('credit_card')
  for (const icon of [defaultAccountIcon, defaultCategoryIcon, defaultEnvelopeIcon]) expect(availableIcons).toContain(icon)
})
