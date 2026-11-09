import { useState, type MouseEvent } from 'react'
import { useNavigate } from 'react-router'
import { useTranslation } from 'react-i18next'
import { RouterPage } from '@/app/router-pages'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { useSignOutConfirm } from '@/features/pendingWrites/hooks'

/** For a plain link to the logout route, which clears the queue of unsent changes:
 *  `askFirst` holds the navigation back whenever there is something to warn about. */
export function useSignOutLink() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const signOut = useSignOutConfirm(t('auth.sign_out.question'))
  const [confirming, setConfirming] = useState(false)
  const askFirst = (e: MouseEvent) => {
    if (signOut.mustAsk) {
      e.preventDefault()
      setConfirming(true)
    }
  }
  const confirm = (
    <ConfirmDialog
      open={confirming}
      onClose={() => setConfirming(false)}
      onConfirm={() => void navigate(RouterPage.LOGOUT)}
      title={t('auth.sign_out.title')}
      question={signOut.question}
      confirmLabel={t('auth.sign_out.action.logout')}
      cancelLabel={t('auth.sign_out.action.cancel')}
      confirmDisabled={signOut.confirmDisabled}
      offlineOk
    />
  )
  return { askFirst, confirm, confirming }
}
