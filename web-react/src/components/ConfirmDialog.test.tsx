import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ConfirmDialog } from './ConfirmDialog'

function mockMatchMedia() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

it('renders the question and fires confirm/cancel', async () => {
  mockMatchMedia()
  const user = userEvent.setup()
  const onClose = vi.fn()
  const onConfirm = vi.fn()
  render(
    <ConfirmDialog
      open
      onClose={onClose}
      onConfirm={onConfirm}
      question="Are you sure you want to delete this transaction?"
      confirmLabel="Delete"
      cancelLabel="Cancel"
    />,
  )
  expect(screen.getByText('Are you sure you want to delete this transaction?')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Delete' }))
  expect(onConfirm).toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: 'Cancel' }))
  expect(onClose).toHaveBeenCalled()
})

it('cancel sits left of the confirm action; destructive colors the confirm button', () => {
  mockMatchMedia()
  render(
    <ConfirmDialog
      open
      onClose={() => {}}
      onConfirm={() => {}}
      question="Delete it?"
      confirmLabel="Delete"
      cancelLabel="Cancel"
      destructive
    />,
  )
  const cancel = screen.getByRole('button', { name: 'Cancel' })
  const confirm = screen.getByRole('button', { name: 'Delete' })
  // DOM order inside the two-column grid: cancel first (left), action second (right)
  expect(cancel.compareDocumentPosition(confirm) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  expect(confirm).toHaveAttribute('data-variant', 'destructive')
  expect(cancel).toHaveAttribute('data-variant', 'secondary')
})
