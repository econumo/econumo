import { act, render, screen } from '@testing-library/react'
import { onlineManager } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { ClassificationList } from './ClassificationList'

function mockViewport() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

// The archived badge must come from the caller (each list has its own catalogue
// key — languages inflect the word differently per noun), never from a
// hard-coded categories key.
it('renders the caller-supplied archived label on archived rows', () => {
  mockViewport()
  const router = createMemoryRouter(
    [{
      path: '/x',
      element: (
        <ClassificationList
          title="Payees"
          createLabel="Create payee"
          deleteTitle="Delete payee?"
          analyticsType="payee"
          archivedLabel="Arquivados"
          items={[
            { id: 'p1', name: 'Grocer', position: 0, isArchived: 1 },
            { id: 'p2', name: 'Butcher', position: 1, isArchived: 0 },
          ]}
          onCreate={() => {}}
          onEdit={() => {}}
          onDelete={() => {}}
        />
      ),
    }],
    { initialEntries: ['/x'] },
  )
  render(<RouterProvider router={router} />)
  expect(screen.getByText('Grocer')).toBeInTheDocument()
  expect(screen.getByText('Arquivados')).toBeInTheDocument()
  // the categories catalogue string must not leak in
  expect(screen.queryByText('Archived')).not.toBeInTheDocument()
})

it('offline: create, reorder and row actions are disabled', () => {
  mockViewport()
  onlineManager.setOnline(false)
  onTestFinished(() => act(() => onlineManager.setOnline(true)))
  const router = createMemoryRouter(
    [{
      path: '/x',
      element: (
        <ClassificationList
          title="Payees"
          createLabel="Create payee"
          deleteTitle="Delete payee?"
          analyticsType="payee"
          archivedLabel="Archived"
          items={[
            { id: 'p1', name: 'Grocer', position: 0, isArchived: 0 },
            { id: 'p2', name: 'Butcher', position: 1, isArchived: 0 },
          ]}
          onCreate={() => {}}
          onEdit={() => {}}
          onDelete={() => {}}
          onMove={() => {}}
          onSort={() => {}}
        />
      ),
    }],
    { initialEntries: ['/x'] },
  )
  render(<RouterProvider router={router} />)
  expect(screen.getByRole('button', { name: /Create payee/ })).toBeDisabled()
  expect(screen.getByRole('button', { name: /order/i })).toBeDisabled()
  expect(screen.queryByRole('button', { name: 'drag Grocer' })).toBeNull()
})
