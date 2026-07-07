import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { PromptDialog } from './PromptDialog'
import { SortDialog } from './SortDialog'

function mockMatchMedia() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

beforeEach(mockMatchMedia)

it('prompt blocks submit on validation and submits a valid value', async () => {
  const user = userEvent.setup()
  const onSubmit = vi.fn()
  render(
    <PromptDialog
      open
      onClose={() => {}}
      onSubmit={onSubmit}
      title="Create new folder"
      inputLabel="Name"
      validate={(v) => (v.length < 3 ? 'Folder name must be 3-64 characters' : null)}
      submitLabel="Create"
      cancelLabel="Cancel"
    />,
  )
  await user.type(screen.getByLabelText('Name'), 'ab')
  await user.click(screen.getByRole('button', { name: 'Create' }))
  expect(await screen.findByText('Folder name must be 3-64 characters')).toBeInTheDocument()
  expect(onSubmit).not.toHaveBeenCalled()

  await user.type(screen.getByLabelText('Name'), 'c')
  await user.keyboard('{Enter}')
  expect(onSubmit).toHaveBeenCalledWith('abc')
})

it('prompt seeds the initial value for edit mode', () => {
  render(
    <PromptDialog open onClose={() => {}} onSubmit={() => {}} title="Change name" inputLabel="Name" initialValue="General" submitLabel="Update" cancelLabel="Cancel" />,
  )
  expect(screen.getByLabelText('Name')).toHaveValue('General')
})

it('sort dialog picks a direction', async () => {
  const user = userEvent.setup()
  const onPick = vi.fn()
  render(<SortDialog open onClose={() => {}} onPick={onPick} />)
  expect(screen.getByText('Sort')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Alphabetically (Z-A)' }))
  expect(onPick).toHaveBeenCalledWith('desc')
})
