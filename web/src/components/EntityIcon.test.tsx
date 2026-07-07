import { render } from '@testing-library/react'
import { EntityIcon } from './EntityIcon'

it('renders the material ligature name as text content', () => {
  const { container } = render(<EntityIcon name="account_balance" />)
  const span = container.querySelector('span.material-icon')!
  expect(span).toHaveTextContent('account_balance')
})

it('falls back to question_mark for missing names', () => {
  const { container } = render(<EntityIcon name={null} />)
  expect(container.querySelector('span.material-icon')).toHaveTextContent('question_mark')
})
