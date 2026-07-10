// KNOWN GAP (see learnings/wave2-b4.md): the bundle's i18next instance is
// uninitialized and unreachable from preview code, so the visible caption
// (t('elements.loader.label') -> "loading") renders as a raw key until the
// bundle gains an i18n bootstrap. Coins + wordmark render correctly.
import { CoinLoader } from 'web'

// Full-page loading state (BudgetPage): three gold coins bounce over the
// econumo wordmark; label is the screen-reader description.
export const Loading = () => (
  <div className="flex w-full justify-center py-8">
    <CoinLoader label="Loading budget data" />
  </div>
)

export const CenteredInPanel = () => (
  <div className="flex h-56 w-96 items-center justify-center rounded-lg border bg-card">
    <CoinLoader label="Loading budget data" />
  </div>
)
