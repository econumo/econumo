import { expect, it } from 'vitest'
import { DEFAULT_ICON, kindAccentClass } from './classificationKind'

it('maps each kind to its default Material ligature', () => {
  expect(DEFAULT_ICON.tag).toBe('tag')     // renders as a hashtag
  expect(DEFAULT_ICON.label).toBe('label')
})

it('gives the two kinds visually distinct accents', () => {
  expect(kindAccentClass('tag')).not.toBe(kindAccentClass('label'))
})
