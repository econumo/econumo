import { useMutation } from '@tanstack/react-query'
import { toast } from 'sonner'
import { createBillingLink } from '@/api/user'
import type { Id } from '@/api/types'
import { apiErrorMessage } from '@/lib/apiError'
import { METRICS, trackEvent } from '@/lib/metrics'

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
          trackEvent(forUserId ? METRICS.ACCESS_PARTNER_CTA_CLICK : METRICS.ACCESS_CTA_CLICK)
          if (tab) {
            tab.location.href = url
          } else {
            window.location.assign(url)
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
