import { ServerVersionNotice, useServerVersionNotice } from '@/components/ServerVersionNotice'
import { SubscriptionBanner, useSubscriptionBanner } from '@/features/access/SubscriptionBanner'
import { InstallBanner } from '@/features/install/InstallBanner'

// One top banner at a time, the most urgent first: access, then an outdated
// server, then the install nudge.
export function TopBanner() {
  const subscription = useSubscriptionBanner()
  const serverVersion = useServerVersionNotice()
  const subscriptionShown = subscription.variant !== null
  return (
    <>
      <SubscriptionBanner banner={subscription} />
      <ServerVersionNotice serverVersion={subscriptionShown ? null : serverVersion} />
      <InstallBanner hidden={subscriptionShown || serverVersion !== null} />
    </>
  )
}
