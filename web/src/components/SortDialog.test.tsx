import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import '@/app/i18n'
import { SortDialog } from './SortDialog'

beforeEach(() => {
  localStorage.clear()
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

describe('SortDialog', () => {
  it('emits a name sort', () => {
    const onPick = vi.fn()
    render(<SortDialog open onClose={() => {}} onPick={onPick} />)
    fireEvent.click(screen.getByText('Alphabetically (A-Z)'))
    expect(onPick).toHaveBeenCalledWith({ by: 'name', direction: 'asc' })
  })

  it('emits a usage sort with the selected period', () => {
    const onPick = vi.fn()
    render(<SortDialog open onClose={() => {}} onPick={onPick} />)
    fireEvent.click(screen.getByRole('button', { name: '3' }))
    fireEvent.click(screen.getByText('Most used first'))
    expect(onPick).toHaveBeenCalledWith({ by: 'usage', direction: 'desc', periodMonths: 3 })
  })
})
