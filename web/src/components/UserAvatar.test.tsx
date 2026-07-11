import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { UserAvatar } from './UserAvatar'

describe('UserAvatar', () => {
  it('renders the dark glyph on a pale tint of the same hue', () => {
    render(<UserAvatar avatar="pets:teal" />)
    const el = screen.getByTestId('user-avatar')
    expect(el).toHaveAttribute('data-avatar', 'pets:teal')
    expect(el.className).toContain('bg-avatar-teal-tint')
    expect(el.className).toContain('text-avatar-teal')
    expect(el).toHaveTextContent('pets')
  })

  it('falls back to sky for an unknown color', () => {
    render(<UserAvatar avatar="face:neon" />)
    expect(screen.getByTestId('user-avatar').className).toContain('bg-avatar-sky-tint')
  })

  it('is decorative (hidden from the accessibility tree)', () => {
    render(<UserAvatar avatar="face:fuchsia" />)
    expect(screen.getByTestId('user-avatar')).toHaveAttribute('aria-hidden', 'true')
  })

  it('applies size and extra classes', () => {
    render(<UserAvatar avatar="face:fuchsia" size="xl" className="rounded-none" />)
    const el = screen.getByTestId('user-avatar')
    expect(el.className).toContain('size-24')
    expect(el.className).toContain('rounded-none')
  })
})
