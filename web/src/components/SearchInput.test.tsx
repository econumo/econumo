import { useState } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SearchInput } from './SearchInput'

function Field() {
  const [value, setValue] = useState('')
  return <SearchInput aria-label="Search" value={value} onChange={(e) => setValue(e.target.value)} onClear={() => setValue('')} />
}

it('shows a clear button only while the field has text', async () => {
  const user = userEvent.setup()
  render(<Field />)
  expect(screen.queryByRole('button', { name: 'Clear search' })).not.toBeInTheDocument()
  await user.type(screen.getByRole('textbox', { name: 'Search' }), 'coffee')
  expect(screen.getByRole('button', { name: 'Clear search' })).toBeInTheDocument()
})

it('clears the field in one tap and keeps the cursor in it', async () => {
  const user = userEvent.setup()
  render(<Field />)
  const field = screen.getByRole('textbox', { name: 'Search' })
  await user.type(field, 'coffee')
  await user.click(screen.getByRole('button', { name: 'Clear search' }))
  expect(field).toHaveValue('')
  expect(field).toHaveFocus()
  expect(screen.queryByRole('button', { name: 'Clear search' })).not.toBeInTheDocument()
})
