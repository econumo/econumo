import { describe, expect, it } from 'vitest'
import { avatarColors, avatarColorAccents, avatarColorSwatches, avatarIcons, defaultAvatar, joinAvatar, splitAvatar } from './avatars'

describe('avatars', () => {
  it('has 7 colors each with accent and swatch classes', () => {
    expect(avatarColors).toHaveLength(7)
    for (const color of avatarColors) {
      expect(avatarColorAccents[color], color).toMatch(/^bg-.+ text-/)
      expect(avatarColorSwatches[color], color).toMatch(/^bg-/)
    }
  })

  it('fits on a single IconPicker page (9 cols x 4 rows)', () => {
    expect(avatarIcons.length).toBeGreaterThan(0)
    expect(avatarIcons.length).toBeLessThanOrEqual(36)
  })

  it('fuchsia renders the brand magenta', () => {
    expect(avatarColorAccents.fuchsia).toBe('bg-avatar-fuchsia-tint text-avatar-fuchsia')
    expect(avatarColorSwatches.fuchsia).toBe('bg-avatar-fuchsia')
  })

  it('joins and splits round-trip', () => {
    expect(joinAvatar('face', 'teal')).toBe('face:teal')
    expect(splitAvatar('face:teal')).toEqual({ icon: 'face', color: 'teal' })
  })

  it('splits on the last colon', () => {
    expect(splitAvatar('weird:name:teal')).toEqual({ icon: 'weird:name', color: 'teal' })
  })

  it('falls back to sky for unknown or missing color', () => {
    expect(splitAvatar('face:neon').color).toBe('sky')
    expect(splitAvatar('just_an_icon').color).toBe('sky')
    expect(splitAvatar('just_an_icon').icon).toBe('just_an_icon')
  })

  it('default avatar is diamond on sky', () => {
    expect(defaultAvatar).toBe('diamond:sky')
    expect(splitAvatar(defaultAvatar)).toEqual({ icon: 'diamond', color: 'sky' })
  })
})
