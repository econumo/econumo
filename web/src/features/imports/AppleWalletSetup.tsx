import { useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Check, ChevronDown, Download, ExternalLink, Info, Loader2, RotateCcw } from 'lucide-react'
import { toast } from 'sonner'
import type { ImportSourceDto } from '@/api/dto/imports'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { Button } from '@/components/ui/button'
import { useCreatePersonalToken, usePersonalTokens, useRevokePersonalToken } from '@/features/settings/security'
import { apiErrorMessage } from '@/lib/apiError'
import { backendHost, getWebsiteUrl } from '@/lib/config'
import { METRICS, trackEvent } from '@/lib/metrics'
import { isIOS, openAppUrl } from '@/lib/platform'
import { useCreateImportSource, useDeleteImportSource } from './queries'
import { STEPS, forgetSetupTicks, type StepId, type WalletSetup } from './walletSetup'

// iOS names an installed shortcut after the file it was imported from, so
// the served basename IS the name the deep link and the automation address.
export const WALLET_SHORTCUT_NAME = 'econumo'
export const WALLET_SHORTCUT_URL = `/shortcuts/${WALLET_SHORTCUT_NAME}.shortcut`
export const WALLET_GUIDE_URL = `${getWebsiteUrl()}/docs/user-guide/apple-wallet`
const INGEST_TOKEN_NAME = 'Apple Wallet'

// Inside an installed PWA iOS keeps every same-origin link in the app's own
// window (a download replaces the app until it is relaunched). An iCloud
// shortcut link is another origin and Shortcuts opens it straight into "Add
// Shortcut", so the install uses it when one is published (Share ->
// Copy iCloud Link on the signed shortcut; re-copy it whenever the recipe
// changes). Shortcuts' import-shortcut scheme accepts only iCloud links, so
// the served file cannot be handed over that way. Empty = the file link.
export const WALLET_SHORTCUT_ICLOUD_URL = 'https://www.icloud.com/shortcuts/9a41df014a97408aaf4092a5bc69d228'

// Run with text input, the shortcut configures itself: JSON with the server
// URL and the ingest token.
export function setupDeepLink(url: string, token: string): string {
  const input = encodeURIComponent(JSON.stringify({ url, token }))
  return `shortcuts://run-shortcut?name=${encodeURIComponent(WALLET_SHORTCUT_NAME)}&input=text&text=${input}`
}

// Everything the guide opens goes to a new window: from the home-screen
// (standalone) app a same-window navigation has no way back, so the user had
// to relaunch the app to continue. These buttons carry shortcut names, which
// must read exactly as the Shortcuts app shows them, so they opt out of the
// global uppercase button rule (an unlayered rule, hence the !important).
function ExternalButton({ href, onClick, variant = 'outline', download = false, children }: {
  href: string
  onClick?: () => void
  variant?: 'default' | 'outline'
  download?: boolean
  children: ReactNode
}) {
  return (
    <Button asChild variant={variant} className="h-10 self-start normal-case! tracking-normal!">
      <a href={href} target="_blank" rel="noopener" download={download || undefined} onClick={onClick}>
        {children}
      </a>
    </Button>
  )
}

export function AppleWalletConnect() {
  const { t } = useTranslation()
  const createSource = useCreateImportSource()
  return (
    <div className="flex flex-col gap-4 rounded-lg bg-econumo-card px-4 py-4 text-sm">
      <p>{t('imports.apple_wallet.intro')}</p>
      <div className="flex flex-col gap-1.5">
        <h2 className="text-xs uppercase text-muted-foreground">{t('imports.apple_wallet.connect_intro.needs_title')}</h2>
        <ul className="flex flex-col gap-1 text-muted-foreground">
          {(['iphone', 'shortcuts', 'time'] as const).map((k) => (
            <li key={k} className="flex gap-2"><Check className="mt-0.5 size-4 shrink-0 text-econumo-purple" />{t(`imports.apple_wallet.connect_intro.needs.${k}`)}</li>
          ))}
        </ul>
      </div>
      <div className="flex flex-col gap-1.5">
        <h2 className="text-xs uppercase text-muted-foreground">{t('imports.apple_wallet.connect_intro.limitations_title')}</h2>
        <ul className="flex flex-col gap-1 text-muted-foreground">
          {(['tap_only', 'online_not_imported'] as const).map((key) => (
            <li key={key} className="flex gap-2">
              <span aria-hidden="true" className="w-4 shrink-0 text-center">•</span>
              <span>{t(`imports.apple_wallet.connect_intro.${key}`)}</span>
            </li>
          ))}
        </ul>
      </div>
      <div className="flex flex-col gap-1.5">
        <h2 className="text-xs uppercase text-muted-foreground">{t('imports.apple_wallet.connect_intro.steps_title')}</h2>
        <ol className="flex flex-col gap-2 text-muted-foreground">
          {(['pay', 'send', 'privacy', 'record'] as const).map((id, i) => (
            <li key={id} className="flex gap-2">
              <span className="w-4 shrink-0 text-right">{i + 1}.</span>
              <div className="min-w-0">
                <span>{t(`imports.apple_wallet.connect_intro.flow.${id}`)}</span>
              </div>
            </li>
          ))}
        </ol>
      </div>
      <Button type="button" className="h-11" disabled={createSource.isPending} onClick={() => createSource.mutate({ provider: 'apple-wallet', name: 'iPhone' })}>
        {t('imports.apple_wallet.connect')}
      </Button>
    </div>
  )
}

function StepMarker({ n, state }: { n: number; state: 'done' | 'current' | 'todo' }) {
  if (state === 'done') {
    return <span aria-hidden="true" className="flex size-6 shrink-0 items-center justify-center rounded-full bg-econumo-purple text-white"><Check className="size-3.5" /></span>
  }
  return (
    <span aria-hidden="true" className={`flex size-6 shrink-0 items-center justify-center rounded-full text-xs ${state === 'current' ? 'border-2 border-econumo-purple text-econumo-purple' : 'border border-muted-foreground/40 text-muted-foreground'}`}>
      {n}
    </span>
  )
}

function Waiting({ children }: { children: ReactNode }) {
  return (
    <span className="flex items-center gap-2 text-muted-foreground" role="status">
      <Loader2 className="size-4 animate-spin" />
      {children}
    </span>
  )
}

function Stepper({ setup, configuring, setConfiguring, resetting }: {
  setup: WalletSetup
  configuring: boolean
  setConfiguring: (value: boolean) => void
  resetting: boolean
}) {
  const { t } = useTranslation()
  const createToken = useCreatePersonalToken()
  const revokeToken = useRevokePersonalToken()
  const tokens = usePersonalTokens()
  const configuringRef = useRef(false)
  // the open step follows the current one as steps complete
  const [open, setOpen] = useState<StepId | null>(setup.current)
  const [followed, setFollowed] = useState<StepId | null>(setup.current)
  if (followed !== setup.current) {
    setFollowed(setup.current)
    setOpen(setup.current)
  }

  const handTick = (id: StepId) => {
    trackEvent(METRICS.IMPORT_SETUP_STEP, { step: id, done: true })
    setup.tick(id)
  }

  // The shortcut on the phone must be the only holder of a live ingest
  // credential, so minting revokes every ingest token first — re-read from
  // the server, not the rendered list, which may be stale (minted from another
  // device, or a list request that failed while the button showed). The
  // Shortcuts link is a second tap: a new window opened after an await is no
  // longer a user gesture, and Safari blocks it.
  const createLink = async () => {
    // A double tap must not mint twice: the ref closes the window before the
    // disabled state has rendered.
    if (configuringRef.current) {
      return
    }
    configuringRef.current = true
    setConfiguring(true)
    try {
      const current = (await tokens.refetch()).data ?? []
      for (const tok of current.filter((x) => x.scope === 'ingest')) {
        await revokeToken.mutateAsync(tok.id)
      }
      const created = await createToken.mutateAsync({ name: INGEST_TOKEN_NAME, expiresAt: null, scope: 'ingest' })
      trackEvent(METRICS.IMPORT_SHORTCUT_CONFIGURE)
      const link = setupDeepLink(backendHost(), created.token)
      setup.setConfigureLink(link)
      // One tap: hand the link to Shortcuts as soon as the token exists. It
      // runs after an await, so iOS may refuse it as not coming from a tap;
      // the "Link Econumo" button the link also shows is the fallback.
      trackEvent(METRICS.IMPORT_SHORTCUT_TEST)
      openAppUrl(link)
    } catch (err) {
      toast.error(apiErrorMessage(err))
    } finally {
      configuringRef.current = false
      setConfiguring(false)
    }
  }

  const text = (id: StepId) => <p className="text-muted-foreground">{t(`imports.apple_wallet.steps.${id}.text`)}</p>
  const doneButton = (id: StepId) => (
    <Button type="button" className="h-10 self-start" disabled={resetting || (id === 'immediate' && !setup.done('payment'))} onClick={() => handTick(id)}>{t('imports.apple_wallet.steps.done')}</Button>
  )

  const body = (id: StepId): ReactNode => {
    const isDone = setup.done(id)
    switch (id) {
      case 'install':
        return (
          <>
            {text(id)}
            {WALLET_SHORTCUT_ICLOUD_URL ? (
              <div className="flex flex-wrap items-center gap-3">
                <ExternalButton href={WALLET_SHORTCUT_ICLOUD_URL} onClick={() => trackEvent(METRICS.IMPORT_SHORTCUT_DOWNLOAD, { shortcut: 'wallet', via: 'icloud' })}>
                  <ExternalLink className="size-4" />{WALLET_SHORTCUT_NAME}
                </ExternalButton>
                <a href={WALLET_SHORTCUT_URL} target="_blank" rel="noopener" className="text-econumo-link underline-offset-2 hover:underline"
                  onClick={() => trackEvent(METRICS.IMPORT_SHORTCUT_DOWNLOAD, { shortcut: 'wallet', via: 'file' })}>
                  {t('imports.apple_wallet.steps.install.file')}
                </a>
              </div>
            ) : (
              <ExternalButton href={WALLET_SHORTCUT_URL} download onClick={() => trackEvent(METRICS.IMPORT_SHORTCUT_DOWNLOAD, { shortcut: 'wallet' })}>
                <Download className="size-4" />{WALLET_SHORTCUT_NAME}
              </ExternalButton>
            )}
            {!isDone ? doneButton(id) : null}
          </>
        )
      case 'configure':
        return (
          <>
            {text(id)}
            <div className="flex flex-wrap items-center gap-3">
              {/* offered after the step is done too: a new iPhone, a revoked
                  token or a reinstalled Shortcut all need the connect again */}
              {setup.configureLink ? (
                <ExternalButton href={setup.configureLink} variant="default" onClick={() => trackEvent(METRICS.IMPORT_SHORTCUT_TEST)}>
                  <ExternalLink className="size-4" />{t('imports.apple_wallet.steps.configure.open')}
                </ExternalButton>
              ) : (
                <Button type="button" variant={isDone ? 'outline' : 'default'} className="h-10" disabled={configuring || resetting} onClick={() => void createLink()}>
                  {t('imports.apple_wallet.steps.configure.button')}
                </Button>
              )}
            </div>
            {/* connecting again revokes the token the Shortcut holds now */}
            {isDone && !setup.configureLink ? <p className="text-xs text-muted-foreground">{t('imports.apple_wallet.steps.configure.again_note')}</p> : null}
            {!isDone ? <><Waiting>{t('imports.apple_wallet.steps.configure.waiting')}</Waiting>{doneButton(id)}</> : null}
          </>
        )
      case 'payment':
        return (
          <>
            {text(id)}
            {!isDone ? <Waiting>{t('imports.apple_wallet.steps.payment.waiting')}</Waiting> : null}
          </>
        )
      default:
        return (
          <>
            {text(id)}
            {!isDone ? doneButton(id) : null}
          </>
        )
    }
  }

  return (
    <div className="flex flex-col gap-1">
      <ol className="flex flex-col">
        {STEPS.map((id, i) => {
          const isOpen = open === id
          const state = setup.done(id) ? 'done' : setup.current === id ? 'current' : 'todo'
          return (
            <li key={id} className="flex flex-col" data-state={state}>
              <button type="button" className="flex items-center gap-3 rounded-md py-2 text-left" aria-expanded={isOpen} onClick={() => setOpen(isOpen ? null : id)}>
                <StepMarker n={i + 1} state={state} />
                <span className={state === 'todo' ? 'text-muted-foreground' : ''}>{t(`imports.apple_wallet.steps.${id}.title`)}</span>
              </button>
              {isOpen ? <div className="flex flex-col gap-3 pb-3 pl-9">{body(id)}</div> : null}
            </li>
          )
        })}
      </ol>
    </div>
  )
}

export function AppleWalletSetup({ setup }: { setup: WalletSetup }) {
  const { t } = useTranslation()
  const complete = setup.current === null
  const [expanded, setExpanded] = useState(!complete)
  const open = !complete || expanded
  const [configuring, setConfiguring] = useState(false)
  const [resetting, setResetting] = useState(false)
  const [resetVersion, setResetVersion] = useState(0)

  const reconfigure = async () => {
    setResetting(true)
    try {
      await setup.reset()
      setResetVersion((version) => version + 1)
      setExpanded(true)
    } catch (err) {
      toast.error(apiErrorMessage(err))
    } finally {
      setResetting(false)
    }
  }
  const reconfigureButton = setup.doneCount > 0 || setup.configureLink ? (
    <button
      type="button"
      className="flex min-h-8 shrink-0 items-center gap-1.5 rounded-md px-2 text-xs text-muted-foreground transition-colors hover:bg-econumo-hover hover:text-econumo-link focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
      disabled={configuring || resetting}
      onClick={() => void reconfigure()}
    >
      <RotateCcw className="size-3.5" />{t('imports.apple_wallet.setup.reconfigure')}
    </button>
  ) : null
  const instructionsLink = (
    <a href={WALLET_GUIDE_URL} target="_blank" rel="noopener" className="inline-flex items-center gap-1.5 text-xs text-econumo-link underline-offset-2 hover:underline">
      {t('imports.apple_wallet.instructions')}<ExternalLink className="size-3.5" />
    </a>
  )
  const stepper = <Stepper key={resetVersion} setup={setup} configuring={configuring} setConfiguring={setConfiguring} resetting={resetting} />

  return (
    <div className="rounded-lg bg-econumo-card px-4 py-3 text-sm">
      <div className={`flex flex-wrap items-center justify-between gap-x-3 gap-y-1 ${open ? 'border-b border-border pb-3' : ''}`}>
        <h2 className="min-w-0 flex-1 font-medium">
          {complete ? (
            <button type="button" className={`flex min-h-8 w-full items-center gap-2 rounded-md text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${open ? '' : 'justify-between'}`} aria-expanded={open} onClick={() => setExpanded((value) => !value)}>
              {t('imports.apple_wallet.setup.title')}
              <ChevronDown className={`size-4 shrink-0 text-muted-foreground transition-transform ${open ? 'rotate-180' : ''}`} />
            </button>
          ) : t('imports.apple_wallet.setup.title')}
        </h2>
        {open ? reconfigureButton : null}
      </div>
      {open ? (
        <div className="pt-3">
          <div className="mb-3">
            {isIOS() ? instructionsLink : (
              <div className="flex items-start gap-3 text-muted-foreground">
                <span className="flex size-6 shrink-0 items-center justify-center" aria-hidden="true"><Info className="size-4 text-econumo-purple" /></span>
                <div className="min-w-0">
                  <p>{t('imports.apple_wallet.steps.hint_desktop')}</p>
                  <div className="mt-2">{instructionsLink}</div>
                </div>
              </div>
            )}
          </div>
          {stepper}
        </div>
      ) : null}
    </div>
  )
}

export function AppleWalletDisconnect({ source }: { source: ImportSourceDto }) {
  const { t } = useTranslation()
  const deleteSource = useDeleteImportSource()
  const [disconnectOpen, setDisconnectOpen] = useState(false)
  return (
    <div className="flex justify-center pt-4">
      <Button type="button" variant="ghost" className="text-destructive hover:text-destructive" onClick={() => setDisconnectOpen(true)}>
        {t('imports.apple_wallet.disconnect_full')}
      </Button>
      <ConfirmDialog
        open={disconnectOpen}
        onClose={() => setDisconnectOpen(false)}
        onConfirm={() => {
          setDisconnectOpen(false)
          forgetSetupTicks(source.id)
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
