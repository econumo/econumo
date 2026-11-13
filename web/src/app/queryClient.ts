import { createAppQueryClient } from '@/lib/appQueryClient'
import { setAnalyticsQueryClient } from '@/lib/analyticsProfile'
import { installTransactionTracking } from '@/features/transactions/revisionTracking'

// The app-wide singleton — importable outside the React tree (the 402
// interceptor invalidates the user query through it).
export const queryClient = createAppQueryClient()

// Lets trackEvent() read the account-profile counts straight out of the
// cache without threading the client through every call site.
setAnalyticsQueryClient(queryClient)

installTransactionTracking(queryClient)

