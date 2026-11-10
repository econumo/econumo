import { act, render, screen } from '@testing-library/react'
import { onlineManager } from '@tanstack/react-query'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { PromptDialog } from '@/components/PromptDialog'

function mockMatchMedia() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

beforeEach(() => {
  mockMatchMedia()
  onlineManager.setOnline(false)
})
afterEach(() => {
  act(() => onlineManager.setOnline(true))
})

const confirm = (offlineOk?: boolean) => (
  <ConfirmDialog open onClose={() => {}} onConfirm={() => {}} question="Sure?" confirmLabel="Yes" cancelLabel="No" offlineOk={offlineOk} />
)

it('a confirmation is disabled offline unless its action queues', () => {
  const { rerender } = render(confirm())
  expect(screen.getByRole('button', { name: 'Yes' })).toBeDisabled()
  expect(screen.getByRole('button', { name: 'No' })).toBeEnabled()
  rerender(confirm(true))
  expect(screen.getByRole('button', { name: 'Yes' })).toBeEnabled()
})

it('a name prompt cannot be submitted offline', () => {
  render(<PromptDialog open onClose={() => {}} onSubmit={() => {}} title="Name" inputLabel="Name" submitLabel="Save" cancelLabel="Cancel" />)
  expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
})
