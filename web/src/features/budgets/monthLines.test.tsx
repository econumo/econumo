import { render, screen } from '@testing-library/react'
import { FolderLine } from './monthLines'

it('an empty folder keeps its three figure columns, so its ⋮ sits where the others do', () => {
  render(<FolderLine name="Bills" folded={false} onToggle={() => {}} sums={null} actionsColumn={false} menu={[{ label: 'Edit', onSelect: () => {} }]} />)
  const menu = screen.getByRole('button', { name: 'menu Bills' })
  const columns = menu.nextElementSibling
  expect(columns?.children).toHaveLength(3)
  expect(columns).toHaveTextContent('')
  expect(screen.queryByTestId('stat-line')).toBeNull()
})
