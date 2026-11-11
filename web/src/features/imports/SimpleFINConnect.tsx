import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CardField, cardFieldControlClass } from '@/components/CardField'
import { Checkbox } from '@/components/ui/checkbox'
import { PasswordInput } from '@/components/PasswordInput'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { useOnline } from '@/hooks/useOnline'
import { apiErrorMessage } from '@/lib/apiError'
import { WrongPassphraseError, createKey, encryptCredential, unlockKey } from '@/lib/importCrypto'
import type { ImportKeyState } from './useImportKey'
import { useClaimSetupToken, useCreateImportSource, useSetImportCredentialKey } from './queries'

const SOURCE_NAME = 'SimpleFIN'

// the form is itself a card, so its fields take the page background
const onCard = 'bg-background'

// The whole connect flow, in order: (1) make sure this device holds the data
// key — create one (first device), unlock the existing one, or, when the
// passphrase is lost, replace it; (2) claim the setup token (the server
// returns the access URL and forgets it); (3) encrypt the URL here and store
// only the ciphertext. The access URL is held in component state and handed
// to the parent — never persisted.
export function SimpleFINConnect({ keyState, onConnected, onCancel, reconnect = false }: {
  keyState: Exclude<ImportKeyState, { status: 'loading' }>
  onConnected: (accessUrl: string) => void
  onCancel?: () => void
  reconnect?: boolean
}) {
  const { t } = useTranslation()
  const online = useOnline()
  const claim = useClaimSetupToken()
  const setKey = useSetImportCredentialKey()
  const create = useCreateImportSource({ handlesOwnErrors: true })
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
    <form
      className="flex flex-col gap-4 rounded-lg bg-econumo-card px-4 py-4 text-sm"
      onSubmit={(e) => {
        e.preventDefault()
        void submit()
      }}
    >
      <p className="text-muted-foreground">{t(reconnect ? 'imports.simplefin.connect.reconnect_intro' : 'imports.simplefin.intro')}</p>
      <div className="flex flex-col gap-1.5">
        <CardField className={onCard} label={t('imports.simplefin.connect.token_label')} htmlFor="simplefin-token">
          <Textarea
            id="simplefin-token" rows={3} value={setupToken} onChange={(e) => setSetupToken(e.target.value)}
            className={`${cardFieldControlClass} min-h-16 resize-none break-all`}
            placeholder={t('imports.simplefin.connect.token_placeholder')}
          />
        </CardField>
        <p className="px-1 text-xs text-muted-foreground">{t('imports.simplefin.connect.token_help')}</p>
      </div>
      {keyState.status === 'locked' && !resetKey ? (
        <label className="flex items-center gap-2">
          <Checkbox checked={resetKey} onCheckedChange={(v) => setResetKey(v === true)} />
          {t('imports.simplefin.connect.forgot_passphrase')}
        </label>
      ) : null}
      {needsPassphrase ? (
        <div className="flex flex-col gap-1.5">
          <CardField
            className={onCard}
            label={t(needsNewKey ? 'imports.simplefin.connect.passphrase_new' : 'imports.simplefin.connect.passphrase_existing')}
            htmlFor="simplefin-passphrase"
          >
            <PasswordInput
              id="simplefin-passphrase" value={passphrase} onChange={(e) => setPassphrase(e.target.value)}
              autoComplete="new-password" className={`${cardFieldControlClass} pr-8`}
            />
          </CardField>
          {needsNewKey ? (
            <>
              <CardField className={onCard} label={t('imports.simplefin.connect.passphrase_confirm')} htmlFor="simplefin-passphrase-confirm">
                <PasswordInput
                  id="simplefin-passphrase-confirm" value={confirm} onChange={(e) => setConfirm(e.target.value)}
                  autoComplete="new-password" className={`${cardFieldControlClass} pr-8`}
                />
              </CardField>
              <p className="px-1 text-xs text-muted-foreground">{t('imports.simplefin.connect.passphrase_help')}</p>
            </>
          ) : null}
        </div>
      ) : null}
      {error ? <p role="alert" className="text-destructive">{error}</p> : null}
      <div className="flex flex-col gap-2 sm:flex-row-reverse sm:justify-start">
        <Button type="submit" className="h-11 sm:min-w-40" disabled={busy || !online}>
          {t(reconnect ? 'imports.simplefin.connect.submit_reconnect' : 'imports.simplefin.connect.submit')}
        </Button>
        {onCancel ? <Button type="button" variant="secondary" className="h-11" onClick={onCancel}>{t('common.button.cancel.label')}</Button> : null}
      </div>
    </form>
  )
}
