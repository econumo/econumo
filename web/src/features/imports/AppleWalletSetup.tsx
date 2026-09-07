import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import type { ImportSourceDto } from '@/api/dto/imports'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { InfoBox } from '@/components/InfoBox'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { useCreatePersonalToken, usePersonalTokens } from '@/features/settings/security'
import { apiErrorMessage } from '@/lib/apiError'
import { backendHost, getWebsiteUrl } from '@/lib/config'
import { METRICS, trackEvent } from '@/lib/metrics'
import { isIOS } from '@/lib/platform'
import { getItem, removeItem, setItem } from '@/lib/storage'
import { useCreateImportSource, useDeleteImportSource, useDiscardImportEvent, useImportQueue, useImportSources } from './queries'

// iOS names an installed shortcut after the file it was imported from, so
// the served basename IS the name the deep link and the automation address.
export const WALLET_SHORTCUT_NAME = 'econumo-wallet-v1'
export const SETUP_SHORTCUT_NAME = 'econumo-setup-v1'
export const WALLET_SHORTCUT_URL = `/shortcuts/${WALLET_SHORTCUT_NAME}.shortcut`
export const SETUP_SHORTCUT_URL = `/shortcuts/${SETUP_SHORTCUT_NAME}.shortcut`
export const WALLET_RUN_DEEP_LINK = `shortcuts://run-shortcut?name=${encodeURIComponent(WALLET_SHORTCUT_NAME)}`
export const MANUAL_GUIDE_URL = `${getWebsiteUrl()}/docs/user-guide/apple-wallet`
const INGEST_TOKEN_NAME = 'Apple Wallet'
// What the server answers to a wallet shortcut run by hand: the Transaction
// input is absent, so the body has no account. That failure IS the proof the
// shortcut reached the server, and it is cleared from the queue on check.
const NO_INPUT_ERROR = 'account is required'

// The Setup shortcut takes one text input: JSON with the server URL and the
// ingest token.
export function setupDeepLink(url: string, token: string): string {
  const input = encodeURIComponent(JSON.stringify({ url, token }))
  return `shortcuts://run-shortcut?name=${encodeURIComponent(SETUP_SHORTCUT_NAME)}&input=text&text=${input}`
}

// Indirected through a stable object (not a bare function export) so tests
// can vi.spyOn it without redefining window.location — a redefine that
// breaks other code's window.location.href reads in jsdom.
export const nav = {
  openDeepLink(url: string) {
    window.location.href = url
  },
}

const STEPS = ['install_wallet', 'install_setup', 'configure', 'test', 'automate', 'payment'] as const
type StepId = (typeof STEPS)[number]

// Hand-ticked steps live per source in localStorage: the server knows nothing
// about the phone-side steps, and a reconnect starts the list over.
const ticksKey = (sourceId: string) => `appleWalletSetup:${sourceId}`

function readTicks(sourceId: string): StepId[] {
  const stored = getItem(ticksKey(sourceId))
  return Array.isArray(stored) ? stored.filter((s): s is StepId => STEPS.includes(s)) : []
}

export function AppleWalletSetup({ source }: { source: ImportSourceDto | null }) {
  const { t } = useTranslation()
  const createSource = useCreateImportSource()
  const deleteSource = useDeleteImportSource()
  const createToken = useCreatePersonalToken()
  const discardEvent = useDiscardImportEvent()
  const sources = useImportSources()
  const queue = useImportQueue()
  const tokens = usePersonalTokens()
  const [disconnectOpen, setDisconnectOpen] = useState(false)
  const [configured, setConfigured] = useState(false)
  const [showSteps, setShowSteps] = useState(false)
  const [testResult, setTestResult] = useState<'ok' | 'none' | null>(null)
  const [paymentResult, setPaymentResult] = useState<'ok' | 'none' | null>(null)
  const [checking, setChecking] = useState(false)
  const [ticks, setTicks] = useState<StepId[]>(() => (source ? readTicks(source.id) : []))
  const serverUrl = backendHost()

  const tick = (id: StepId, on: boolean) => {
    if (!source) {
      return
    }
    const next = on ? [...new Set([...ticks, id])] : ticks.filter((s) => s !== id)
    setTicks(next)
    setItem(ticksKey(source.id), next)
  }

  const configureHere = async () => {
    try {
      const created = await createToken.mutateAsync({ name: INGEST_TOKEN_NAME, expiresAt: null, scope: 'ingest' })
      trackEvent(METRICS.IMPORT_SHORTCUT_CONFIGURE)
      setConfigured(true)
      nav.openDeepLink(setupDeepLink(serverUrl, created.token))
    } catch (err) {
      toast.error(apiErrorMessage(err))
    }
  }

  const runTest = () => {
    trackEvent(METRICS.IMPORT_SHORTCUT_TEST)
    nav.openDeepLink(WALLET_RUN_DEEP_LINK)
  }

  // Any event from this source proves the shortcut reached the server — a
  // no-input run lands in "needs attention" and is swept away here so the
  // user never has to discard their own test by hand.
  const checkTest = async () => {
    if (!source) {
      return
    }
    setChecking(true)
    try {
      const [q, s] = await Promise.all([queue.refetch(), sources.refetch()])
      const failed = (q.data?.failed ?? []).filter((e) => e.sourceId === source.id)
      const queued = [...(q.data?.queued ?? []), ...(q.data?.skipped ?? [])].some((e) => e.sourceId === source.id)
      const tapped = (s.data ?? []).some((src) => src.id === source.id && src.cards.some((c) => c.tapCount > 0))
      const ok = failed.length > 0 || queued || tapped
      for (const e of failed.filter((f) => f.error === NO_INPUT_ERROR)) {
        await discardEvent.mutateAsync(e.eventId)
      }
      trackEvent(METRICS.IMPORT_SHORTCUT_CHECK, { step: 'test', ok })
      setTestResult(ok ? 'ok' : 'none')
      if (ok) {
        tick('test', true)
      }
    } catch (err) {
      toast.error(apiErrorMessage(err))
    } finally {
      setChecking(false)
    }
  }

  const checkPayment = async () => {
    if (!source) {
      return
    }
    setChecking(true)
    try {
      const s = await sources.refetch()
      const tapped = (s.data ?? []).find((src) => src.id === source.id)?.cards.some((c) => c.tapCount > 0) ?? false
      trackEvent(METRICS.IMPORT_SHORTCUT_CHECK, { step: 'payment', ok: tapped })
      setPaymentResult(tapped ? 'ok' : 'none')
    } catch (err) {
      toast.error(apiErrorMessage(err))
    } finally {
      setChecking(false)
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

  const tappedCard = source.cards.find((c) => c.tapCount > 0)
  const hasIngestToken = configured || (tokens.data ?? []).some((tok) => tok.scope === 'ingest')
  const auto: Partial<Record<StepId, boolean>> = { configure: hasIngestToken, payment: tappedCard !== undefined }
  const done = (id: StepId) => auto[id] === true || ticks.includes(id)
  const allDone = STEPS.every(done)
  const ios = isIOS()

  const step = (id: StepId, children: ReactNode) => (
    <li key={id} className="flex gap-3">
      <Checkbox id={`wallet-step-${id}`} className="mt-0.5" checked={done(id)} onCheckedChange={(checked) => tick(id, checked === true)} />
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <label htmlFor={`wallet-step-${id}`} className="font-medium">{t(`imports.apple_wallet.steps.${id}.title`)}</label>
        {children}
      </div>
    </li>
  )

  const download = (name: string, url: string, shortcut: 'wallet' | 'setup') => (
    // target=_blank: from the home-screen (standalone) app a same-window
    // navigation to the file has no back button, so the user had to
    // relaunch the app to fetch the second shortcut.
    <a href={url} download target="_blank" rel="noopener" onClick={() => trackEvent(METRICS.IMPORT_SHORTCUT_DOWNLOAD, { shortcut })} className="self-start rounded-md border px-3 py-2">
      {name}
    </a>
  )

  return (
    <div className="flex flex-col gap-4 rounded-lg bg-econumo-card px-4 py-3.5 text-sm">
      <div className="flex items-center justify-between gap-2">
        <span className="font-medium">{t('imports.apple_wallet.connected')}</span>
        <Button type="button" variant="ghost" size="sm" onClick={() => setDisconnectOpen(true)}>
          {t('imports.apple_wallet.disconnect')}
        </Button>
      </div>

      {allDone ? (
        <div className="flex items-center justify-between gap-2">
          <span className="text-muted-foreground">{t('imports.apple_wallet.steps.complete')}</span>
          <Button type="button" variant="ghost" size="sm" onClick={() => setShowSteps((v) => !v)}>
            {t(showSteps ? 'imports.apple_wallet.steps.hide' : 'imports.apple_wallet.steps.show')}
          </Button>
        </div>
      ) : null}

      {allDone && !showSteps ? null : (
        <>
          {ios ? null : <InfoBox>{t('imports.apple_wallet.steps.hint_desktop')}</InfoBox>}
          <ol className="flex flex-col gap-4">
            {step('install_wallet', (
              <>
                <span className="text-muted-foreground">{t('imports.apple_wallet.steps.install_wallet.text')}</span>
                {download(WALLET_SHORTCUT_NAME, WALLET_SHORTCUT_URL, 'wallet')}
              </>
            ))}
            {step('install_setup', (
              <>
                <span className="text-muted-foreground">{t('imports.apple_wallet.steps.install_setup.text')}</span>
                {download(SETUP_SHORTCUT_NAME, SETUP_SHORTCUT_URL, 'setup')}
              </>
            ))}
            {step('configure', (
              <>
                <span className="text-muted-foreground">{t('imports.apple_wallet.steps.configure.text')}</span>
                <div className="flex flex-wrap items-center gap-3 pt-1">
                  {ios ? (
                    <Button type="button" className="h-11" disabled={createToken.isPending} onClick={() => void configureHere()}>
                      {t('imports.apple_wallet.steps.configure.button')}
                    </Button>
                  ) : null}
                  <a href={MANUAL_GUIDE_URL} target="_blank" rel="noopener" className="text-primary underline-offset-2 hover:underline">
                    {t('imports.apple_wallet.steps.configure.manual')}
                  </a>
                </div>
                {configured ? <span className="text-muted-foreground">{t('imports.apple_wallet.steps.configure.done')}</span> : null}
              </>
            ))}
            {step('test', (
              <>
                <span className="text-muted-foreground">{t('imports.apple_wallet.steps.test.text')}</span>
                <div className="flex flex-wrap items-center gap-2 pt-1">
                  {ios ? (
                    <Button type="button" className="h-11" onClick={runTest}>
                      {t('imports.apple_wallet.steps.test.button')}
                    </Button>
                  ) : null}
                  <Button type="button" variant="outline" className="h-11" disabled={checking} onClick={() => void checkTest()}>
                    {t('imports.apple_wallet.steps.test.check')}
                  </Button>
                </div>
                {testResult ? <span className="text-muted-foreground">{t(`imports.apple_wallet.steps.test.${testResult}`)}</span> : null}
              </>
            ))}
            {step('automate', (
              <span className="text-muted-foreground">{t('imports.apple_wallet.steps.automate.text')}</span>
            ))}
            {step('payment', (
              <>
                <span className="text-muted-foreground">{t('imports.apple_wallet.steps.payment.text')}</span>
                <div className="pt-1">
                  <Button type="button" variant="outline" className="h-11" disabled={checking} onClick={() => void checkPayment()}>
                    {t('imports.apple_wallet.steps.payment.check')}
                  </Button>
                </div>
                {paymentResult === 'ok' && tappedCard ? (
                  <span className="text-muted-foreground">{t('imports.apple_wallet.steps.payment.ok', { card: tappedCard.externalName })}</span>
                ) : null}
                {paymentResult === 'none' ? <span className="text-muted-foreground">{t('imports.apple_wallet.steps.payment.none')}</span> : null}
              </>
            ))}
          </ol>
          <InfoBox>{t('imports.apple_wallet.same_named_cards')}</InfoBox>
        </>
      )}

      <ConfirmDialog
        open={disconnectOpen}
        onClose={() => setDisconnectOpen(false)}
        onConfirm={() => {
          setDisconnectOpen(false)
          removeItem(ticksKey(source.id))
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
