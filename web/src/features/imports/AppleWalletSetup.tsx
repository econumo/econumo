import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Check, Copy } from 'lucide-react'
import { toast } from 'sonner'
import type { ImportSourceDto } from '@/api/dto/imports'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { InfoBox } from '@/components/InfoBox'
import { Button } from '@/components/ui/button'
import { useCreatePersonalToken } from '@/features/settings/security'
import { apiErrorMessage } from '@/lib/apiError'
import { copyText } from '@/lib/clipboard'
import { backendHost } from '@/lib/config'
import { METRICS, trackEvent } from '@/lib/metrics'
import { isIOS } from '@/lib/platform'
import { useCreateImportSource, useDeleteImportSource } from './queries'

export const WALLET_SHORTCUT_URL = '/shortcuts/econumo-wallet-v1.shortcut'
export const SETUP_SHORTCUT_URL = '/shortcuts/econumo-setup-v1.shortcut'
const INGEST_TOKEN_NAME = 'Apple Wallet'

// The Setup shortcut takes one text input: JSON with the server URL and the
// ingest token. Encoded once here so the deep link and the manual recipe
// never disagree about the shape.
export function setupDeepLink(url: string, token: string): string {
  const input = encodeURIComponent(JSON.stringify({ url, token }))
  return `shortcuts://run-shortcut?name=Econumo%20Setup&input=text&text=${input}`
}

// Indirected through a stable object (not a bare function export) so tests
// can vi.spyOn it without redefining window.location — a redefine that
// breaks other code's window.location.href reads in jsdom.
export const nav = {
  openDeepLink(url: string) {
    window.location.href = url
  },
}

const REQUEST_BODY = `{
  "account": "<Card name>",
  "payee": "<Merchant>",
  "amount": "<Amount>",
  "currency": "<Currency code>",
  "occurredAt": "<Date, ISO 8601>",
  "eventId": "<Transaction identifier>"
}`

export function AppleWalletSetup({ source }: { source: ImportSourceDto | null }) {
  const { t } = useTranslation()
  const createSource = useCreateImportSource()
  const deleteSource = useDeleteImportSource()
  const createToken = useCreatePersonalToken()
  const [disconnectOpen, setDisconnectOpen] = useState(false)
  const [manualOpen, setManualOpen] = useState(false)
  // The raw token is shown exactly once (the API never returns it again), so
  // it lives only in component state and dies with the page.
  const [token, setToken] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  const [configured, setConfigured] = useState(false)
  const serverUrl = backendHost()

  // Both callers need the same "mint once, reuse" token, and both need the
  // same failure behavior (toast, no half-finished UI) — centralized here.
  const mintToken = async (): Promise<string | null> => {
    try {
      const created = await createToken.mutateAsync({ name: INGEST_TOKEN_NAME, expiresAt: null, scope: 'ingest' })
      setToken(created.token)
      return created.token
    } catch (err) {
      toast.error(apiErrorMessage(err))
      return null
    }
  }

  const configureHere = async () => {
    const value = token ?? (await mintToken())
    if (!value) {
      return
    }
    trackEvent(METRICS.IMPORT_SHORTCUT_CONFIGURE)
    setConfigured(true)
    nav.openDeepLink(setupDeepLink(serverUrl, value))
  }

  const revealManual = async () => {
    if (token) {
      setManualOpen(true)
      return
    }
    // Only open the panel once the token mint actually succeeds — otherwise
    // it's stuck showing the '…' placeholder forever.
    const value = await mintToken()
    if (value) {
      setManualOpen(true)
    }
  }

  if (!source) {
    return (
      <div className="flex flex-col gap-3 rounded-lg bg-econumo-card px-4 py-3.5 text-sm">
        <p className="text-muted-foreground">{t('imports.apple_wallet.intro')}</p>
        <Button type="button" className="h-11" disabled={createSource.isPending} onClick={() => createSource.mutate({ provider: 'apple-wallet', name: 'iPhone' })}>
          {t('imports.apple_wallet.connect')}
        </Button>
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-4 rounded-lg bg-econumo-card px-4 py-3.5 text-sm">
      <div className="flex items-center justify-between gap-2">
        <span className="font-medium">{t('imports.apple_wallet.connected')}</span>
        <Button type="button" variant="ghost" size="sm" onClick={() => setDisconnectOpen(true)}>
          {t('imports.apple_wallet.disconnect')}
        </Button>
      </div>

      <ol className="flex flex-col gap-4">
        <li className="flex flex-col gap-1">
          <span className="font-medium">{t('imports.apple_wallet.steps.install.title')}</span>
          <span className="text-muted-foreground">{t('imports.apple_wallet.steps.install.text')}</span>
          <div className="flex flex-wrap gap-2 pt-1">
            <a href={WALLET_SHORTCUT_URL} download onClick={() => trackEvent(METRICS.IMPORT_SHORTCUT_DOWNLOAD, { shortcut: 'wallet' })} className="rounded-md border px-3 py-2">
              {t('imports.apple_wallet.steps.install.wallet')}
            </a>
            <a href={SETUP_SHORTCUT_URL} download onClick={() => trackEvent(METRICS.IMPORT_SHORTCUT_DOWNLOAD, { shortcut: 'setup' })} className="rounded-md border px-3 py-2">
              {t('imports.apple_wallet.steps.install.setup')}
            </a>
          </div>
        </li>

        <li className="flex flex-col gap-1">
          <span className="font-medium">{t('imports.apple_wallet.steps.configure.title')}</span>
          {isIOS() ? (
            <>
              <span className="text-muted-foreground">{t('imports.apple_wallet.steps.configure.text')}</span>
              <Button type="button" className="mt-1 h-11" disabled={createToken.isPending} onClick={() => void configureHere()}>
                {t('imports.apple_wallet.steps.configure.button')}
              </Button>
              {configured ? <InfoBox>{t('imports.apple_wallet.steps.configure.done')}</InfoBox> : null}
            </>
          ) : (
            <span className="text-muted-foreground">{t('imports.apple_wallet.steps.configure.desktop')}</span>
          )}
          <button type="button" className="self-start text-left text-primary underline-offset-2 hover:underline" onClick={() => void revealManual()}>
            {t('imports.apple_wallet.manual.toggle')}
          </button>
          {manualOpen ? (
            <div className="mt-2 flex flex-col gap-2 rounded-md border p-3">
              <label className="text-xs uppercase text-muted-foreground">{t('imports.apple_wallet.manual.token')}</label>
              <div className="flex items-center gap-2">
                <code className="min-w-0 flex-1 break-all text-xs">{token ?? '…'}</code>
                {token ? (
                  <Button type="button" variant="ghost" size="icon" aria-label={t('user.page.settings.profile.tokens.created_dialog.copy')} onClick={() => void copyText(token).then(setCopied)}>
                    {copied ? <Check className="size-4" /> : <Copy className="size-4" />}
                  </Button>
                ) : null}
              </div>
              <label className="text-xs uppercase text-muted-foreground">{t('imports.apple_wallet.manual.url')}</label>
              <code className="break-all text-xs">{serverUrl}</code>
              <label className="text-xs uppercase text-muted-foreground">{t('imports.apple_wallet.manual.body')}</label>
              <pre className="overflow-x-auto text-xs">{REQUEST_BODY}</pre>
              <ol className="list-decimal pl-4 text-muted-foreground">
                <li>{t('imports.apple_wallet.manual.action_1')}</li>
                <li>{t('imports.apple_wallet.manual.action_2', { url: serverUrl })}</li>
                <li>{t('imports.apple_wallet.manual.action_3')}</li>
                <li>{t('imports.apple_wallet.manual.action_4')}</li>
                <li>{t('imports.apple_wallet.manual.action_5')}</li>
              </ol>
            </div>
          ) : null}
        </li>

        <li className="flex flex-col gap-1">
          <span className="font-medium">{t('imports.apple_wallet.steps.automate.title')}</span>
          <span className="text-muted-foreground">{t('imports.apple_wallet.steps.automate.text')}</span>
        </li>
      </ol>

      <InfoBox>{t('imports.apple_wallet.same_named_cards')}</InfoBox>

      <ConfirmDialog
        open={disconnectOpen}
        onClose={() => setDisconnectOpen(false)}
        onConfirm={() => {
          setDisconnectOpen(false)
          deleteSource.mutate(source.id)
        }}
        title={t('imports.apple_wallet.disconnect_modal.title')}
        question={t('imports.apple_wallet.disconnect_modal.question')}
        confirmLabel={t('imports.apple_wallet.disconnect')}
        cancelLabel={t('common.button.cancel.label')}
        destructive
      />
    </div>
  )
}
