import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Checkbox } from '@/components/ui/checkbox'
import { PasswordInput } from '@/components/PasswordInput'
import { Button } from '@/components/ui/button'
import { InfoBox } from '@/components/InfoBox'
import { apiErrorMessage } from '@/lib/apiError'
import { WrongPassphraseError, createKey, encryptCredential, unlockKey } from '@/lib/importCrypto'
import type { ImportKeyState } from './useImportKey'
import { useClaimSetupToken, useCreateImportSource, useSetImportCredentialKey } from './queries'

const SOURCE_NAME = 'SimpleFIN'

// The whole connect flow, in order: (1) make sure this device holds the data
// key — create one (first device), unlock the existing one, or, when the
// passphrase is lost, replace it; (2) claim the setup token (the server
// returns the access URL and forgets it); (3) encrypt the URL here and store
// only the ciphertext. The access URL is held in component state and handed
// to the parent — never persisted.
export function SimpleFINConnect({ keyState, onConnected, reconnect = false }: {
  keyState: Exclude<ImportKeyState, { status: 'loading' }>
  onConnected: (accessUrl: string) => void
  reconnect?: boolean
}) {
  const { t } = useTranslation()
  const claim = useClaimSetupToken()
  const setKey = useSetImportCredentialKey()
  const create = useCreateImportSource()
  const [setupToken, setSetupToken] = useState('')
  const [passphrase, setPassphrase] = useState('')
  const [confirm, setConfirm] = useState('')
  const [resetKey, setResetKey] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const needsNewKey = keyState.status === 'none' || (keyState.status === 'locked' && resetKey)
  const needsPassphrase = keyState.status !== 'unlocked' || needsNewKey

  const submit = async () => {
    setError(null)
    if (!setupToken.trim()) {
      setError(t('imports.simplefin.connect.errors.token_required'))
      return
    }
    if (needsPassphrase && passphrase.length < 8) {
      setError(t('imports.simplefin.connect.errors.passphrase_short'))
      return
    }
    if (needsNewKey && passphrase !== confirm) {
      setError(t('imports.simplefin.connect.errors.passphrase_mismatch'))
      return
    }
    setBusy(true)
    try {
      if (needsNewKey) {
        await setKey.mutateAsync(await createKey(passphrase))
      } else if (keyState.status === 'locked') {
        await unlockKey(passphrase, keyState.wrapped)
      }
      const accessUrl = await claim.mutateAsync(setupToken.trim())
      const credentialCiphertext = await encryptCredential(accessUrl)
      await create.mutateAsync({ provider: 'simplefin', name: SOURCE_NAME, credentialCiphertext })
      onConnected(accessUrl)
    } catch (err) {
      setError(err instanceof WrongPassphraseError ? t('imports.simplefin.unlock.wrong_passphrase') : apiErrorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex flex-col gap-3">
      <InfoBox>{t(reconnect ? 'imports.simplefin.connect.reconnect_intro' : 'imports.simplefin.connect.intro')}</InfoBox>
      <label className="text-xs uppercase text-muted-foreground" htmlFor="simplefin-token">{t('imports.simplefin.connect.token_label')}</label>
      <textarea
        id="simplefin-token" rows={3} value={setupToken} onChange={(e) => setSetupToken(e.target.value)}
        className="rounded-lg border bg-background px-3 py-2 text-sm"
        placeholder={t('imports.simplefin.connect.token_placeholder')}
      />
      <p className="text-xs text-muted-foreground">{t('imports.simplefin.connect.token_help')}</p>
      {keyState.status === 'locked' && !resetKey ? (
        <label className="flex items-center gap-2 text-sm">
          <Checkbox checked={resetKey} onCheckedChange={(v) => setResetKey(v === true)} />
          {t('imports.simplefin.connect.forgot_passphrase')}
        </label>
      ) : null}
      {needsPassphrase ? (
        <>
          <label className="text-xs uppercase text-muted-foreground" htmlFor="simplefin-passphrase">
            {t(needsNewKey ? 'imports.simplefin.connect.passphrase_new' : 'imports.simplefin.connect.passphrase_existing')}
          </label>
          <PasswordInput id="simplefin-passphrase" value={passphrase} onChange={(e) => setPassphrase(e.target.value)} autoComplete="new-password" />
          {needsNewKey ? (
            <>
              <PasswordInput
                aria-label={t('imports.simplefin.connect.passphrase_confirm')} placeholder={t('imports.simplefin.connect.passphrase_confirm')}
                value={confirm} onChange={(e) => setConfirm(e.target.value)} autoComplete="new-password"
              />
              <p className="text-xs text-muted-foreground">{t('imports.simplefin.connect.passphrase_help')}</p>
            </>
          ) : null}
        </>
      ) : null}
      {error ? <p role="alert" className="text-sm text-destructive">{error}</p> : null}
      <Button type="button" className="w-full sm:w-auto" disabled={busy} onClick={() => void submit()}>
        {t(reconnect ? 'imports.simplefin.connect.submit_reconnect' : 'imports.simplefin.connect.submit')}
      </Button>
    </div>
  )
}
