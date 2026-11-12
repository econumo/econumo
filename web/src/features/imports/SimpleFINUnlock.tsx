import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { ImportCredentialKeyDto } from '@/api/dto/imports'
import { CardField, cardFieldControlClass } from '@/components/CardField'
import { PasswordInput } from '@/components/PasswordInput'
import { Button } from '@/components/ui/button'
import { apiErrorMessage } from '@/lib/apiError'
import { WrongPassphraseError, unlockKey } from '@/lib/importCrypto'

// the form is itself a card, so its field takes the page background
const onCard = 'bg-background'

export function SimpleFINUnlock({ wrapped, stale, onUnlocked, onReconnect }: {
  wrapped: ImportCredentialKeyDto
  stale: boolean
  onUnlocked: () => void
  onReconnect: () => void
}) {
  const { t } = useTranslation()
  const [passphrase, setPassphrase] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const submit = async () => {
    if (!passphrase) {
      return
    }
    setBusy(true)
    setError(null)
    try {
      await unlockKey(passphrase, wrapped)
      onUnlocked()
    } catch (err) {
      setError(err instanceof WrongPassphraseError ? t('imports.simplefin.unlock.wrong_passphrase') : apiErrorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form
      className="flex flex-col gap-4 rounded-lg bg-econumo-card px-4 py-4 text-sm"
      onSubmit={(e) => {
        e.preventDefault()
        void submit()
      }}
    >
      <p className="text-muted-foreground">{t(stale ? 'imports.simplefin.unlock.stale_intro' : 'imports.simplefin.unlock.intro')}</p>
      <div>
        <CardField className={onCard} label={t('imports.simplefin.unlock.passphrase')} htmlFor="simplefin-unlock">
          <PasswordInput
            id="simplefin-unlock" value={passphrase} onChange={(e) => setPassphrase(e.target.value)}
            autoComplete="current-password" className={`${cardFieldControlClass} pr-8`}
          />
        </CardField>
      </div>
      {error ? <p role="alert" className="text-destructive">{error}</p> : null}
      <div className="flex flex-col gap-2 sm:flex-row-reverse sm:justify-start">
        <Button type="submit" className="h-11 sm:min-w-40" disabled={busy || !passphrase}>{t('imports.simplefin.unlock.submit')}</Button>
        <Button type="button" variant="secondary" className="h-11" onClick={onReconnect}>{t('imports.simplefin.unlock.reconnect')}</Button>
      </div>
      <p className="text-xs text-muted-foreground">{t('imports.simplefin.unlock.forgot_help')}</p>
    </form>
  )
}
