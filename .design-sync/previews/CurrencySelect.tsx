import { CurrencySelect } from 'web'

// Options come from useCurrencies() (TanStack Query), served by the bundle's
// EconumoPreviewProvider (cfg.provider) with seeded USD/EUR/GBP/JPY/CHF data.
// The id below is the seeded USD row from .design-sync/ds-extras.tsx.
const USD_ID = '0197a5f6-0001-7000-8000-000000000001'

export const SelectedCurrency = () => (
  <div className="w-72">
    <CurrencySelect value={USD_ID} onChange={() => {}} aria-label="Currency" />
  </div>
)

export const EmptyTrigger = () => (
  <div className="w-72">
    <CurrencySelect value={null} onChange={() => {}} aria-label="Currency" />
  </div>
)

export const Disabled = () => (
  <div className="w-72">
    <CurrencySelect value={USD_ID} onChange={() => {}} disabled aria-label="Currency" />
  </div>
)
