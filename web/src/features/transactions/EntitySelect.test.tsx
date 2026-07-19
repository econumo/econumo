import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { EntitySelect } from './EntitySelect'

const OPTIONS = [
  { value: 'c1', label: 'Food', icon: 'restaurant' },
  { value: 'c2', label: 'Fuel' },
  { value: 'c3', label: 'Rent' },
]

beforeEach(() => {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

const combobox = () => screen.getByRole('combobox', { name: 'Category' })

// macOS Safari's plain Tab only reaches text fields — a <button> picker is
// unreachable there, so the trigger must be a real <input>.
it('renders the picker as a text input so Tab reaches it in every browser', () => {
  render(<EntitySelect aria-label="Category" value={null} onChange={() => {}} options={OPTIONS} />)
  expect(combobox().tagName).toBe('INPUT')
})

it('shows the selected option label in the field', () => {
  render(<EntitySelect aria-label="Category" value="c1" onChange={() => {}} options={OPTIONS} />)
  expect(combobox()).toHaveValue('Food')
})

it('typing filters the options and Enter picks the highlighted one', async () => {
  const user = userEvent.setup()
  const onChange = vi.fn()
  render(<EntitySelect aria-label="Category" value={null} onChange={onChange} options={OPTIONS} />)

  await user.click(combobox())
  await user.keyboard('fue')
  expect(await screen.findByRole('option', { name: 'Fuel' })).toBeInTheDocument()
  expect(screen.queryByRole('option', { name: 'Food' })).not.toBeInTheDocument()

  await user.keyboard('{Enter}')
  expect(onChange).toHaveBeenCalledWith('c2')
})

it('clicking an option selects it', async () => {
  const user = userEvent.setup()
  const onChange = vi.fn()
  render(<EntitySelect aria-label="Category" value={null} onChange={onChange} options={OPTIONS} />)

  await user.click(combobox())
  await user.click(await screen.findByRole('option', { name: 'Rent' }))
  expect(onChange).toHaveBeenCalledWith('c3')
})

it('Escape abandons the search and restores the selected label', async () => {
  const user = userEvent.setup()
  const onChange = vi.fn()
  render(<EntitySelect aria-label="Category" value="c1" onChange={onChange} options={OPTIONS} />)

  await user.click(combobox())
  await user.keyboard('xyz')
  await user.keyboard('{Escape}')
  expect(combobox()).toHaveValue('Food')
  expect(onChange).not.toHaveBeenCalled()
})

it('offers create-on-the-fly when the text matches nothing and passes validation', async () => {
  const user = userEvent.setup()
  const onCreate = vi.fn()
  render(
    <EntitySelect
      aria-label="Category"
      value={null}
      onChange={() => {}}
      options={OPTIONS}
      onCreate={onCreate}
      createValidator={(name) => name.length >= 3}
    />,
  )

  await user.click(combobox())
  await user.keyboard('Te')
  expect(screen.queryByRole('option', { name: /Add/ })).not.toBeInTheDocument()

  await user.keyboard('a')
  expect(await screen.findByRole('option', { name: /Add.*Tea/ })).toBeInTheDocument()

  await user.keyboard('{Enter}')
  expect(onCreate).toHaveBeenCalledWith('Tea')
})

it('clearable shows a — row that clears the selection', async () => {
  const user = userEvent.setup()
  const onChange = vi.fn()
  render(<EntitySelect aria-label="Category" value="c1" onChange={onChange} options={OPTIONS} clearable />)

  await user.click(combobox())
  await user.click(await screen.findByRole('option', { name: '—' }))
  expect(onChange).toHaveBeenCalledWith(null)
})
