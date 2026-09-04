import { createAppQueryClient } from '@/lib/queryPersist'
import { setAnalyticsQueryClient } from '@/lib/analyticsProfile'
import { setAnalyticsGroup } from '@/lib/analytics'
import { analyticsHost } from '@/lib/metrics'
import { getInstanceId } from '@/lib/config'

// The app-wide singleton — importable outside the React tree (the 402
// interceptor invalidates the user query through it).
export const queryClient = createAppQueryClient()

// Lets trackEvent() read the account-profile counts straight out of the
// cache without threading the client through every call site.
setAnalyticsQueryClient(queryClient)

// The group identifies the deployment, not a person, so it is set once at
// boot rather than per-user; an unconfigured instance id skips the call
// rather than registering an "unknown" group.
const instanceId = getInstanceId()
if (instanceId) {
  setAnalyticsGroup(instanceId, analyticsHost())
}
