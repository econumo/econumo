import { useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Check, ChevronDown, CirclePlay, Download, ExternalLink, Loader2 } from 'lucide-react'
import { toast } from 'sonner'
import type { ImportSourceDto } from '@/api/dto/imports'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { InfoBox } from '@/components/InfoBox'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { Button } from '@/components/ui/button'
import { useCreatePersonalToken, usePersonalTokens, useRevokePersonalToken } from '@/features/settings/security'
import { apiErrorMessage } from '@/lib/apiError'
import { backendHost, getWebsiteUrl } from '@/lib/config'
import { METRICS, trackEvent } from '@/lib/metrics'
import { isIOS } from '@/lib/platform'
import { useCreateImportSource, useDeleteImportSource } from './queries'
import { SETUP_VIDEOS, embedUrl, youtubeId, type SetupVideo } from './setupVideos'
import { STEPS, forgetSetupTicks, type StepId, type WalletSetup } from './walletSetup'

// iOS names an installed shortcut after the file it was imported from, so
// the served basename IS the name the deep link and the automation address.
export const WALLET_SHORTCUT_NAME = 'econumo'
export const WALLET_SHORTCUT_URL = `/shortcuts/${WALLET_SHORTCUT_NAME}.shortcut`
export const MANUAL_GUIDE_URL = `${getWebsiteUrl()}/docs/user-guide/apple-wallet`
const INGEST_TOKEN_NAME = 'Apple Wallet'

// Inside an installed PWA iOS keeps every same-origin link in the app's own
// window (a download replaces the app until it is relaunched). An iCloud
// shortcut link is another origin and Shortcuts opens it straight into "Add
// Shortcut", so on iOS the install uses it when one is published (Share ->
// Copy iCloud Link on the signed shortcut; re-copy it whenever the recipe
// changes). Shortcuts' import-shortcut scheme accepts only iCloud links, so
// the served file cannot be handed over that way. Empty = the file link.
export const WALLET_SHORTCUT_ICLOUD_URL = 'https://www.icloud.com/shortcuts/55565f20fb794a559e4e66cc1dc828bc'

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

function SetupVideos() {
  const { t } = useTranslation()
  const [playing, setPlaying] = useState<SetupVideo | null>(null)
  if (SETUP_VIDEOS.length === 0) {
    return null
  }
  const id = playing ? youtubeId(playing.url) : null
  const linkClass = 'text-econumo-link underline-offset-2 hover:underline'
  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm">
      <span className="flex items-center gap-2 text-muted-foreground"><CirclePlay className="size-4 text-econumo-purple" />{t('imports.apple_wallet.video')}</span>
      {SETUP_VIDEOS.map((v) => {
        const track = () => trackEvent(METRICS.IMPORT_SETUP_VIDEO, { version: v.label })
        // a link the player cannot take still opens, just on YouTube itself
        return youtubeId(v.url) ? (
          <button key={v.label} type="button" className={linkClass} onClick={() => { track(); setPlaying(v) }}>{v.label}</button>
        ) : (
          <a key={v.label} href={v.url} target="_blank" rel="noopener" className={linkClass} onClick={track}>{v.label}</a>
        )
      })}
      <ResponsiveDialog
        open={playing !== null}
        onOpenChange={(o) => !o && setPlaying(null)}
        title={`${t('imports.apple_wallet.video_title')} · ${playing?.label ?? ''}`}
      >
        {playing && id ? (
          <div className="flex flex-col items-center gap-3">
            {/* phone screen recordings are portrait */}
            <iframe
              src={embedUrl(id)}
              title={`${t('imports.apple_wallet.video_title')} · ${playing.label}`}
              className="aspect-[9/16] h-[min(60vh,640px)] max-w-full rounded-lg bg-black"
              allow="autoplay; encrypted-media; picture-in-picture; fullscreen"
              allowFullScreen
            />
            <a href={playing.url} target="_blank" rel="noopener" className={linkClass}>{t('imports.apple_wallet.video_youtube')}</a>
          </div>
        ) : null}
      </ResponsiveDialog>
    </div>
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
        <h2 className="text-xs uppercase text-muted-foreground">{t('imports.apple_wallet.connect_intro.steps_title')}</h2>
        <ol className="flex flex-col gap-1 text-muted-foreground">
          {STEPS.map((id, i) => (
            <li key={id} className="flex gap-2"><span className="w-4 shrink-0 text-right">{i + 1}.</span>{t(`imports.apple_wallet.steps.${id}.title`)}</li>
          ))}
        </ol>
      </div>
      <SetupVideos />
      <p className="text-xs text-muted-foreground">{t('imports.apple_wallet.connect_intro.privacy')}</p>
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

function Stepper({ setup }: { setup: WalletSetup }) {
  const { t } = useTranslation()
  const createToken = useCreatePersonalToken()
  const revokeToken = useRevokePersonalToken()
  const tokens = usePersonalTokens()
  const [configuring, setConfiguring] = useState(false)
  const configuringRef = useRef(false)
  const ios = isIOS()
  // the open step follows the current one as steps complete
  const [open, setOpen] = useState<StepId | null>(setup.current)
  const [followed, setFollowed] = useState<StepId | null>(setup.current)
  if (followed !== setup.current) {
    setFollowed(setup.current)
    setOpen(setup.current)
  }

  const handTick = (id: StepId, on: boolean) => {
    trackEvent(METRICS.IMPORT_SETUP_STEP, { step: id, done: on })
    setup.tick(id, on)
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
      setup.setConfigureLink(setupDeepLink(backendHost(), created.token))
    } catch (err) {
      toast.error(apiErrorMessage(err))
    } finally {
      configuringRef.current = false
      setConfiguring(false)
    }
  }

  const text = (id: StepId) => <p className="text-muted-foreground">{t(`imports.apple_wallet.steps.${id}.text`)}</p>
  const doneButton = (id: StepId) => (
    <Button type="button" className="h-10 self-start" onClick={() => handTick(id, true)}>{t('imports.apple_wallet.steps.done')}</Button>
  )
  const markDone = (id: StepId) => (
    <Button type="button" variant="ghost" size="sm" className="self-start" onClick={() => handTick(id, true)}>{t('imports.apple_wallet.steps.mark_done')}</Button>
  )

  const body = (id: StepId): ReactNode => {
    const isDone = setup.done(id)
    const undo = isDone && setup.handTicked(id)
      ? <Button type="button" variant="ghost" size="sm" className="self-start" onClick={() => handTick(id, false)}>{t('imports.apple_wallet.steps.undo')}</Button>
      : null
    switch (id) {
      case 'install':
        return (
          <>
            {text(id)}
            {ios && WALLET_SHORTCUT_ICLOUD_URL ? (
              <>
                <ExternalButton href={WALLET_SHORTCUT_ICLOUD_URL} onClick={() => trackEvent(METRICS.IMPORT_SHORTCUT_DOWNLOAD, { shortcut: 'wallet', via: 'icloud' })}>
                  <Download className="size-4" />{WALLET_SHORTCUT_NAME}
                </ExternalButton>
                <a href={WALLET_SHORTCUT_URL} target="_blank" rel="noopener" className="self-start text-sm text-econumo-link underline-offset-2 hover:underline"
                  onClick={() => trackEvent(METRICS.IMPORT_SHORTCUT_DOWNLOAD, { shortcut: 'wallet', via: 'file' })}>
                  {t('imports.apple_wallet.steps.install.file')}
                </a>
              </>
            ) : (
              <ExternalButton href={WALLET_SHORTCUT_URL} download onClick={() => trackEvent(METRICS.IMPORT_SHORTCUT_DOWNLOAD, { shortcut: 'wallet' })}>
                <Download className="size-4" />{WALLET_SHORTCUT_NAME}
              </ExternalButton>
            )}
            {isDone ? undo : doneButton(id)}
          </>
        )
      case 'configure':
        return (
          <>
            {text(id)}
            <div className="flex flex-wrap items-center gap-3">
              {ios && !isDone && setup.configureLink ? (
                <ExternalButton href={setup.configureLink} variant="default" onClick={() => trackEvent(METRICS.IMPORT_SHORTCUT_TEST)}>
                  <ExternalLink className="size-4" />{t('imports.apple_wallet.steps.configure.open', { name: WALLET_SHORTCUT_NAME })}
                </ExternalButton>
              ) : ios && !isDone ? (
                <Button type="button" className="h-10" disabled={configuring} onClick={() => void createLink()}>
                  {t('imports.apple_wallet.steps.configure.button')}
                </Button>
              ) : null}
              <a href={MANUAL_GUIDE_URL} target="_blank" rel="noopener" className="text-econumo-link underline-offset-2 hover:underline">
                {t('imports.apple_wallet.steps.configure.manual')}
              </a>
            </div>
            {isDone ? undo : <><Waiting>{t('imports.apple_wallet.steps.configure.waiting')}</Waiting>{markDone(id)}</>}
          </>
        )
      case 'payment':
        return (
          <>
            {text(id)}
            {isDone ? undo : <><Waiting>{t('imports.apple_wallet.steps.payment.waiting')}</Waiting>{markDone(id)}</>}
          </>
        )
      default:
        return (
          <>
            {text(id)}
            {isDone ? undo : doneButton(id)}
          </>
        )
    }
  }

  return (
    <div className="flex flex-col gap-1">
      {ios ? null : <InfoBox>{t('imports.apple_wallet.steps.hint_desktop')}</InfoBox>}
      <SetupVideos />
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

export function AppleWalletSetup({ setup, collapsible }: { setup: WalletSetup; collapsible: boolean }) {
  const { t } = useTranslation()
  // Once taps are flowing the guide starts folded to its progress row.
  const [open, setOpen] = useState(!collapsible)
  const allDone = setup.current === null

  if (!collapsible) {
    return (
      <div className="rounded-lg bg-econumo-card px-4 py-3 text-sm">
        <Stepper setup={setup} />
      </div>
    )
  }
  return (
    <section className="flex flex-col gap-2">
      <h2 className="px-1 pt-2 text-xs uppercase text-muted-foreground">{t('imports.apple_wallet.setup.header')}</h2>
      <div className="flex flex-col rounded-lg bg-econumo-card text-sm">
        <button
          type="button"
          className={`flex items-center justify-between gap-2 px-4 py-3.5 text-left hover:bg-econumo-hover ${open ? 'rounded-t-lg' : 'rounded-lg'}`}
          aria-expanded={open}
          onClick={() => setOpen((v) => !v)}
        >
          <span>{t('imports.apple_wallet.setup.title')}</span>
          <span className="flex items-center gap-2 text-xs text-muted-foreground">
            {allDone ? t('imports.apple_wallet.steps.complete') : t('imports.apple_wallet.setup.progress', { done: setup.doneCount, total: STEPS.length })}
            <ChevronDown className={`size-4 transition-transform ${open ? 'rotate-180' : ''}`} />
          </span>
        </button>
        {open ? <div className="px-4 pb-3"><Stepper setup={setup} /></div> : null}
      </div>
    </section>
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
