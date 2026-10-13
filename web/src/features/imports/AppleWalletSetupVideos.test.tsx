import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { AppleWalletConnect } from './AppleWalletSetup'

vi.mock('./setupVideos', () => ({
  SETUP_VIDEOS: [
    { label: 'iOS 27', url: 'https://econumo.com/videos/apple-wallet-ios27' },
    { label: 'iOS 26', url: 'https://econumo.com/videos/apple-wallet-ios26' },
  ],
}))

it('offers one setup video per iOS version, each in a new window', () => {
  server.use(...coreHandlers())
  render(
    <QueryClientProvider client={new QueryClient()}>
      <AppleWalletConnect />
    </QueryClientProvider>,
  )
  expect(screen.getByText('Watch the setup video:')).toBeInTheDocument()
  for (const [label, url] of [['iOS 27', 'https://econumo.com/videos/apple-wallet-ios27'], ['iOS 26', 'https://econumo.com/videos/apple-wallet-ios26']]) {
    const link = screen.getByRole('link', { name: label })
    expect(link).toHaveAttribute('href', url)
    expect(link).toHaveAttribute('target', '_blank')
  }
})
