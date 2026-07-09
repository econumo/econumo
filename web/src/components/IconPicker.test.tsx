import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { IconPicker } from './IconPicker'
import { availableIcons } from '@/lib/icons'

// embla queries matchMedia for responsive options on init
beforeEach(() => {
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches: false,
    media: query,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  }))
})

it('renders every icon as an option and marks the current one selected', () => {
  render(<IconPicker value={availableIcons[1]} onChange={() => {}} aria-label="Icon" />)
  const listbox = screen.getByRole('listbox', { name: 'Icon' })
  expect(listbox).toBeInTheDocument()
  expect(screen.getAllByRole('option')).toHaveLength(availableIcons.length)
  expect(screen.getByRole('option', { name: availableIcons[1] })).toHaveAttribute('aria-selected', 'true')
})

it('paginates 36 icons per page (dot per page)', () => {
  render(<IconPicker value={availableIcons[0]} onChange={() => {}} aria-label="Icon" />)
  const dots = screen.getAllByRole('button', { name: /icons page/ })
  expect(dots).toHaveLength(Math.ceil(availableIcons.length / 36))
})

it('picking an icon calls onChange', async () => {
  const user = userEvent.setup()
  const onChange = vi.fn()
  render(<IconPicker value={availableIcons[0]} onChange={onChange} aria-label="Icon" />)
  await user.click(screen.getByRole('option', { name: availableIcons[2] }))
  expect(onChange).toHaveBeenCalledWith(availableIcons[2])
})

it('fill mode keeps the base 4-row pagination when no extra height is allocated', () => {
  // jsdom reports zero heights, so the measured allocation never exceeds the
  // 4-row minimum — fill must behave exactly like the fixed picker
  render(<IconPicker fill value={availableIcons[0]} onChange={() => {}} aria-label="Icon" />)
  expect(screen.getByRole('listbox', { name: 'Icon' })).toBeInTheDocument()
  const dots = screen.getAllByRole('button', { name: /icons page/ })
  expect(dots).toHaveLength(Math.ceil(availableIcons.length / 36))
})
