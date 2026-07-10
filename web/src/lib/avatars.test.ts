import { describe, expect, it } from 'vitest'
import { avatarColors, avatarColorClasses, defaultAvatar, joinAvatar, splitAvatar } from './avatars'

describe('avatars', () => {
  it('has 16 colors each with a background class', () => {
    expect(avatarColors).toHaveLength(16)
    for (const color of avatarColors) {
      expect(avatarColorClasses[color], color).toMatch(/^bg-/)
    }
  })

  it('fuchsia renders the brand magenta', () => {
    expect(avatarColorClasses.fuchsia).toBe('bg-econumo-magenta')
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
