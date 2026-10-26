import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { OptionSelect } from './OptionSelect'

function mockPhone(matches: boolean) {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

const OPTIONS = [
  { value: 'monthly', label: 'Monthly' },
  { value: 'weekly', label: 'Weekly' },
]

function renderSelect() {
  const onValueChange = vi.fn()
  render(<OptionSelect aria-label="Schedule" title="Schedule" value="monthly" onValueChange={onValueChange} options={OPTIONS} />)
  return { onValueChange }
}

it('on a phone it opens the picker sheet, with no search, and a pick closes it', async () => {
  mockPhone(true)
  const user = userEvent.setup()
  const { onValueChange } = renderSelect()
  const trigger = screen.getByRole('button', { name: 'Schedule' })
  expect(trigger).toHaveTextContent('Monthly')
  await user.click(trigger)
  const sheet = await screen.findByRole('dialog', { name: 'Schedule' })
  expect(within(sheet).queryByRole('textbox')).not.toBeInTheDocument()
  expect(within(sheet).getByRole('option', { name: 'Monthly' })).toHaveAttribute('data-checked', 'true')
  await user.click(within(sheet).getByRole('option', { name: 'Weekly' }))
  expect(onValueChange).toHaveBeenCalledWith('weekly')
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
})

it('on a desktop it is the dropdown', () => {
  mockPhone(false)
  renderSelect()
  expect(screen.getByRole('combobox', { name: 'Schedule' })).toHaveTextContent('Monthly')
})
