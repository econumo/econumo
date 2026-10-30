import { render, screen } from '@testing-library/react'
import { PeriodDates } from './PeriodDates'

it('shows nothing for a calendar-month budget', () => {
  const { container } = render(<PeriodDates periodKey="2026-11-01" startDay={1} />)
  expect(container).toBeEmptyDOMElement()
})

it('shows the shifted range with its inclusive last day', () => {
  render(<PeriodDates periodKey="2026-11-01" startDay={28} />)
  const range = screen.getByTestId('period-dates')
  expect(range).toHaveTextContent('Oct 28')
  expect(range).toHaveTextContent('Nov 27')
})

it('names the start month for days 2-15', () => {
  render(<PeriodDates periodKey="2026-10-01" startDay={5} />)
  expect(screen.getByTestId('period-dates')).toHaveTextContent(/Oct 5.*Nov 4/)
})
