// Bundled into _ds_bundle.js via cfg.extraEntries. Data-dependent Econumo
// components (CurrencySelect, CurrencyPickerDialog) call TanStack Query hooks;
// react-query lives INSIDE the bundle, so the provider must too — a
// QueryClientProvider imported by a preview from node_modules is a different
// module instance and its context never reaches the bundle's useQueryClient.
// EconumoPreviewProvider wraps every preview (cfg.provider) with a client
// seeded from realistic static data — no network, no retries.
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { queryKeys } from '../web/src/app/queryKeys'
// Initializes the BUNDLE's react-i18next default instance (module-scoped, so
// a preview-side init can never reach it) — CoinLoader/LoadingDialog/
// FailDialog/SortDialog t() calls resolve instead of showing raw keys.
import '../web/src/app/i18n'
// sonner is inlined in the bundle too — previews (and the design agent) need
// the SAME module's toast() for the real <Toaster/> to receive toasts.
export { toast } from 'sonner'
// recharts is inlined too — ChartContainer/ChartTooltipContent only work with
// chart primitives from the SAME instance, and the design agent has no other
// way to reach them. Cards are suppressed via componentSrcMap nulls.
export {
  Area, AreaChart, Bar, BarChart, CartesianGrid, Cell, Line, LineChart,
  Pie, PieChart, RadialBar, RadialBarChart, ResponsiveContainer, XAxis, YAxis,
} from 'recharts'

const CURRENCIES = [
  { id: '0197a5f6-0001-7000-8000-000000000001', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 },
  { id: '0197a5f6-0001-7000-8000-000000000002', code: 'EUR', name: 'Euro', symbol: '€', fractionDigits: 2 },
  { id: '0197a5f6-0001-7000-8000-000000000003', code: 'GBP', name: 'British Pound', symbol: '£', fractionDigits: 2 },
  { id: '0197a5f6-0001-7000-8000-000000000004', code: 'JPY', name: 'Japanese Yen', symbol: '¥', fractionDigits: 0 },
  { id: '0197a5f6-0001-7000-8000-000000000005', code: 'CHF', name: 'Swiss Franc', symbol: '₣', fractionDigits: 2 },
]

// CurrentUserDto shape (web/src/api/dto/user.ts) — AvatarPickerDialog and
// other useUserData() consumers read it; avatar is the "<icon>:<color>" value.
const USER = {
  id: '0197a5f6-0002-7000-8000-000000000001',
  name: 'Anna Kovaleva',
  email: 'anna.kovaleva@fastmail.com',
  avatar: 'owl:teal',
  options: [],
  currency: 'USD',
  reportPeriod: 'month',
}

const RATES = CURRENCIES.map((c, i) => ({
  currencyId: c.id,
  baseCurrencyId: CURRENCIES[0].id,
  rate: [1, 0.92, 0.79, 157.4, 0.89][i] ?? 1,
  updatedAt: '2026-07-01 00:00:00',
}))

export function EconumoPreviewProvider({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity, gcTime: Infinity } },
  })
  client.setQueryData(queryKeys.currencies, CURRENCIES)
  client.setQueryData(queryKeys.currencyRates, RATES)
  client.setQueryData(queryKeys.user, USER)
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>
}
