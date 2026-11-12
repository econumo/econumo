import { useEffect, useRef, useState } from 'react'
import { ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router'
import { toast } from 'sonner'
import { RouterPage } from '@/app/router-pages'
import { InfoBox } from '@/components/InfoBox'
import { OptionSelect } from '@/components/OptionSelect'
import { Switch } from '@/components/ui/switch'
import { SettingsShell } from '@/features/settings/SettingsShell'
import { useUserData } from '@/features/user/queries'
import { METRICS, trackEvent } from '@/lib/metrics'
import { getSessionUserId } from '@/lib/storage'
import {
  activeConfig,
  clearConfig,
  DEFAULT_LOCK_AFTER_MS,
  LOCK_AFTER_OPTIONS,
  readConfig,
  resetAttempts,
  writeConfig,
  type AppLockConfig,
} from './config'
import { APP_LOCK_CONFIG_KEY } from './keys'
import { clearFreshMarker, unlock } from './lockStore'
import { hashSecret } from './secret'
import { SecretDialog, type SecretFlow } from './SecretDialog'

const rowClass = 'flex items-center justify-between gap-4 rounded-lg bg-econumo-card px-4 py-3.5'

export function AppLockPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { data: user } = useUserData()
  const [config, setConfig] = useState<AppLockConfig | null>(() => activeConfig())
  const [flow, setFlow] = useState<{ kind: SecretFlow; seq: number; open: boolean } | null>(null)
  const flowSeq = useRef(0)
  // The flow still on screen; a check or hash finishing after the dialog was
  // dismissed must not change the lock.
  const openSeq = useRef<number | null>(null)
  const enabled = config !== null
  const userId = getSessionUserId() ?? user?.id ?? null

  const refresh = () => setConfig(activeConfig())

  // Another tab may change the lock; this page shows what is stored.
  useEffect(() => {
    const onStorage = (event: StorageEvent) => {
      if (event.key === null || event.key === APP_LOCK_CONFIG_KEY) {
        setConfig(activeConfig())
      }
    }
    window.addEventListener('storage', onStorage)
    return () => window.removeEventListener('storage', onStorage)
  }, [])

  const openFlow = (kind: SecretFlow) => {
    const seq = ++flowSeq.current
    openSeq.current = seq
    setFlow({ kind, seq, open: true })
  }
  const closeFlow = () => {
    openSeq.current = null
    setFlow((prev) => (prev ? { ...prev, open: false } : prev))
  }

  // Every write starts from what is stored now, so a change from another tab survives.
  const update = (change: Partial<AppLockConfig>): boolean => {
    const current = readConfig()
    if (current === null || current.userId !== config?.userId) {
      refresh()
      return false
    }
    writeConfig({ ...current, ...change })
    refresh()
    return true
  }

  const onComplete = async (secret?: string) => {
    if (flow === null) {
      return
    }
    const seq = flow.seq
    const stillOpen = () => openSeq.current === seq
    if (!stillOpen()) {
      return
    }
    if (flow.kind === 'disable') {
      clearConfig()
      unlock()
      trackEvent(METRICS.APP_LOCK_DISABLED)
    } else if (flow.kind === 'enable' && secret && userId) {
      const hash = await hashSecret(secret)
      if (!stillOpen()) {
        return
      }
      resetAttempts()
      // Turning the lock on does not lock this tab: the user just proved who
      // they are. A sign-in marker left in this tab would skip the next cold
      // start's lock, so it is spent.
      clearFreshMarker()
      writeConfig({ v: 1, userId, secret: hash, lockAfterMs: DEFAULT_LOCK_AFTER_MS })
      trackEvent(METRICS.APP_LOCK_ENABLED, { kind: hash.kind })
    } else if (flow.kind === 'change' && secret) {
      const hash = await hashSecret(secret)
      if (!stillOpen()) {
        return
      }
      if (update({ secret: hash })) {
        trackEvent(METRICS.APP_LOCK_SECRET_CHANGED, { kind: hash.kind })
        toast.success(hash.kind === 'pattern' ? t('appLock.settings.pattern_changed') : t('appLock.settings.pin_changed'))
      }
    }
    refresh()
    closeFlow()
  }

  const onExceeded = () => {
    closeFlow()
    trackEvent(METRICS.APP_LOCK_ATTEMPTS_EXCEEDED)
    void navigate(RouterPage.LOGOUT, { state: { keepPendingWrites: true, resetAppLock: true } })
  }

  const onLockAfterChange = (value: string) => {
    const ms = Number(value)
    if (ms === config?.lockAfterMs || !(LOCK_AFTER_OPTIONS as readonly number[]).includes(ms)) {
      return
    }
    if (update({ lockAfterMs: ms })) {
      trackEvent(METRICS.APP_LOCK_TIMEOUT_CHANGED, { lockAfter: ms / 1000 })
    }
  }

  const lockAfterOptions = [
    { value: '0', label: t('appLock.settings.lock_after.immediately') },
    { value: '60000', label: t('appLock.settings.lock_after.one_minute') },
    { value: '300000', label: t('appLock.settings.lock_after.five_minutes') },
    { value: '900000', label: t('appLock.settings.lock_after.fifteen_minutes') },
  ]

  return (
    <SettingsShell
      title={t('appLock.settings.title')}
      backTo={RouterPage.SETTINGS_PROFILE}
      crumbs={[
        { label: t('settings.page.header_desktop'), to: RouterPage.SETTINGS },
        { label: t('user.page.settings.profile.menu_item'), to: RouterPage.SETTINGS_PROFILE },
      ]}
      editableOffline
    >
      <InfoBox>{t('appLock.settings.about')}</InfoBox>

      <div className="flex max-w-md flex-col gap-2 py-2">
        <div className={rowClass}>
          <div className="flex min-w-0 flex-col gap-0.5">
            <label htmlFor="app-lock-enabled" className="text-sm">
              {t('appLock.settings.enabled_label')}
            </label>
            <span className="text-xs text-muted-foreground">{t('appLock.settings.enabled_hint')}</span>
          </div>
          <Switch
            id="app-lock-enabled"
            checked={enabled}
            disabled={!enabled && userId === null}
            onCheckedChange={(checked) => openFlow(checked ? 'enable' : 'disable')}
          />
        </div>

        {enabled ? (
          <>
            <button
              type="button"
              className={`${rowClass} text-left text-sm hover:bg-econumo-hover`}
              onClick={() => openFlow('change')}
            >
              {config.secret.kind === 'pattern' ? t('appLock.settings.change_pattern') : t('appLock.settings.change_pin')}
              <ChevronRight className="size-4 shrink-0 text-muted-foreground" />
            </button>

            <div className={rowClass}>
              <span className="text-sm">
                {t('appLock.settings.lock_after.label')}
              </span>
              <OptionSelect
                id="app-lock-after"
                aria-label={t('appLock.settings.lock_after.label')}
                title={t('appLock.settings.lock_after.label')}
                value={String(config.lockAfterMs)}
                onValueChange={onLockAfterChange}
                options={lockAfterOptions}
                className="w-40 shrink-0"
              />
            </div>
          </>
        ) : null}
      </div>

      {flow ? (
        <SecretDialog
          key={flow.seq}
          open={flow.open}
          flow={flow.kind}
          current={config ? { kind: config.secret.kind, length: config.secret.length } : null}
          onClose={closeFlow}
          onComplete={onComplete}
          onExceeded={onExceeded}
        />
      ) : null}
    </SettingsShell>
  )
}
