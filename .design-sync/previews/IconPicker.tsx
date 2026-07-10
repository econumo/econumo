import { IconPicker } from 'web'

export const AccountIcon = () => (
  <div className="w-80">
    <IconPicker value="shopping_cart" onChange={() => {}} aria-label="Account icon" />
  </div>
)

export const LaterPageSelection = () => (
  <div className="w-80">
    <IconPicker value="festival" onChange={() => {}} aria-label="Category icon" />
  </div>
)
