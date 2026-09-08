import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { ImportCredentialKeyDto } from '@/api/dto/imports'
import { PasswordInput } from '@/components/PasswordInput'
import { Button } from '@/components/ui/button'
import { InfoBox } from '@/components/InfoBox'
import { WrongPassphraseError, forgetKey, unlockKey } from '@/lib/importCrypto'

export function SimpleFINUnlock({ wrapped, onUnlocked, onReconnect }: {
  wrapped: ImportCredentialKeyDto
  onUnlocked: () => void
  onReconnect: () => void
}) {
  const { t } = useTranslation()
  const [passphrase, setPassphrase] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const submit = async () => {
    setBusy(true)
    setError(null)
    try {
      await unlockKey(passphrase, wrapped)
      onUnlocked()
    } catch (err) {
      setError(err instanceof WrongPassphraseError ? t('imports.simplefin.unlock.wrong_passphrase') : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex flex-col gap-3">
      <InfoBox>{t('imports.simplefin.unlock.intro')}</InfoBox>
      <PasswordInput
        aria-label={t('imports.simplefin.unlock.passphrase')} placeholder={t('imports.simplefin.unlock.passphrase')}
        value={passphrase} onChange={(e) => setPassphrase(e.target.value)} autoComplete="current-password"
        onKeyDown={(e) => { if (e.key === 'Enter') void submit() }}
      />
      {error ? <p role="alert" className="text-sm text-destructive">{error}</p> : null}
      <div className="flex flex-wrap gap-2">
        <Button type="button" disabled={busy || !passphrase} onClick={() => void submit()}>{t('imports.simplefin.unlock.submit')}</Button>
        <Button type="button" variant="secondary" onClick={onReconnect}>{t('imports.simplefin.unlock.reconnect')}</Button>
      </div>
      <p className="text-xs text-muted-foreground">{t('imports.simplefin.unlock.forgot_help')}</p>
    </div>
  )
}

// "Forget this device" lives on the page (it is also offered while unlocked).
export async function forgetThisDevice(): Promise<void> {
  await forgetKey()
}
