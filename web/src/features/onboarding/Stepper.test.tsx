import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Stepper } from './Stepper'
import type { OnboardingStep } from './useOnboardingSteps'

function mockViewport(compact: boolean) {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: q.includes('1023') ? compact : false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

const steps: OnboardingStep[] = [
  { key: 'import', state: 'done' },
  { key: 'categories', state: 'skipped' },
  { key: 'accounts', state: 'open' },
  { key: 'plan', state: 'open' },
]

it('desktop shows every step with its state and marks the selected one current', () => {
  mockViewport(false)
  render(<Stepper steps={steps} current="accounts" onSelect={() => undefined} />)
  const item = (name: string) => screen.getByRole('button', { name: new RegExp(name) })
  expect(item('Import')).toHaveAttribute('data-state', 'done')
  expect(item('Import')).toHaveTextContent('optional')
  expect(screen.getByTestId('step-check-import')).toBeInTheDocument()
  expect(item('Categories')).toHaveAttribute('data-state', 'skipped')
  expect(item('Accounts')).toHaveAttribute('data-state', 'current')
  expect(item('Accounts')).toHaveAttribute('aria-current', 'step')
  expect(item('Budget & plan')).toHaveAttribute('data-state', 'open')
  // the state is spoken, not only coloured
  expect(item('Import')).toHaveAccessibleName(/Import.*, done/)
  expect(item('Categories')).toHaveAccessibleName(/Categories.*, skipped/)
  expect(item('Accounts')).not.toHaveAccessibleName(/done|skipped/)
  expect(screen.queryByText(/Getting started ·/)).not.toBeInTheDocument()
})

it('selecting a step reports its key', async () => {
  mockViewport(false)
  const onSelect = vi.fn()
  render(<Stepper steps={steps} current="accounts" onSelect={onSelect} />)
  await userEvent.click(screen.getByRole('button', { name: /Categories/ }))
  expect(onSelect).toHaveBeenCalledWith('categories')
})

it('a done step is read-only: it keeps its tick and says done, but cannot be selected', async () => {
  mockViewport(false)
  const onSelect = vi.fn()
  render(<Stepper steps={steps} current="accounts" onSelect={onSelect} />)
  const done = screen.getByRole('button', { name: /Import/ })
  expect(done).toHaveAttribute('aria-disabled', 'true')
  expect(done).not.toHaveClass('hover:text-foreground')
  expect(done).toHaveAccessibleName(/, done/)
  expect(screen.getByTestId('step-check-import')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: /Categories/ })).not.toHaveAttribute('aria-disabled')
  await userEvent.click(done)
  expect(onSelect).not.toHaveBeenCalled()
})

it('compact shows the position and a segmented bar that also selects', async () => {
  mockViewport(true)
  const onSelect = vi.fn()
  render(<Stepper steps={steps} current="accounts" onSelect={onSelect} />)
  expect(screen.getByText('Getting started · 3 of 4')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Accounts' })).toHaveAttribute('data-state', 'current')
  expect(screen.getByRole('button', { name: 'Import, done' })).toHaveAttribute('data-state', 'done')
  expect(screen.getByRole('button', { name: 'Categories, skipped' })).toHaveAttribute('data-state', 'skipped')
  // a 24px tall tap target around the thin bar
  expect(screen.getByRole('button', { name: 'Accounts' })).toHaveClass('h-6')
  await userEvent.click(screen.getByRole('button', { name: 'Import, done' }))
  expect(onSelect).not.toHaveBeenCalled()
  await userEvent.click(screen.getByRole('button', { name: 'Categories, skipped' }))
  expect(onSelect).toHaveBeenCalledWith('categories')
})
