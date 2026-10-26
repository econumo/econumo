import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
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

it('does not cancel touch pointerdown on options so the list can pan', async () => {
  const user = userEvent.setup()
  render(<EntitySelect aria-label="Category" value={null} onChange={() => {}} options={OPTIONS} />)

  await user.click(combobox())
  const option = await screen.findByRole('option', { name: 'Food' })

  expect(fireEvent.pointerDown(option, { pointerType: 'touch' })).toBe(true)
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

it('a disabled option is marked disabled and cannot be clicked', async () => {
  const user = userEvent.setup()
  const onChange = vi.fn()
  const options = [...OPTIONS, { value: 'c4', label: 'Locked', disabled: true }]
  render(<EntitySelect aria-label="Category" value={null} onChange={onChange} options={options} />)

  await user.click(combobox())
  const locked = await screen.findByRole('option', { name: 'Locked' })
  expect(locked).toHaveAttribute('aria-disabled', 'true')
  await user.click(locked)
  expect(onChange).not.toHaveBeenCalled()
})

it('Enter on a filter that matches only a disabled option selects nothing', async () => {
  const user = userEvent.setup()
  const onChange = vi.fn()
  const options = [...OPTIONS, { value: 'c4', label: 'Locked', disabled: true }]
  render(<EntitySelect aria-label="Category" value={null} onChange={onChange} options={options} />)

  await user.click(combobox())
  await user.keyboard('lock')
  expect(await screen.findByRole('option', { name: 'Locked' })).toBeInTheDocument()
  await user.keyboard('{Enter}')
  expect(onChange).not.toHaveBeenCalled()
})

it('matches when a character is skipped', async () => {
  const user = userEvent.setup()
  render(<EntitySelect aria-label="Category" value={null} onChange={() => {}} options={[...OPTIONS, { value: 'c4', label: 'Groceries' }]} />)
  await user.click(combobox())
  await user.keyboard('grcries')
  expect(await screen.findByRole('option', { name: 'Groceries' })).toBeInTheDocument()
})

it('lists a prefix match before a substring match', async () => {
  const user = userEvent.setup()
  const options = [{ value: 'x1', label: 'Car rent' }, { value: 'x2', label: 'Rent' }]
  render(<EntitySelect aria-label="Category" value={null} onChange={() => {}} options={options} />)
  await user.click(combobox())
  await user.keyboard('rent')
  const names = (await screen.findAllByRole('option')).map((o) => o.textContent)
  expect(names).toEqual(['Rent', 'Car rent'])
})

it('still offers create when the typed name only fuzzy-matches', async () => {
  const user = userEvent.setup()
  render(<EntitySelect aria-label="Category" value={null} onChange={() => {}} options={OPTIONS} onCreate={() => {}} />)
  await user.click(combobox())
  await user.keyboard('Fod')
  expect(await screen.findByRole('option', { name: 'Food' })).toBeInTheDocument()
  expect(screen.getByRole('option', { name: /Fod/ })).toBeInTheDocument()
})

it('trailing spaces do not make an existing name look new', async () => {
  const user = userEvent.setup()
  render(<EntitySelect aria-label="Category" value={null} onChange={() => {}} options={OPTIONS} onCreate={() => {}} />)
  await user.click(combobox())
  await user.keyboard('rent ')
  expect(await screen.findByRole('option', { name: 'Rent' })).toBeInTheDocument()
  expect(screen.queryByRole('option', { name: /Add/ })).not.toBeInTheDocument()
})

it('creates with the typed name trimmed', async () => {
  const user = userEvent.setup()
  const onCreate = vi.fn()
  render(<EntitySelect aria-label="Category" value={null} onChange={() => {}} options={OPTIONS} onCreate={onCreate} />)
  await user.click(combobox())
  await user.keyboard(' Tea ')
  await user.click(await screen.findByRole('option', { name: /Add.*Tea/ }))
  expect(onCreate).toHaveBeenCalledWith('Tea')
})

it('matches on searchText when given, not on the displayed label', async () => {
  const user = userEvent.setup()
  const accounts = [
    { value: 'a1', label: 'Savings ($1,000.00)', searchText: 'Savings' },
    { value: 'a2', label: 'Cash ($10.00)', searchText: 'Cash' },
    { value: 'a3', label: 'Cashback card ($5.00)', searchText: 'Cashback card' },
  ]
  render(<EntitySelect aria-label="Account" value={null} onChange={() => {}} options={accounts} />)
  const input = screen.getByRole('combobox', { name: 'Account' })

  await user.click(input)
  await user.keyboard('csh')
  expect((await screen.findAllByRole('option')).map((o) => o.textContent)).toEqual(['Cash ($10.00)', 'Cashback card ($5.00)'])

  await user.clear(input)
  await user.keyboard('1')
  expect(screen.queryAllByRole('option')).toHaveLength(0)

  await user.clear(input)
  await user.keyboard('cash')
  expect((await screen.findAllByRole('option'))[0]).toHaveTextContent('Cash ($10.00)')
})

// portaled into a grid dialog, a laid-out wrapper took a grid row and its gap
// and grew the dialog by 16px the moment the list opened
it('the open list adds no box to the dialog it is portaled into', async () => {
  const user = userEvent.setup()
  render(
    <div data-slot="dialog-content" className="grid gap-4">
      <EntitySelect aria-label="Category" value={null} onChange={() => {}} options={OPTIONS} />
    </div>,
  )
  await user.click(combobox())
  await screen.findByRole('option', { name: 'Food' })
  expect(document.querySelector('[data-base-ui-portal]')).toHaveClass('contents')
})

describe('on a phone', () => {
  beforeEach(() => {
    window.matchMedia = vi.fn().mockImplementation((q: string) => ({
      matches: true, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
    }))
  })

  // a field iOS's ^ / v keys stop on, but one that never raises a keyboard
  it('the field shows the selection and raises no keyboard', () => {
    render(<EntitySelect aria-label="Category" value="c1" onChange={() => {}} options={OPTIONS} />)
    expect(combobox()).toHaveValue('Food')
    expect(combobox()).toHaveAttribute('inputmode', 'none')
  })

  it("the sheet's ^ / v stops move on to the fields around the picker", async () => {
    const user = userEvent.setup()
    render(
      <div data-slot="dialog-content">
        <input aria-label="Amount" />
        <EntitySelect aria-label="Category" value={null} onChange={() => {}} options={OPTIONS} />
        <textarea aria-label="Notes" />
      </div>,
    )
    await user.click(combobox())
    await screen.findByRole('dialog')
    // the hidden stops sit either side of the search
    const stops = () => within(screen.getByRole('dialog')).getAllByRole('textbox', { hidden: true })
    act(() => stops()[2].focus())
    expect(screen.getByRole('textbox', { name: 'Notes' })).toHaveFocus()
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())

    await user.click(combobox())
    act(() => stops()[0].focus())
    expect(screen.getByRole('textbox', { name: 'Amount' })).toHaveFocus()
  })

  it('tapping it opens a sheet whose search filters the list, and a pick closes it', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<EntitySelect aria-label="Category" value={null} onChange={onChange} options={OPTIONS} />)

    await user.click(combobox())
    const sheet = await screen.findByRole('dialog', { name: 'Category' })
    // the search takes focus straight away, so typing filters without another tap
    expect(within(sheet).getByRole('textbox')).toHaveFocus()
    await user.keyboard('fue')
    expect(within(sheet).queryByRole('option', { name: 'Food' })).not.toBeInTheDocument()
    await user.click(within(sheet).getByRole('option', { name: 'Fuel' }))

    expect(onChange).toHaveBeenCalledWith('c2')
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  })

  it('Enter in the search picks the first match; the clear row clears', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<EntitySelect aria-label="Category" value="c1" clearable onChange={onChange} options={OPTIONS} />)

    await user.click(combobox())
    await user.click(within(await screen.findByRole('dialog')).getByRole('option', { name: '—' }))
    expect(onChange).toHaveBeenLastCalledWith(null)

    await user.click(combobox())
    await user.type(within(await screen.findByRole('dialog')).getByRole('textbox'), 're{Enter}')
    expect(onChange).toHaveBeenLastCalledWith('c3')
  })

  it('a new name offers to create it', async () => {
    const user = userEvent.setup()
    const onCreate = vi.fn()
    render(<EntitySelect aria-label="Category" value={null} onChange={() => {}} onCreate={onCreate} options={OPTIONS} />)

    await user.click(combobox())
    const sheet = await screen.findByRole('dialog')
    await user.type(within(sheet).getByRole('textbox'), 'Gym')
    await user.click(within(sheet).getByRole('option', { name: /Gym/ }))
    expect(onCreate).toHaveBeenCalledWith('Gym')
  })
})
