import { expect, it } from 'vitest'
import { CLASSIFICATION_KINDS, DEFAULT_ICON } from './classificationKind'

it('maps each kind to its default Material ligature', () => {
  expect(DEFAULT_ICON.tag).toBe('tag')     // renders as a hashtag
  expect(DEFAULT_ICON.label).toBe('label')
})

it('offers the reporting kind first, so it is the default choice', () => {
  expect(CLASSIFICATION_KINDS[0]).toBe('label')
  expect(CLASSIFICATION_KINDS).toEqual(['label', 'tag'])
})
