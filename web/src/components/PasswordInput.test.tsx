import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { PasswordInput } from './PasswordInput'

it('hides the value by default and toggles visibility with the eye button', async () => {
  const user = userEvent.setup()
  render(<PasswordInput placeholder="Password" />)

  const input = screen.getByPlaceholderText('Password')
  expect(input).toHaveAttribute('type', 'password')

  await user.click(screen.getByRole('button', { name: 'Show password' }))
  expect(input).toHaveAttribute('type', 'text')

  await user.click(screen.getByRole('button', { name: 'Hide password' }))
  expect(input).toHaveAttribute('type', 'password')
})

it('keeps the typed value across toggling', async () => {
  const user = userEvent.setup()
  render(<PasswordInput placeholder="Password" />)

  const input = screen.getByPlaceholderText('Password')
  await user.type(input, 'qwe123')
  await user.click(screen.getByRole('button', { name: 'Show password' }))
  expect(input).toHaveValue('qwe123')
})
