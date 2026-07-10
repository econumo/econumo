import { Toggle } from 'web'
import { Archive, Star } from 'lucide-react'

export const Pressed = () => (
  <div className="flex flex-wrap items-center gap-3">
    <Toggle aria-label="Favorite account">
      <Star />
      Favorite
    </Toggle>
    <Toggle defaultPressed aria-label="Favorite account, pressed">
      <Star />
      Favorite
    </Toggle>
  </div>
)

export const OutlineVariant = () => (
  <div className="flex flex-wrap items-center gap-3">
    <Toggle variant="outline" aria-label="Show archived">
      <Archive />
      Show archived
    </Toggle>
    <Toggle variant="outline" defaultPressed aria-label="Show archived, pressed">
      <Archive />
      Show archived
    </Toggle>
  </div>
)

export const Sizes = () => (
  <div className="flex flex-wrap items-center gap-3">
    <Toggle size="sm" variant="outline" defaultPressed>
      Expenses
    </Toggle>
    <Toggle size="default" variant="outline" defaultPressed>
      Expenses
    </Toggle>
    <Toggle size="lg" variant="outline" defaultPressed>
      Expenses
    </Toggle>
  </div>
)

export const Disabled = () => (
  <div className="flex flex-wrap items-center gap-3">
    <Toggle disabled>
      <Star />
      Favorite
    </Toggle>
    <Toggle variant="outline" disabled defaultPressed>
      <Archive />
      Show archived
    </Toggle>
  </div>
)
