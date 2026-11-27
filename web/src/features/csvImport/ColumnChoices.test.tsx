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

it('puts the pick first, then the likely parts, then the rest under Other parts of your file', () => {
  renderChoices({ selected: 3, candidates: [1], tags: { 0: 'used for category', 1: 'used for payee' } })
  const group = screen.getByRole('radiogroup')
  const rows = within(group).getAllByRole('radio')
  expect(rows.map((r) => r.textContent)).toEqual([
    expect.stringContaining('Mexico2025 · #oregon2410'),
    expect.stringContaining('Cafe'),
    expect.stringContaining('Coffee'),
    expect.stringContaining('A long reference number'),
  ])
  expect(group.textContent!.indexOf('Other parts of your file')).toBeGreaterThan(group.textContent!.indexOf('Cafe'))
  expect(group.textContent!.indexOf('Other parts of your file')).toBeLessThan(group.textContent!.indexOf('Coffee'))
  expect(screen.getByRole('radio', { name: /Coffee/ })).toHaveAccessibleDescription('used for category')
  expect(screen.getByRole('radio', { name: /Cafe/ })).toHaveAccessibleDescription('used for payee')
})

it('has no divider when every part fits', () => {
  renderChoices({ selected: 0, candidates: [0, 1, 2, 3] })
  expect(screen.queryByText('Other parts of your file')).not.toBeInTheDocument()
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

it('with a list that truly has one entry, a note says so', () => {
  const profiles = profileColumns(parseCsvText('Date\n2026-01-02\n'))
  render(<ColumnChoices profiles={profiles} label="Which?" candidates={[0]} selected={null} onSelect={() => {}} nothingFitsNote="Nothing fits." onlyOneNote="Only one fits." />)
  expect(screen.getByText('Only one fits.')).toBeInTheDocument()
  expect(screen.getAllByRole('radio')).toHaveLength(1)
})

it('with one that fits and others in the file, all are listed and there is no note', () => {
  renderChoices({ selected: null, candidates: [1] })
  expect(screen.queryByText('Only one fits.')).not.toBeInTheDocument()
  expect(screen.getAllByRole('radio')).toHaveLength(4)
  expect(screen.getByText('Other parts of your file')).toBeInTheDocument()
})

it('says nothing extra with several that fit, or with a None answer beside the one', () => {
  renderChoices({ selected: null, candidates: [0, 1] })
  expect(screen.queryByText('Only one fits.')).not.toBeInTheDocument()
  expect(screen.queryByText('Nothing fits, so here is everything.')).not.toBeInTheDocument()
})

it('a None answer beside the one that fits needs no note', () => {
  renderChoices({ selected: null, candidates: [1], noneLabel: 'None' })
  expect(screen.queryByText('Only one fits.')).not.toBeInTheDocument()
  expect(screen.getAllByRole('radio')).toHaveLength(5)
})
