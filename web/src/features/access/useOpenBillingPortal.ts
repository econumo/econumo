import { useMutation } from '@tanstack/react-query'
import { toast } from 'sonner'
import { createBillingLink } from '@/api/user'
import type { Id } from '@/api/types'
import { apiErrorMessage } from '@/lib/apiError'
import { METRICS, billingAnalyticsFragment, trackEvent } from '@/lib/metrics'

// The one place that opens the billing portal (banner, settings, connections).
// window.open must happen synchronously inside the click gesture or popup
// blockers eat it — open a blank tab first, then point it at the minted link.
export function useOpenBillingPortal() {
  const mutation = useMutation({ mutationFn: (forUserId?: Id) => createBillingLink(forUserId) })
  return {
    pending: mutation.isPending,
    open: (forUserId?: Id) => {
      const tab = window.open('', '_blank')
      mutation.mutate(forUserId, {
        onSuccess: (url) => {
          trackEvent(METRICS.SUBSCRIPTION_CTA_CLICK, { target: forUserId ? 'partner' : 'self' })
          // The portal reports as this same analytics user (see
          // billingAnalyticsFragment); a fragment, so no server sees it.
          const fragment = billingAnalyticsFragment()
          const target = fragment && !url.includes('#') ? `${url}#${fragment}` : url
          if (tab) {
            tab.location.href = target
          } else {
            window.location.assign(target)
          }
        },
        onError: (err) => {
          tab?.close()
          toast.error(apiErrorMessage(err))
        },
      })
    },
  }
}
