import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ColumnChoices } from './ColumnChoices'
import { profileColumns } from './columns'
import { parseCsvText } from './parse'

const CSV =
  'Memo,Merchant,Ref,Note\n' +
  'Coffee,Cafe,3f2504e0-4f89-41d3-9a0c-0305e82c3301,"Mexico2025;#oregon2410"\n' +
  'Salary,Shop,9b2f1c64-7a1e-4c55-8d2e-1b6f0a9e7d10,Trip\n'

function renderChoices(props: { selected: number | null; tags?: Record<number, string>; candidates?: number[] }) {
  const profiles = profileColumns(parseCsvText(CSV))
  render(<ColumnChoices profiles={profiles} label="Which?" candidates={props.candidates ?? [0]} onSelect={() => {}} {...props} />)
}

const showAll = () => userEvent.click(screen.getByRole('button', { name: 'Show all' }))

it('puts the pick first, then free parts, then the used ones under Already used', async () => {
  renderChoices({ selected: 3, tags: { 0: 'used for category', 1: 'used for payee' } })
  await showAll()
  const group = screen.getByRole('radiogroup')
  const rows = within(group).getAllByRole('radio')
  expect(rows.map((r) => r.textContent)).toEqual([
    expect.stringContaining('Mexico2025 · #oregon2410'),
    expect.stringContaining('A long reference number'),
    expect.stringContaining('Coffee'),
    expect.stringContaining('Cafe'),
  ])
  expect(group.textContent!.indexOf('Already used')).toBeGreaterThan(group.textContent!.indexOf('A long reference number'))
  expect(group.textContent!.indexOf('Already used')).toBeLessThan(group.textContent!.indexOf('Coffee'))
  expect(screen.getByRole('radio', { name: /Cafe/ })).toHaveAccessibleDescription('used for payee')
})

it('has no Already used heading when nothing is used elsewhere', async () => {
  renderChoices({ selected: 0 })
  await showAll()
  expect(screen.queryByText('Already used')).not.toBeInTheDocument()
})

it('a long reference is the plain phrase, its source name stays', async () => {
  renderChoices({ selected: 0 })
  await showAll()
  const row = screen.getByRole('radio', { name: 'A long reference number from “Ref”' })
  expect(row).not.toHaveTextContent('3f2504e0')
})
