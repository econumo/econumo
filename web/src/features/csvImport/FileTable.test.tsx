import { useState } from 'react'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { FileTable } from './FileTable'
import { parseCsvText } from './parse'

const FILE = parseCsvText('Day,Sum,Text,\n2026-01-02,-5,Coffee,\n2026-01-03,7,Salary,\n,,,\n2026-01-04,1,Gum,\n')

function Harness(props: { initial?: number | null; labels?: Record<number, string>; locked?: Set<number>; noneLabel?: string; mutedRows?: Set<number> }) {
  const [selected, setSelected] = useState<number | null>(props.initial ?? null)
  return (
    <>
      <FileTable
        file={FILE}
        label="Tap the column with the date"
        selected={selected}
        onSelect={setSelected}
        labels={props.labels}
        locked={props.locked}
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

it('a locked column cannot be chosen by tap or by keyboard', async () => {
  render(<Harness initial={0} locked={new Set([1])} />)
  const user = userEvent.setup()
  await user.click(screen.getByRole('cell', { name: '-5' }))
  expect(selected()).toBe('0')
  await user.tab()
  await user.keyboard('{ArrowRight}')
  expect(selected()).toBe('2')
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
