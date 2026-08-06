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

it('passes an accent colour class through to the glyph', () => {
  const { container } = render(<EntityIcon name="label" className="text-violet-600 dark:text-violet-400" />)
  const span = container.querySelector('span.material-icon')!
  expect(span).toHaveClass('text-violet-600', 'dark:text-violet-400')
})
