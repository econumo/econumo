import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import { avatarColors } from './avatars'
import { availableIcons } from './icons'

// Guards the two cross-language contracts in internal/user/avatar.go:
// the color allowlist must match exactly, and every icon the backend can
// randomly assign must exist in the frontend icon set.
function goStringSlice(src: string, varName: string): string[] {
  const m = src.match(new RegExp(`${varName} = \\[\\]string\\{([^}]*)\\}`, 's'))
  if (!m) {
    throw new Error(`${varName} slice not found in avatar.go`)
  }
  return [...m[1].matchAll(/"([a-z0-9_]+)"/g)].map((x) => x[1])
}

describe('backend avatar constants stay in sync', () => {
  // vitest runs with cwd = web/, so the Go source is one level up.
  const goSrc = readFileSync(resolve(process.cwd(), '../internal/user/avatar.go'), 'utf8')

  it('color allowlists are identical (names and order)', () => {
    expect(goStringSlice(goSrc, 'AvatarColors')).toEqual([...avatarColors])
  })

  it('every backend random icon exists in availableIcons', () => {
    const backendIcons = goStringSlice(goSrc, 'RandomAvatarIcons')
    expect(backendIcons.length).toBeGreaterThan(0)
    for (const icon of backendIcons) {
      expect(availableIcons, `backend icon "${icon}" missing from availableIcons`).toContain(icon)
    }
  })

  it('the standard default is a backend random icon on fuchsia', () => {
    expect(goSrc).toContain('DefaultAvatar = "face:fuchsia"')
    expect(availableIcons).toContain('face')
  })
})
