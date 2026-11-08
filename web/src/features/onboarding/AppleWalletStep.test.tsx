import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { AppleWalletStep } from './AppleWalletStep'

it('Set up opens the Apple Wallet settings; Skip skips', async () => {
  const onSkip = vi.fn().mockResolvedValue(undefined)
  const router = createMemoryRouter(
    [
      { path: '/', element: <AppleWalletStep onDone={vi.fn()} onSkip={onSkip} /> },
      { path: '/settings/apple-wallet', element: <div>WALLET SETTINGS</div> },
    ],
    { initialEntries: ['/'] },
  )
  render(<RouterProvider router={router} />)
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Skip' }))
  expect(onSkip).toHaveBeenCalledTimes(1)
  await user.click(screen.getByRole('button', { name: 'Set up' }))
  expect(await screen.findByText('WALLET SETTINGS')).toBeInTheDocument()
})

it('the Shortcut tile opens the Apple Wallet settings', async () => {
  const router = createMemoryRouter(
    [
      { path: '/', element: <AppleWalletStep onDone={vi.fn()} onSkip={vi.fn()} /> },
      { path: '/settings/apple-wallet', element: <div>WALLET SETTINGS</div> },
    ],
    { initialEntries: ['/'] },
  )
  render(<RouterProvider router={router} />)
  await userEvent.setup().click(screen.getByRole('button', { name: 'Set up the Shortcut' }))
  expect(await screen.findByText('WALLET SETTINGS')).toBeInTheDocument()
})
