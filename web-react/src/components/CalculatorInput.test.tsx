import { useState } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CalculatorInput } from './CalculatorInput'

function mockViewport(mobile: boolean) {
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches: query.includes('767') ? mobile : false,
    media: query,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  }))
}

function Harness({ onSubmit = () => {} }: { onSubmit?: () => void }) {
  const [value, setValue] = useState('')
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault()
        onSubmit()
      }}
    >
      <CalculatorInput aria-label="Amount" value={value} onChange={setValue} />
      <button type="submit">save</button>
    </form>
  )
}

it('evaluates when the user types =', async () => {
  mockViewport(false)
  const user = userEvent.setup()
  render(<Harness />)
  await user.type(screen.getByLabelText('Amount'), '5+3*2=')
  expect(screen.getByLabelText('Amount')).toHaveValue('11')
})

it('Enter evaluates a pending formula instead of submitting', async () => {
  mockViewport(false)
  const onSubmit = vi.fn()
  const user = userEvent.setup()
  render(<Harness onSubmit={onSubmit} />)
  await user.type(screen.getByLabelText('Amount'), '5+5{Enter}')
  expect(screen.getByLabelText('Amount')).toHaveValue('10')
  expect(onSubmit).not.toHaveBeenCalled()
})

it('Enter on a plain number submits the form', async () => {
  mockViewport(false)
  const onSubmit = vi.fn()
  const user = userEvent.setup()
  render(<Harness onSubmit={onSubmit} />)
  await user.type(screen.getByLabelText('Amount'), '42{Enter}')
  expect(onSubmit).toHaveBeenCalled()
})

it('leaves an invalid trailing-operator value as typed', async () => {
  mockViewport(false)
  const user = userEvent.setup()
  render(<Harness />)
  await user.type(screen.getByLabelText('Amount'), '5+')
  expect(screen.getByLabelText('Amount')).toHaveValue('5+')
})

it('mobile keypad appends operators and = evaluates', async () => {
  mockViewport(true)
  const user = userEvent.setup()
  render(<Harness />)
  const input = screen.getByLabelText('Amount')
  await user.type(input, '6')
  await user.click(screen.getByRole('button', { name: '×' }))
  await user.type(input, '7')
  await user.click(screen.getByRole('button', { name: '=' }))
  expect(input).toHaveValue('42')
})
