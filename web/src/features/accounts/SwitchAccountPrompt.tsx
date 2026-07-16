import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router'
import { useUiStore } from '@/app/uiStore'
import { RouterPage } from '@/app/router-pages'
import { useAccounts } from './queries'

// The post-transfer bottom prompt: "Switch to <recipient account>".
export function SwitchAccountPrompt() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const accountId = useUiStore((s) => s.switchAccountPrompt)
  const setPrompt = useUiStore((s) => s.setSwitchAccountPrompt)
  const { data: accounts } = useAccounts()

  if (!accountId) {
    return null
  }
  const account = accounts?.find((a) => a.id === accountId)
  if (!account) {
    return null
  }

  return (
    <div className="fixed inset-x-0 bottom-0 z-50 flex items-center justify-center gap-2 border-t bg-background p-3 pb-[max(env(safe-area-inset-bottom),0.75rem)] shadow-lg">
      <button
        type="button"
        className="flex items-center gap-1 text-sm"
        onClick={() => {
          setPrompt(null)
          navigate(RouterPage.ACCOUNT(account.id))
        }}
      >
        {t('accounts.switch_to_account')} <strong>{account.name}</strong>
      </button>
      <button type="button" aria-label="close" className="text-muted-foreground hover:text-foreground" onClick={() => setPrompt(null)}>
        <X className="size-4" />
      </button>
    </div>
  )
}
