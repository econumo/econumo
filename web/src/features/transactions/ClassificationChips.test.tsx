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

it('gives a selected budget tag the same tint treatment, at full strength', () => {
  renderChips([chip({ checked: true })])
  const budget = byName('Italy 2026').className
  // same shape as a reporting chip — tint + border, not a solid block — but a
  // stronger tint, since one budget tag per transaction can carry the weight
  expect(solidFill(budget)).toBe(false)
  expect(budget).toContain('border-primary')
  expect(budget).toContain('bg-primary/25')
})

it('keeps a selected reporting tag quieter than a selected budget tag', () => {
  renderChips([
    chip({ checked: true }),
    chip({ kind: 'label', id: 'lb1', name: 'Doggo', icon: 'label', checked: true }),
  ])
  const budget = byName('Italy 2026').className
  const reporting = byName('Doggo').className
  expect(solidFill(reporting)).toBe(false)
  // still visibly selected, just quieter than the budget tag beside it
  expect(reporting).toContain('bg-primary/10')
  expect(budget).toContain('bg-primary/25')
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
