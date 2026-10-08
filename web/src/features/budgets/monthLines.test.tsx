import type { ReactNode } from 'react'
import { render, screen, within } from '@testing-library/react'
import { FolderLine, MonthSectionHeader, TotalLine } from './monthLines'
import { LineControlsContext, LineLayoutContext } from './monthLayout'

it('an empty folder reads as a dash in each of its three columns, so its ⋮ sits where the others do', () => {
  render(<FolderLine name="Bills" folded={false} onToggle={() => {}} sums={null} actionsColumn={false} menu={[{ label: 'Edit', onSelect: () => {} }]} />)
  const menu = screen.getByRole('button', { name: 'menu Bills' })
  const columns = menu.nextElementSibling
  expect(columns?.children).toHaveLength(3)
  expect(columns?.textContent).toBe('———')
  expect(screen.queryByTestId('stat-line')).toBeNull()
})

function inPlan(node: ReactNode, cols = 3, selectedCol = 1) {
  return render(<LineLayoutContext.Provider value={{ kind: 'plan', cols, selectedCol }}>{node}</LineLayoutContext.Provider>)
}

it('plan layout: a section line shows its per-month sums while open, never headings', () => {
  inPlan(<MonthSectionHeader foldKey="expense" label="Expenses" headings={['A', 'B', 'C']} sums={['1', '2', '3']} actionsColumn={false} testId="sec" />)
  const line = screen.getByTestId('sec')
  expect(within(line).getByText('2')).toBeInTheDocument()
  expect(within(line).queryByText('B')).not.toBeInTheDocument()
})

it('plan layout: the selected month column is tinted on every line', () => {
  inPlan(<FolderLine name="Daily" folded={false} onToggle={() => {}} sums={['1', '2', '3']} actionsColumn={false} />)
  const cells = document.querySelectorAll('[data-col]')
  expect(cells).toHaveLength(3)
  expect(cells[1].className).toContain('bg-accent/40')
  expect(cells[0].className).not.toContain('bg-accent/40')
})

it('plan layout: every month column opens with a vertical hairline', () => {
  inPlan(<FolderLine name="Daily" folded={false} onToggle={() => {}} sums={['1', '2', '3']} actionsColumn={false} />)
  for (const cell of document.querySelectorAll('[data-col]')) {
    expect(cell.className).toContain('border-l')
    expect(cell.className).toContain('border-border/60')
  }
})

it('budget layout: the three figure columns draw no vertical lines', () => {
  render(<FolderLine name="Daily" folded={false} onToggle={() => {}} sums={['1', '2', '3']} actionsColumn={false} />)
  expect(document.querySelector('.border-l')).toBeNull()
})

it('plan layout: an empty folder shows one dash per month', () => {
  inPlan(<FolderLine name="Empty" folded={false} onToggle={() => {}} sums={null} actionsColumn={false} />, 4, 0)
  expect(screen.getAllByText('—')).toHaveLength(4)
})

it('plan layout: a totals line colours each negative month on its own', () => {
  inPlan(<TotalLine testId="bal" label="Balance" values={['5', '-3', '1']} negative={[false, true, false]} actionsColumn={false} />)
  expect(screen.getByText('-3').closest('[data-col]')!.className).toContain('text-expense')
  expect(screen.getByText('5').closest('[data-col]')!.className).not.toContain('text-expense')
})

it('plan layout: a hidden ⋮ takes no room in the name column until the line is hovered', () => {
  inPlan(<FolderLine name="A rather long folder name" folded={false} onToggle={() => {}} sums={['1', '2', '3']} actionsColumn={false} menu={[{ label: 'Edit', onSelect: () => {} }]} />)
  const slot = screen.getByRole('button', { name: 'menu A rather long folder name' }).parentElement!
  expect(slot.className).toMatch(/(^| )w-0( |$)/)
  expect(slot.className).toContain('group-hover/line:w-7')
  expect(slot.className).toContain('has-[[data-state=open]]:w-7')
})

it('plan layout: on a touch screen in edit mode the ⋮ keeps its room', () => {
  render(
    <LineLayoutContext.Provider value={{ kind: 'plan', cols: 3, selectedCol: 1 }}>
      <LineControlsContext.Provider value="always">
        <FolderLine name="Bills" folded={false} onToggle={() => {}} sums={['1', '2', '3']} actionsColumn={false} menu={[{ label: 'Edit', onSelect: () => {} }]} />
      </LineControlsContext.Provider>
    </LineLayoutContext.Provider>,
  )
  expect(screen.getByRole('button', { name: 'menu Bills' }).parentElement!.className).not.toMatch(/(^| )w-0( |$)/)
})
