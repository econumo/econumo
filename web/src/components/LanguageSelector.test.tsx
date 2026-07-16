import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18n from '@/app/i18n'
import { locale } from '@/lib/config'
import { LanguageSelector } from './LanguageSelector'

// Radix's DropdownMenuTrigger opens on a real pointer sequence, not a bare
// "click" event, so this drives it with userEvent (as the rest of the
// codebase's menu-interaction tests do) rather than fireEvent.
it('switches language, persists it, and updates <html lang>', async () => {
  const user = userEvent.setup()
  render(<LanguageSelector />)
  await user.click(screen.getByRole('button', { name: /language/i }))
  await user.click(await screen.findByText('Русский'))
  await waitFor(() => expect(i18n.language).toBe('ru'))
  expect(locale()).toBe('ru')
  expect(document.documentElement.lang).toBe('ru')
  await user.click(screen.getByRole('button', { name: /язык|language/i }))
  await user.click(await screen.findByText('English'))
  await waitFor(() => expect(i18n.language).toBe('en'))
})
