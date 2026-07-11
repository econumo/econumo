import { describe, expect, it } from 'vitest'
import { avatarColors, avatarColorAccents, avatarColorSwatches, avatarIcons, defaultAvatar, joinAvatar, splitAvatar } from './avatars'

describe('avatars', () => {
  it('has 7 colors each with accent and swatch classes', () => {
    expect(avatarColors).toHaveLength(7)
    for (const color of avatarColors) {
      expect(avatarColorAccents[color], color).toMatch(/^border-.+ text-/)
      expect(avatarColorSwatches[color], color).toMatch(/^bg-/)
    }
  })

  it('fits on a single IconPicker page (9 cols x 4 rows)', () => {
    expect(avatarIcons.length).toBeGreaterThan(0)
    expect(avatarIcons.length).toBeLessThanOrEqual(36)
  })

  it('fuchsia renders the brand magenta', () => {
    expect(avatarColorAccents.fuchsia).toBe('border-econumo-magenta text-econumo-magenta')
    expect(avatarColorSwatches.fuchsia).toBe('bg-econumo-magenta')
  })

  it('joins and splits round-trip', () => {
    expect(joinAvatar('face', 'teal')).toBe('face:teal')
    expect(splitAvatar('face:teal')).toEqual({ icon: 'face', color: 'teal' })
  })

  it('splits on the last colon', () => {
    expect(splitAvatar('weird:name:teal')).toEqual({ icon: 'weird:name', color: 'teal' })
  })

  it('falls back to fuchsia for unknown or missing color', () => {
    expect(splitAvatar('face:neon').color).toBe('fuchsia')
    expect(splitAvatar('just_an_icon').color).toBe('fuchsia')
    expect(splitAvatar('just_an_icon').icon).toBe('just_an_icon')
  })

  it('default avatar is face on fuchsia', () => {
    expect(defaultAvatar).toBe('face:fuchsia')
    expect(splitAvatar(defaultAvatar)).toEqual({ icon: 'face', color: 'fuchsia' })
  })
})
