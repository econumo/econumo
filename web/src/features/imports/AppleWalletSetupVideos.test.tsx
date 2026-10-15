import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { AppleWalletConnect } from './AppleWalletSetup'
import { youtubeId } from './setupVideos'

vi.mock('./setupVideos', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./setupVideos')>()),
  SETUP_VIDEOS: [
    { label: 'iOS 27', url: 'https://youtube.com/shorts/AbCdEfGhIj1?feature=share' },
    { label: 'iOS 26', url: 'https://example.com/apple-wallet-ios26.mp4' },
  ],
}))

beforeEach(() => {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  server.use(...coreHandlers())
})

function renderConnect() {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <AppleWalletConnect />
    </QueryClientProvider>,
  )
}

it('plays a YouTube setup video in a dialog, loading the player only then, with a way out to YouTube', async () => {
  const user = userEvent.setup()
  renderConnect()
  expect(screen.getByText('Watch the setup video:')).toBeInTheDocument()
  expect(document.querySelector('iframe')).toBeNull()
  await user.click(screen.getByRole('button', { name: 'iOS 27' }))
  const dialog = await screen.findByRole('dialog', { name: 'Setup video · iOS 27' })
  expect(within(dialog).getByTitle('Setup video · iOS 27')).toHaveAttribute('src', 'https://www.youtube-nocookie.com/embed/AbCdEfGhIj1?playsinline=1&rel=0&modestbranding=1')
  const out = within(dialog).getByRole('link', { name: 'Open on YouTube' })
  expect(out).toHaveAttribute('href', 'https://youtube.com/shorts/AbCdEfGhIj1?feature=share')
  expect(out).toHaveAttribute('target', '_blank')
})

it('a video link the player cannot take opens in a new window instead', () => {
  renderConnect()
  const link = screen.getByRole('link', { name: 'iOS 26' })
  expect(link).toHaveAttribute('href', 'https://example.com/apple-wallet-ios26.mp4')
  expect(link).toHaveAttribute('target', '_blank')
})

it('reads the video id from every shape a YouTube link is shared in', () => {
  for (const url of [
    'https://youtu.be/AbCdEfGhIj1',
    'https://youtu.be/AbCdEfGhIj1?si=xyz',
    'https://www.youtube.com/watch?v=AbCdEfGhIj1&t=10',
    'https://m.youtube.com/watch?v=AbCdEfGhIj1',
    'https://youtube.com/shorts/AbCdEfGhIj1',
    'https://www.youtube.com/embed/AbCdEfGhIj1',
  ]) {
    expect(youtubeId(url)).toBe('AbCdEfGhIj1')
  }
  for (const url of ['https://example.com/watch?v=AbCdEfGhIj1', 'https://youtu.be/short', 'not a url', 'https://www.youtube.com/@channel']) {
    expect(youtubeId(url)).toBeNull()
  }
})
