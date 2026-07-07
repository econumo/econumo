import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { useUiStore } from '@/app/uiStore'
import { SwitchAccountPrompt } from './SwitchAccountPrompt'

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  server.use(...coreHandlers())
  useUiStore.setState({ switchAccountPrompt: null })
})

it('renders the recipient name and navigates on click', async () => {
  const user = userEvent.setup()
  const router = createMemoryRouter(
    [
      { path: '/', element: <SwitchAccountPrompt /> },
      { path: '/account/:id', element: <div>ACCOUNT PAGE</div> },
    ],
    { initialEntries: ['/'] },
  )
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  useUiStore.getState().setSwitchAccountPrompt('a2')
  expect(await screen.findByText('Bank')).toBeInTheDocument()
  expect(screen.getByText('Switch to')).toBeInTheDocument()
  await user.click(screen.getByText('Bank'))
  expect(await screen.findByText('ACCOUNT PAGE')).toBeInTheDocument()
  expect(useUiStore.getState().switchAccountPrompt).toBeNull()
})
