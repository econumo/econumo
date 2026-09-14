import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import type { OAuthProviderId } from '@/api/dto/oauth'
import { isNativeApp } from '@/lib/platform'
import { backendHost } from '@/lib/config'
import { ProviderMark } from './providerIcons'
import { useOAuthInFlight, useProviders, useStartOAuth } from './oauthQueries'

// Apple's button must be black-on-white or white-on-black with the mark;
// Google's must be light with the multi-colour mark. Both get the auth pages'
// h-11 so they line up with the password form's buttons.
const buttonClass: Record<OAuthProviderId, string> = {
  google: 'h-11 w-full border border-border bg-white text-black hover:bg-zinc-50',
  apple: 'h-11 w-full bg-black text-white hover:bg-zinc-800',
  oidc: 'h-11 w-full',
}

// Explicit keys (not a template literal) so the i18n key guard sees them.
const buttonLabel: Record<OAuthProviderId, string> = {
  google: 'auth.oauth.button.google',
  apple: 'auth.oauth.button.apple',
  oidc: 'auth.oauth.button.oidc',
}

export function ProviderButtons({ intent }: { intent: 'login' | 'link' }) {
  const { t } = useTranslation()
  const providers = useProviders()
  const start = useStartOAuth()
  const inFlight = useOAuthInFlight((s) => s.inFlight)
  // On the web the flow returns to the BACKEND's origin (ECONUMO_URL), where
  // this tab's sessionStorage flow secret does not exist; a SPA pointed at a
  // different backend cannot finish the exchange, so it must not offer it.
  if (!isNativeApp() && backendHost() !== window.location.origin) {
    return null
  }
  if (!providers.data || providers.data.length === 0) {
    return null
  }
  return (
    <div data-testid="provider-buttons" className="flex flex-col gap-3">
      {intent === 'login' ? (
        <div className="flex items-center gap-3 text-xs text-muted-foreground">
          <div className="h-px flex-1 bg-border" />
          <span>{t('auth.oauth.divider')}</span>
          <div className="h-px flex-1 bg-border" />
        </div>
      ) : null}
      {providers.data.map((p) => (
        <Button
          key={p.id}
          type="button"
          variant={p.id === 'oidc' ? 'secondary' : 'outline'}
          className={buttonClass[p.id]}
          disabled={start.isPending || inFlight}
          onClick={() => start.mutate({ provider: p.id, intent })}
        >
          <ProviderMark id={p.id} />
          {p.id === 'oidc' ? t('auth.oauth.button.oidc', { name: p.name }) : t(buttonLabel[p.id])}
        </Button>
      ))}
    </div>
  )
}
