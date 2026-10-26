import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CommandItem } from '@/components/ui/command'
import { PickerSheet } from './PickerSheet'

function mockPhone(matches: boolean) {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

const NAMES = ['Apple', 'Banana', 'Cherry']

function renderSheet(props: Partial<Parameters<typeof PickerSheet>[0]> = {}) {
  const onPick = vi.fn()
  render(
    <PickerSheet open onClose={() => {}} title="Fruit" searchLabel="Search" actions={<button type="button">OK</button>} {...props}>
      {(query) =>
        NAMES.filter((n) => n.toLowerCase().includes(query.toLowerCase())).map((name) => (
          <CommandItem key={name} value={name} onSelect={() => onPick(name)}>
            {name}
          </CommandItem>
        ))
      }
    </PickerSheet>,
  )
  return { onPick }
}

const follows = (a: Element, b: Element) => (a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0

describe('on a phone', () => {
  beforeEach(() => mockPhone(true))

  // the sheet rests on the keyboard: its pinned bottom row sits right above the keys
  it('puts the search below the list and above the actions, focused on open', () => {
    renderSheet()
    const search = screen.getByRole('textbox', { name: 'Search' })
    expect(follows(screen.getByRole('option', { name: 'Cherry' }), search)).toBe(true)
    expect(follows(search, screen.getByRole('button', { name: 'OK' }))).toBe(true)
    expect(search).toHaveFocus()
  })

  it('leaves the keyboard down when the search is not search-first', () => {
    renderSheet({ focusSearch: false })
    expect(screen.getByRole('textbox', { name: 'Search' })).not.toHaveFocus()
  })

  it('filters as you type; Enter picks the highlighted row, ↓ moves the highlight', async () => {
    const user = userEvent.setup()
    const { onPick } = renderSheet()
    await user.keyboard('an')
    expect(screen.queryByRole('option', { name: 'Apple' })).not.toBeInTheDocument()
    await user.keyboard('{Enter}')
    expect(onPick).toHaveBeenLastCalledWith('Banana')

    await user.clear(screen.getByRole('textbox', { name: 'Search' }))
    await user.keyboard('{ArrowDown}{Enter}')
    expect(onPick).toHaveBeenLastCalledWith('Banana')
  })

  it('Enter on a typed query is the caller’s best match until an arrow key moves the highlight', async () => {
    const user = userEvent.setup()
    const onSearchEnter = vi.fn()
    const { onPick } = renderSheet({ onSearchEnter })
    await user.keyboard('a{Enter}')
    expect(onSearchEnter).toHaveBeenCalledWith('a')
    expect(onPick).not.toHaveBeenCalled()

    await user.keyboard('{ArrowDown}{Enter}')
    expect(onPick).toHaveBeenCalledWith('Banana')
  })

  // an IME's Enter confirms the word being composed, not a pick
  it('ignores Enter during IME composition', () => {
    const onSearchEnter = vi.fn()
    const { onPick } = renderSheet({ onSearchEnter })
    const search = screen.getByRole('textbox', { name: 'Search' })
    fireEvent.keyDown(search, { key: 'Enter', isComposing: true })
    fireEvent.keyDown(search, { key: 'Enter', keyCode: 229 })
    expect(onSearchEnter).not.toHaveBeenCalled()
    expect(onPick).not.toHaveBeenCalled()
  })

  it('a short list leaves the search out', () => {
    renderSheet({ searchable: false })
    expect(screen.queryByRole('textbox', { name: 'Search' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'OK' })).toBeInTheDocument()
  })
})

describe('on a desktop', () => {
  beforeEach(() => mockPhone(false))

  it('keeps the search above the list', () => {
    renderSheet()
    const search = screen.getByRole('combobox', { name: 'Search' })
    expect(follows(search, screen.getByRole('option', { name: 'Apple' }))).toBe(true)
    expect(search).toHaveFocus()
  })
})
