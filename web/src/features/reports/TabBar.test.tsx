import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { TabBar } from './TabBar'

function setViewport(phone: boolean) {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: phone && q.includes('max-width'), media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

const tabs = [{ id: 't1', title: 'Groceries 3y' }, { id: 't2', title: 'Net worth' }]

function renderBar(over: Partial<Parameters<typeof TabBar>[0]> = {}) {
  const props = {
    tabs, activeId: 't1', canAdd: true,
    onActivate: vi.fn(), onNew: vi.fn(), onClose: vi.fn(), onRename: vi.fn(), onDuplicate: vi.fn(), onMove: vi.fn(),
    ...over,
  }
  render(<TabBar {...props} />)
  return props
}

describe('desktop strip', () => {
  beforeEach(() => setViewport(false))

  it('marks the active tab and activates another on click', async () => {
    const p = renderBar()
    expect(screen.getByRole('tab', { name: 'Groceries 3y' })).toHaveAttribute('aria-selected', 'true')
    await userEvent.click(screen.getByRole('tab', { name: 'Net worth' }))
    expect(p.onActivate).toHaveBeenCalledWith('t2')
  })

  it('double-click renames inline; Enter saves', async () => {
    const p = renderBar()
    const user = userEvent.setup()
    await user.dblClick(screen.getByRole('tab', { name: 'Net worth' }))
    const input = screen.getByRole('textbox', { name: 'Tab name' })
    expect(input).toHaveValue('Net worth')
    await user.clear(input)
    await user.type(input, 'Wealth{Enter}')
    expect(p.onRename).toHaveBeenCalledWith('t2', 'Wealth')
    expect(screen.queryByRole('textbox')).toBeNull()
  })

  it('Escape cancels the rename', async () => {
    const p = renderBar()
    const user = userEvent.setup()
    await user.dblClick(screen.getByRole('tab', { name: 'Net worth' }))
    await user.type(screen.getByRole('textbox', { name: 'Tab name' }), 'xyz{Escape}')
    expect(p.onRename).not.toHaveBeenCalled()
    expect(screen.queryByRole('textbox')).toBeNull()
    expect(screen.getByRole('tab', { name: 'Net worth' })).toBeInTheDocument()
  })

  it('× closes a tab without activating it', async () => {
    const p = renderBar()
    await userEvent.click(screen.getByRole('button', { name: 'Close Net worth' }))
    expect(p.onClose).toHaveBeenCalledWith('t2')
    expect(p.onActivate).not.toHaveBeenCalled()
  })

  it('+ opens a new tab', async () => {
    const p = renderBar()
    await userEvent.click(screen.getByRole('button', { name: 'New tab' }))
    expect(p.onNew).toHaveBeenCalled()
  })

  it('+ is disabled at the cap', () => {
    renderBar({ canAdd: false })
    expect(screen.getByRole('button', { name: 'New tab' })).toBeDisabled()
  })
})

describe('phone pills', () => {
  beforeEach(() => setViewport(true))

  it('renders pills without close buttons; a tap activates', async () => {
    const p = renderBar()
    expect(screen.queryByRole('button', { name: 'Close Net worth' })).toBeNull()
    await userEvent.click(screen.getByRole('tab', { name: 'Net worth' }))
    expect(p.onActivate).toHaveBeenCalledWith('t2')
  })

  it('a long press opens Rename / Duplicate / Close', async () => {
    const p = renderBar()
    const user = userEvent.setup()
    // useLongPress treats a Shift+click as the long press
    await user.keyboard('{Shift>}')
    await user.click(screen.getByRole('tab', { name: 'Net worth' }))
    await user.keyboard('{/Shift}')
    expect(p.onActivate).not.toHaveBeenCalled()
    await user.click(await screen.findByRole('menuitem', { name: 'Duplicate' }))
    expect(p.onDuplicate).toHaveBeenCalledWith('t2')
  })

  it('Rename from the menu edits the pill in place; Escape cancels', async () => {
    const p = renderBar()
    const user = userEvent.setup()
    await user.keyboard('{Shift>}')
    await user.click(screen.getByRole('tab', { name: 'Net worth' }))
    await user.keyboard('{/Shift}')
    await user.click(await screen.findByRole('menuitem', { name: 'Rename' }))
    const input = await screen.findByRole('textbox', { name: 'Tab name' })
    await user.type(input, '{Escape}')
    expect(p.onRename).not.toHaveBeenCalled()
    expect(screen.queryByRole('textbox')).toBeNull()
  })

  it('Close from the menu closes the tab', async () => {
    const p = renderBar()
    const user = userEvent.setup()
    await user.keyboard('{Shift>}')
    await user.click(screen.getByRole('tab', { name: 'Groceries 3y' }))
    await user.keyboard('{/Shift}')
    await user.click(await screen.findByRole('menuitem', { name: 'Close' }))
    expect(p.onClose).toHaveBeenCalledWith('t1')
  })
})
