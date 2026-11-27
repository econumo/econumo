import { render, screen, within } from '@testing-library/react'
import { ColumnChoices } from './ColumnChoices'
import { profileColumns } from './columns'
import { parseCsvText } from './parse'

const CSV =
  'Memo,Merchant,Ref,Note\n' +
  'Coffee,Cafe,3f2504e0-4f89-41d3-9a0c-0305e82c3301,"Mexico2025;#oregon2410"\n' +
  'Salary,Shop,9b2f1c64-7a1e-4c55-8d2e-1b6f0a9e7d10,Trip\n'

function renderChoices(props: { selected: number | null; tags?: Record<number, string>; candidates?: number[]; noneLabel?: string }) {
  const profiles = profileColumns(parseCsvText(CSV))
  render(<ColumnChoices profiles={profiles} label="Which?" nothingFitsNote="Nothing fits, so here is everything." onlyOneNote="Only one fits." onSelect={() => {}} {...props} candidates={props.candidates ?? []} />)
}

it('puts the pick first, then free parts, then the used ones under Already used', () => {
  renderChoices({ selected: 3, tags: { 0: 'used for category', 1: 'used for payee' } })
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

it('has no Already used heading when nothing is used elsewhere', () => {
  renderChoices({ selected: 0 })
  expect(screen.queryByText('Already used')).not.toBeInTheDocument()
})

it('a long reference is the plain phrase, its source name stays', () => {
  renderChoices({ selected: 0 })
  const row = screen.getByRole('radio', { name: 'A long reference number from “Ref”' })
  expect(row).not.toHaveTextContent('3f2504e0')
})

it('with nothing that fits, the whole file is listed under a note, and there is no Show all', () => {
  renderChoices({ selected: null })
  expect(screen.getByText('Nothing fits, so here is everything.')).toBeInTheDocument()
  expect(screen.getAllByRole('radio')).toHaveLength(4)
  expect(screen.queryByRole('button', { name: 'Show all' })).not.toBeInTheDocument()
})

it('with exactly one that fits, only it is listed, under a note', () => {
  renderChoices({ selected: null, candidates: [1] })
  expect(screen.getByText('Only one fits.')).toBeInTheDocument()
  expect(screen.getAllByRole('radio')).toHaveLength(1)
  expect(screen.queryByRole('button', { name: 'Show all' })).not.toBeInTheDocument()
  expect(screen.queryByText('Already used')).not.toBeInTheDocument()
})

it('says nothing extra with several that fit, or with a None answer beside the one', () => {
  renderChoices({ selected: null, candidates: [0, 1] })
  expect(screen.queryByText('Only one fits.')).not.toBeInTheDocument()
  expect(screen.queryByText('Nothing fits, so here is everything.')).not.toBeInTheDocument()
})

it('a None answer beside the one that fits needs no note', () => {
  renderChoices({ selected: null, candidates: [1], noneLabel: 'None' })
  expect(screen.queryByText('Only one fits.')).not.toBeInTheDocument()
  expect(screen.getAllByRole('radio')).toHaveLength(2)
})
