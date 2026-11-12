import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { PinPad } from './PinPad'

function dots(): number {
  return Number(screen.getByTestId('pin-dots').dataset.filled)
}

async function tap(user: ReturnType<typeof userEvent.setup>, digits: string) {
  for (const d of digits) {
    await user.click(screen.getByRole('button', { name: d }))
  }
}

it('verify mode submits on its own once the stored length is typed', async () => {
  const user = userEvent.setup()
  const onSubmit = vi.fn().mockResolvedValue(true)
  render(<PinPad mode="verify" length={4} onSubmit={onSubmit} />)
  expect(screen.getByTestId('pin-dots').children).toHaveLength(4)
  await tap(user, '123')
  expect(dots()).toBe(3)
  expect(onSubmit).not.toHaveBeenCalled()
  await tap(user, '4')
  await waitFor(() => expect(onSubmit).toHaveBeenCalledWith('1234'))
  await waitFor(() => expect(dots()).toBe(0))
  expect(screen.queryByRole('button', { name: 'Continue' })).not.toBeInTheDocument()
})

it('a rejected PIN shakes and clears the dots', async () => {
  const user = userEvent.setup()
  render(<PinPad mode="verify" length={4} onSubmit={() => false} error="Wrong PIN" />)
  await tap(user, '1111')
  await waitFor(() => expect(screen.getByTestId('pin-dots')).toHaveClass('pin-shake'))
  expect(dots()).toBe(0)
  expect(screen.getByText('Wrong PIN')).toHaveAttribute('aria-live', 'assertive')
})

it('backspace removes the last digit', async () => {
  const user = userEvent.setup()
  render(<PinPad mode="verify" length={4} onSubmit={vi.fn()} />)
  expect(screen.getByRole('button', { name: 'Delete digit' })).toBeDisabled()
  await tap(user, '12')
  await user.click(screen.getByRole('button', { name: 'Delete digit' }))
  expect(dots()).toBe(1)
})

it('create mode takes 4 to 8 digits and submits on Continue', async () => {
  const user = userEvent.setup()
  const onSubmit = vi.fn().mockResolvedValue(true)
  render(<PinPad mode="create" onSubmit={onSubmit} />)
  const next = screen.getByRole('button', { name: 'Continue' })
  await tap(user, '123')
  expect(next).toBeDisabled()
  await tap(user, '456789')
  expect(dots()).toBe(8)
  expect(screen.getByTestId('pin-dots').children).toHaveLength(8)
  expect(onSubmit).not.toHaveBeenCalled()
  await user.click(next)
  await waitFor(() => expect(onSubmit).toHaveBeenCalledWith('12345678'))
})

it('takes digits, Backspace and Enter from the keyboard, but not from a text field', async () => {
  const user = userEvent.setup()
  const onSubmit = vi.fn().mockResolvedValue(true)
  render(
    <>
      <input aria-label="other" />
      <PinPad mode="create" onSubmit={onSubmit} />
    </>,
  )
  await user.keyboard('12345')
  expect(dots()).toBe(5)
  await user.keyboard('{Backspace}')
  expect(dots()).toBe(4)
  await user.click(screen.getByLabelText('other'))
  await user.keyboard('9')
  expect(dots()).toBe(4)
  fireEvent.keyDown(window, { key: 'Enter' })
  await waitFor(() => expect(onSubmit).toHaveBeenCalledWith('1234'))
})

it('ignores the keyboard when another surface is on top', async () => {
  const user = userEvent.setup()
  render(<PinPad mode="verify" length={4} onSubmit={vi.fn()} keyboard={false} />)
  await user.keyboard('12')
  expect(dots()).toBe(0)
})

it('disabled blocks the keypad and the keyboard', async () => {
  const user = userEvent.setup()
  render(<PinPad mode="verify" length={4} onSubmit={vi.fn()} disabled />)
  expect(screen.getByRole('button', { name: '1' })).toBeDisabled()
  await user.keyboard('12')
  expect(dots()).toBe(0)
})

describe('Enter outside the pad', () => {
  it('still activates a focused button on the lock screen', async () => {
    const user = userEvent.setup()
    const onForgot = vi.fn()
    render(
      <>
        <PinPad mode="verify" length={4} onSubmit={vi.fn()} />
        <button type="button" onClick={onForgot}>
          Forgot PIN?
        </button>
      </>,
    )
    screen.getByRole('button', { name: 'Forgot PIN?' }).focus()
    await user.keyboard('{Enter}')
    expect(onForgot).toHaveBeenCalledOnce()
  })

  it('in create mode activates a focused outside button instead of submitting', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    const onClose = vi.fn()
    render(
      <>
        <PinPad mode="create" onSubmit={onSubmit} />
        <button type="button" onClick={onClose}>
          Close
        </button>
      </>,
    )
    await tap(user, '1234')
    screen.getByRole('button', { name: 'Close' }).focus()
    await user.keyboard('{Enter}')
    expect(onClose).toHaveBeenCalledOnce()
    expect(onSubmit).not.toHaveBeenCalled()
  })

  it('in create mode submits when nothing outside has focus', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn().mockReturnValue(true)
    render(<PinPad mode="create" onSubmit={onSubmit} />)
    await tap(user, '1234')
    ;(document.activeElement as HTMLElement | null)?.blur()
    await user.keyboard('{Enter}')
    await waitFor(() => expect(onSubmit).toHaveBeenCalledWith('1234'))
  })

  it('in create mode submits when focus is on the dialog around it', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn().mockReturnValue(true)
    render(
      <div role="dialog" tabIndex={-1} data-testid="dialog">
        <PinPad mode="create" onSubmit={onSubmit} />
      </div>,
    )
    await tap(user, '1234')
    screen.getByTestId('dialog').focus()
    await user.keyboard('{Enter}')
    await waitFor(() => expect(onSubmit).toHaveBeenCalledWith('1234'))
  })
})

it('shows the hint under the dots, and an error in its place', () => {
  const { rerender } = render(<PinPad mode="create" onSubmit={vi.fn()} hint="4 to 8 digits" />)
  const line = screen.getByText('4 to 8 digits')
  expect(line).toHaveClass('text-muted-foreground')
  expect(screen.getByTestId('pin-dots').compareDocumentPosition(line) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  rerender(<PinPad mode="create" onSubmit={vi.fn()} hint="4 to 8 digits" error="The PINs don't match. Try again." />)
  expect(screen.queryByText('4 to 8 digits')).not.toBeInTheDocument()
  expect(screen.getByText("The PINs don't match. Try again.")).toHaveClass('text-destructive')
})

it('create mode shows filled dots plus empty slots up to the minimum', async () => {
  const user = userEvent.setup()
  render(<PinPad mode="create" onSubmit={vi.fn()} />)
  expect(screen.getByTestId('pin-dots').children).toHaveLength(4)
  await tap(user, '12345')
  expect(screen.getByTestId('pin-dots').children).toHaveLength(5)
})

it('hides the delete key while nothing is typed', async () => {
  const user = userEvent.setup()
  render(<PinPad mode="verify" length={4} onSubmit={vi.fn()} />)
  const erase = screen.getByRole('button', { name: 'Delete digit' })
  expect(erase).toHaveClass('invisible')
  await tap(user, '1')
  expect(erase).not.toHaveClass('invisible')
  expect(erase).toBeEnabled()
})

it('a key never takes focus from a pointer, so no ring stays behind', () => {
  render(<PinPad mode="verify" length={4} onSubmit={vi.fn()} />)
  const key = screen.getByRole('button', { name: '5' })
  const down = new MouseEvent('mousedown', { bubbles: true, cancelable: true })
  key.dispatchEvent(down)
  expect(down.defaultPrevented).toBe(true)
})

it('a rejected PIN stays on screen in red while it shakes', async () => {
  const user = userEvent.setup()
  render(<PinPad mode="verify" length={4} onSubmit={() => false} />)
  await tap(user, '1111')
  await waitFor(() => expect(screen.getByTestId('pin-dots').querySelectorAll('.bg-destructive')).toHaveLength(4))
  await waitFor(() => expect(screen.getByTestId('pin-dots').querySelectorAll('.bg-destructive')).toHaveLength(0))
})
