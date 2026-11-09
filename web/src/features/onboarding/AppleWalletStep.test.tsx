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

it('says the integration can be set up later in Settings', () => {
  const router = createMemoryRouter([{ path: '/', element: <AppleWalletStep onDone={vi.fn()} onSkip={vi.fn()} /> }], { initialEntries: ['/'] })
  render(<RouterProvider router={router} />)
  expect(screen.getByText('You can set up the Apple Wallet integration any time later in Settings.')).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Set up the Shortcut' })).not.toBeInTheDocument()
})
