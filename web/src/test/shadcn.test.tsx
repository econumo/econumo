import { render, screen } from '@testing-library/react'
import { Button } from '@/components/ui/button'

it('renders a shadcn button', () => {
  render(<Button>Save</Button>)
  expect(screen.getByRole('button', { name: 'Save' })).toBeInTheDocument()
})
