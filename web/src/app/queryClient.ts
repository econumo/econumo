import { createAppQueryClient } from '@/lib/queryPersist'
import { setAnalyticsQueryClient } from '@/lib/analyticsProfile'

// The app-wide singleton — importable outside the React tree (the 402
// interceptor invalidates the user query through it).
export const queryClient = createAppQueryClient()

// Lets trackEvent() read the account-profile counts straight out of the
// cache without threading the client through every call site.
setAnalyticsQueryClient(queryClient)
