import { useState } from 'react'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { CollapsedColumn } from './columnLabels'
import { FileTable } from './FileTable'
import { parseCsvText } from './parse'
import type { StepId } from './prepare'

const FILE = parseCsvText('Day,Sum,Text,\n2026-01-02,-5,Coffee,\n2026-01-03,7,Salary,\n,,,\n2026-01-04,1,Gum,\n')

function Harness(props: {
  initial?: number | null
  labels?: Record<number, string>
  collapsed?: Record<number, CollapsedColumn>
  onOpenStep?: (step: StepId) => void
  noneLabel?: string
  mutedRows?: Set<number>
}) {
  const [selected, setSelected] = useState<number | null>(props.initial ?? null)
  return (
    <>
      <FileTable
        file={FILE}
        label="Tap the column with the date"
        selected={selected}
        onSelect={setSelected}
        labels={props.labels}
        collapsed={props.collapsed}
        onOpenStep={props.onOpenStep}
        noneLabel={props.noneLabel}
        mutedRows={props.mutedRows}
      />
      <output aria-label="selected">{String(selected)}</output>
    </>
  )
}

const selected = () => screen.getByLabelText('selected').textContent

it('shows the header and the rows with something in them, every column', () => {
  render(<Harness />)
  const group = screen.getByRole('radiogroup', { name: 'Tap the column with the date' })
  expect(within(group).getAllByRole('radio').map((r) => r.textContent)).toEqual(['Day', 'Sum', 'Text', 'Column 4'])
  expect(within(group).getAllByRole('row')).toHaveLength(4)
  expect(screen.getByRole('cell', { name: 'Gum' })).toBeInTheDocument()
})

it('tapping any cell of a column chooses it', async () => {
  render(<Harness initial={0} />)
  const user = userEvent.setup()
  await user.click(screen.getByRole('cell', { name: 'Salary' }))
  expect(selected()).toBe('2')
  expect(screen.getByRole('radio', { name: 'Text' })).toBeChecked()
  expect(screen.getByRole('radio', { name: 'Day' })).not.toBeChecked()
  expect(screen.getByRole('cell', { name: 'Salary' })).toHaveClass('bg-primary/10')
})

it('the chosen column is the one stop for Tab, and arrow keys move the choice', async () => {
  render(<Harness initial={1} />)
  const user = userEvent.setup()
  await user.tab()
  expect(screen.getByRole('radio', { name: 'Sum' })).toHaveFocus()
  await user.keyboard('{ArrowRight}')
  expect(selected()).toBe('2')
  expect(screen.getByRole('radio', { name: 'Text' })).toHaveFocus()
  await user.keyboard('{ArrowLeft}{ArrowLeft}')
  expect(selected()).toBe('0')
  await user.keyboard('{End}')
  expect(selected()).toBe('3')
})

it('names what other steps use a column for', () => {
  render(<Harness labels={{ 1: 'Amount' }} />)
  expect(screen.getByRole('radio', { name: 'Sum' })).toHaveAccessibleDescription('Amount')
})

it('a column another step answered is a narrow strip that opens that step', async () => {
  const onOpenStep = vi.fn()
  render(<Harness initial={0} collapsed={{ 1: { label: 'Amount', step: 'amount' } }} onOpenStep={onOpenStep} />)
  const user = userEvent.setup()
  expect(screen.queryByRole('radio', { name: 'Sum' })).not.toBeInTheDocument()
  expect(screen.queryByRole('cell', { name: '-5' })).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Amount — tap to change' }))
  expect(onOpenStep).toHaveBeenCalledWith('amount')
  expect(selected()).toBe('0')
})

it('arrow keys skip a collapsed column; the step’s own choice stays open even if another step uses it', async () => {
  render(<Harness initial={0} collapsed={{ 0: { label: 'Date', step: 'date' }, 1: { label: 'Amount', step: 'amount' } }} />)
  const user = userEvent.setup()
  expect(screen.getByRole('radio', { name: 'Day' })).toBeChecked()
  await user.tab()
  await user.keyboard('{ArrowRight}')
  expect(selected()).toBe('2')
  expect(screen.getByRole('button', { name: 'Date — tap to change' })).toBeInTheDocument()
})

it('offers no column as an answer when asked to', async () => {
  render(<Harness initial={2} noneLabel="The file has no description" />)
  const user = userEvent.setup()
  await user.click(screen.getByRole('radio', { name: 'The file has no description' }))
  expect(selected()).toBe('null')
  expect(screen.getByRole('radio', { name: 'The file has no description' })).toBeChecked()
})

it('rows that will be skipped are shown muted', () => {
  render(<Harness mutedRows={new Set([1])} />)
  expect(screen.getByRole('cell', { name: 'Salary' }).closest('tr')).toHaveClass('text-muted-foreground/70')
  expect(screen.getByRole('cell', { name: 'Coffee' }).closest('tr')).not.toHaveClass('text-muted-foreground/70')
})
