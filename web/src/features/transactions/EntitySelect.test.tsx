import { fireEvent, render, screen } from '@testing-library/react'
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

describe('on a phone, opening the picker scrolls the form until its list fits above the keyboard', () => {
  const original = Object.getOwnPropertyDescriptor(window, 'visualViewport')
  beforeEach(() => {
    window.matchMedia = vi.fn().mockImplementation((q: string) => ({
      matches: true, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
    }))
    // the keyboard leaves the top 500px visible
    Object.defineProperty(window, 'visualViewport', { configurable: true, value: Object.assign(new EventTarget(), { height: 500, offsetTop: 0 }) })
  })
  afterEach(() => {
    if (original) {
      Object.defineProperty(window, 'visualViewport', original)
    } else {
      delete (window as { visualViewport?: unknown }).visualViewport
    }
  })

  async function openAt(fieldTop: number, bodyTop = 60) {
    const user = userEvent.setup()
    render(
      <div data-slot="dialog-body">
        <EntitySelect aria-label="Category" value={null} onChange={() => {}} options={OPTIONS} />
      </div>,
    )
    const body = document.querySelector<HTMLElement>('[data-slot="dialog-body"]')!
    const field = document.querySelector<HTMLElement>('[data-slot="entity-select"]')!
    body.getBoundingClientRect = () => ({ top: bodyTop }) as DOMRect
    field.getBoundingClientRect = () => ({ top: fieldTop, bottom: fieldTop + 44 }) as DOMRect
    await user.click(combobox())
    await screen.findByRole('option', { name: 'Food' })
    await new Promise((resolve) => requestAnimationFrame(resolve))
    return body.scrollTop
  }

  it('leaves a field with room below it in place', async () => {
    expect(await openAt(100)).toBe(0)
  })

  it('scrolls a lower field up by the missing room', async () => {
    // 200 + 44 + 264 - 500
    expect(await openAt(200)).toBe(8)
  })

  it('stops once the field reaches the top of the form', async () => {
    // short by 208, but the field is only 400 - 300 - 8 below the form's top
    expect(await openAt(400, 300)).toBe(92)
  })
})
