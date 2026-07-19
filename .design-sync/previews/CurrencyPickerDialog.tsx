// KNOWN GAP (see learnings/wave2-b4.md): the bundle's i18next instance is
// uninitialized and unreachable from preview code, so the component's own
// t() strings (the search placeholder) render as raw keys until the bundle
// gains an i18n bootstrap. Everything else below is real.
import { CurrencyPickerDialog } from 'web'

// Currency options come from useCurrencies(), served by the bundle's
// EconumoPreviewProvider (seeded USD/EUR/GBP/JPY/CHF). USD id below is the
// seeded row from .design-sync/ds-extras.tsx.
const USD_ID = '0197a5f6-0001-7000-8000-000000000001'

export const PickCurrency = () => (
  <CurrencyPickerDialog open title="Currency" value={USD_ID} onClose={() => {}} onPick={() => {}} />
)

export const NoSelection = () => (
  <CurrencyPickerDialog open title="Currency" value={null} onClose={() => {}} onPick={() => {}} />
)
