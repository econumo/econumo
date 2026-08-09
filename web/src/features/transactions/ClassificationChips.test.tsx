import { render, screen } from '@testing-library/react'
import { ClassificationChips } from './ClassificationChips'
import type { ClassificationChip } from './useTransactionForm'

const chip = (over: Partial<ClassificationChip>): ClassificationChip => ({
  kind: 'tag', id: 'tg1', name: 'Italy 2026', icon: 'tag', checked: false, ...over,
})

function renderChips(chips: ClassificationChip[]) {
  render(<ClassificationChips chips={chips} onToggle={() => {}} />)
}

const byName = (name: string) => screen.getByRole('checkbox', { name: new RegExp(name) })

// A transaction carries ONE budget tag but any number of reporting tags, so the
// selected reporting chips are the ones that pile up. The budget tag keeps the
// solid fill (it is the single, deliberate choice); reporting tags get a
// quieter treatment so a handful of them stop competing with the dialog's own
// actions.
const solidFill = (cls: string) => cls.split(/\s+/).includes('bg-primary')

it('gives a selected budget tag the solid fill', () => {
  renderChips([chip({ checked: true })])
  expect(solidFill(byName('Italy 2026').className)).toBe(true)
})

it('keeps a selected reporting tag quieter than a selected budget tag', () => {
  renderChips([
    chip({ checked: true }),
    chip({ kind: 'label', id: 'lb1', name: 'Doggo', icon: 'label', checked: true }),
  ])
  const reporting = byName('Doggo').className
  // solid for the budget tag beside it, a quiet tint for this one
  expect(solidFill(byName('Italy 2026').className)).toBe(true)
  expect(solidFill(reporting)).toBe(false)
  expect(reporting).toContain('bg-primary/10')
})

it('leaves an unselected chip of either kind alone', () => {
  renderChips([
    chip({}),
    chip({ kind: 'label', id: 'lb1', name: 'Doggo', icon: 'label' }),
  ])
  expect(solidFill(byName('Italy 2026').className)).toBe(false)
  expect(solidFill(byName('Doggo').className)).toBe(false)
  expect(byName('Doggo').className).not.toMatch(/bg-primary\//)
})
