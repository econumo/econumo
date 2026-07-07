import { render, screen } from '@testing-library/react'
import { SortableList, orderByPreview } from './SortableList'

const items = [
  { id: 'a', name: 'Alpha' },
  { id: 'b', name: 'Beta' },
  { id: 'c', name: 'Gamma' },
]

// jsdom has no layout, so real dnd-kit drag simulation is not possible here;
// pages drive onReorder directly in their tests, and the reorder math is
// covered by lib/ordering tests.
it('renders all rows with their drag handles wired', () => {
  render(
    <SortableList
      items={items}
      onReorder={() => {}}
      renderItem={(item, handle) => (
        <div>
          <button aria-label={`drag ${item.id}`} {...handle.attributes} {...(handle.listeners ?? {})}>
            ::
          </button>
          <span>{item.name}</span>
        </div>
      )}
    />,
  )
  expect(screen.getByText('Alpha')).toBeInTheDocument()
  expect(screen.getByText('Gamma')).toBeInTheDocument()
  // sortable handles get role/tabindex attributes from dnd-kit
  expect(screen.getByLabelText('drag a')).toHaveAttribute('aria-roledescription', 'sortable')
})

it('disabled mode renders plain rows without dnd wiring', () => {
  render(
    <SortableList
      disabled
      items={items}
      onReorder={() => {}}
      renderItem={(item, handle) => (
        <div>
          <button aria-label={`drag ${item.id}`} {...handle.attributes} {...(handle.listeners ?? {})}>
            ::
          </button>
          <span>{item.name}</span>
        </div>
      )}
    />,
  )
  expect(screen.getByLabelText('drag a')).not.toHaveAttribute('aria-roledescription')
})

it('orderByPreview keeps the dropped order, drops vanished ids, appends unknown items', () => {
  expect(orderByPreview(items, null)).toEqual(items)
  expect(orderByPreview(items, ['c', 'a', 'b']).map((i) => i.id)).toEqual(['c', 'a', 'b'])
  expect(orderByPreview(items, ['c', 'gone', 'a']).map((i) => i.id)).toEqual(['c', 'a', 'b'])
})
