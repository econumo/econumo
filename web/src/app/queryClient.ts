import { createAppQueryClient } from '@/lib/queryPersist'
import { setAnalyticsQueryClient } from '@/lib/analyticsProfile'

// The app-wide singleton — importable outside the React tree (the 402
// interceptor invalidates the user query through it).
export const queryClient = createAppQueryClient()

// Lets trackEvent() read the account-profile counts straight out of the
// cache without threading the client through every call site.
setAnalyticsQueryClient(queryClient)

// The analytics group ($group_id/$group_name) is resolved lazily inside
// trackEvent() (web/src/lib/metrics.ts), not here: in the mobile app this
// module evaluates before fetchServerConfig() merges the real INSTANCE_ID,
// so a fixed module-scope call would never see it.
