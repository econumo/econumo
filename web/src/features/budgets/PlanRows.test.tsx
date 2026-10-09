import { render, screen } from '@testing-library/react'
import { SumCell } from './PlanRows'

const sel = '2026-07-01'

function sumCell(sum: { actual: string; planned: string } | null, month = sel) {
  render(<SumCell index={0} sum={sum} month={month} ctx={{ selected: sel }} fmt={(v) => `${v}.00`} />)
  return screen.getByTestId('plan-sum-0')
}

describe('SumCell', () => {
  it("reads actual and plan as two figures at the cell's two ends: a small truncating actual, a plan that never shrinks", () => {
    const cell = sumCell({ actual: '12445', planned: '6500' })
    expect(cell.textContent).toBe('12445.006500.00')
    expect(cell).toHaveClass('justify-between')
    const actual = screen.getByText('12445.00')
    expect(actual).toHaveClass('text-xs', 'min-w-0', 'truncate')
    expect(actual).toHaveAttribute('title', '12445.00')
    expect(screen.getByText('6500.00')).toHaveClass('text-sm', 'shrink-0', 'whitespace-nowrap')
  })

  it('a zero actual next to a plan is a dash: "— … plan"', () => {
    expect(sumCell({ actual: '0', planned: '55' }).textContent).toBe('—55.00')
  })

  it('a zero actual with no plan is blank', () => {
    expect(sumCell({ actual: '0', planned: '0' }).textContent).toBe('')
  })

  it('a lone actual has no dot', () => {
    expect(sumCell({ actual: '20', planned: '0' }).textContent).toBe('20.00')
  })

  it('after the selected month the plan stands alone', () => {
    expect(sumCell({ actual: '0', planned: '55' }, '2026-08-01').textContent).toBe('55.00')
  })

  it('a month with no data is empty', () => {
    expect(sumCell(null).textContent).toBe('')
  })
})
