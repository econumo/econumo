import { screen, within } from '@testing-library/react'
import type { UserEvent } from '@testing-library/user-event'

type Field = 'accounts' | 'savings'

/** opens the Accounts or Savings accounts picker of a budget form */
export async function openPicker(user: UserEvent, field: Field): Promise<HTMLElement> {
  await user.click(await screen.findByTestId(`budget-${field}-field`))
  return screen.findByRole('dialog', { name: field === 'accounts' ? 'Accounts' : 'Savings accounts' })
}

/** toggles each named option (its aria-label) in a picker, then confirms with OK */
export async function toggleInPicker(user: UserEvent, field: Field, ...labels: string[]): Promise<void> {
  const dialog = await openPicker(user, field)
  for (const label of labels) {
    await user.click(within(dialog).getByRole('option', { name: label }))
  }
  await user.click(within(dialog).getByRole('button', { name: 'OK' }))
}
