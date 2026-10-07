import type { ReactNode } from 'react'
import { render, screen, within } from '@testing-library/react'
import { FolderLine, MonthSectionHeader, TotalLine } from './monthLines'
import { LineLayoutContext } from './monthLayout'

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

it('plan layout: an empty folder shows one dash per month', () => {
  inPlan(<FolderLine name="Empty" folded={false} onToggle={() => {}} sums={null} actionsColumn={false} />, 4, 0)
  expect(screen.getAllByText('—')).toHaveLength(4)
})

it('plan layout: a totals line colours each negative month on its own', () => {
  inPlan(<TotalLine testId="bal" label="Balance" values={['5', '-3', '1']} negative={[false, true, false]} actionsColumn={false} />)
  expect(screen.getByText('-3').closest('[data-col]')!.className).toContain('text-expense')
  expect(screen.getByText('5').closest('[data-col]')!.className).not.toContain('text-expense')
})
