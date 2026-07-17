import { useState } from 'react'
import { Check, Copy, KeyRound, Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Calendar } from '@/components/ui/calendar'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { CardField, cardFieldControlClass } from '@/components/CardField'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { RouterPage } from '@/app/router-pages'
import type { CreatedPersonalTokenDto, PersonalTokenDto } from '@/api/dto/user'
import { calendarLocale } from '@/lib/calendarLocale'
import { formatDate, parseDateTime } from '@/lib/datetime'
import { useCreatePersonalToken, usePersonalTokens, useRevokePersonalToken } from './security'
import { parseUtcDateTime, relativeTime } from './securityFormat'
import { SettingsShell } from './SettingsShell'

type ExpiryChoice = 'd30' | 'd90' | 'd365' | 'custom' | 'never'

// expiresAtFrom maps the dialog's expiry choice to the API's
// "YYYY-MM-DD HH:mm:ss" UTC datetime (end of day for a custom date), or null
// for a token that never expires. Exported for tests.
export function expiresAtFrom(choice: ExpiryChoice, customDate: string, now: Date = new Date()): string | null {
  const days = { d30: 30, d90: 90, d365: 365 }[choice as 'd30' | 'd90' | 'd365']
  if (days) {
    const d = new Date(now.getTime() + days * 24 * 3600_000)
    const pad = (n: number) => String(n).padStart(2, '0')
    return `${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}-${pad(d.getUTCDate())} ${pad(d.getUTCHours())}:${pad(d.getUTCMinutes())}:${pad(d.getUTCSeconds())}`
  }
  if (choice === 'custom' && customDate) {
    return `${customDate} 23:59:59`
  }
  return null
}

export function PersonalTokensPage() {
  const { t, i18n } = useTranslation()
  const { data: tokens } = usePersonalTokens()
  const createToken = useCreatePersonalToken()
  const revokeToken = useRevokePersonalToken()

  const [createOpen, setCreateOpen] = useState(false)
  const [name, setName] = useState('')
  const [expiry, setExpiry] = useState<ExpiryChoice>('never')
  const [customDate, setCustomDate] = useState('')
  const [calendarOpen, setCalendarOpen] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)
  const [created, setCreated] = useState<CreatedPersonalTokenDto | null>(null)
  const [copied, setCopied] = useState(false)
  const [confirmRevoke, setConfirmRevoke] = useState<PersonalTokenDto | null>(null)

  const resetForm = () => {
    setName('')
    setExpiry('never')
    setCustomDate('')
    setFormError(null)
  }

  const submit = () => {
    if (!name.trim()) {
      setFormError(t('user.page.settings.profile.tokens.form.name.validation.required_field'))
      return
    }
    const expiresAt = expiresAtFrom(expiry, customDate)
    if (expiry === 'custom' && (!expiresAt || parseUtcDateTime(expiresAt) <= new Date())) {
      setFormError(t('user.page.settings.profile.tokens.form.expiry.validation.invalid_date'))
      return
    }
    setFormError(null)
    createToken.mutate(
      { name: name.trim(), expiresAt },
      {
        onSuccess: (result) => {
          setCreateOpen(false)
          resetForm()
          setCopied(false)
          setCreated(result)
        },
        onError: () => setFormError(t('user.page.settings.profile.tokens.form.expiry.validation.invalid_date')),
      },
    )
  }

  const copy = () => {
    if (created) {
      void navigator.clipboard.writeText(created.token)
      setCopied(true)
    }
  }

  const expiryOptions: ExpiryChoice[] = ['d30', 'd90', 'd365', 'custom', 'never']

  return (
    <SettingsShell
      title={t('user.page.settings.profile.tokens.header')}
      backTo={RouterPage.SETTINGS_PROFILE}
      crumbs={[
        { label: t('settings.page.header_desktop'), to: RouterPage.SETTINGS },
        { label: t('user.page.settings.profile.menu_item'), to: RouterPage.SETTINGS_PROFILE },
      ]}
      actions={
        <Button
          type="button"
          size="sm"
          onClick={() => setCreateOpen(true)}
          title={t('user.page.settings.profile.tokens.create.label')}
          aria-label={t('user.page.settings.profile.tokens.create.label')}
        >
          <Plus className="size-4" />
          <span className="hidden sm:inline">{t('user.page.settings.profile.tokens.create.label')}</span>
        </Button>
      }
    >
      <p className="max-w-md px-1 py-2 text-xs text-muted-foreground">
        {t('user.page.settings.profile.tokens.description')}
      </p>

      <div className="flex max-w-md flex-col gap-2 py-2">
        {tokens && tokens.length === 0 ? (
          <p className="px-1 text-sm text-muted-foreground">{t('user.page.settings.profile.tokens.empty')}</p>
        ) : null}
        {(tokens ?? []).map((token) => (
          <div key={token.id} className="flex items-center gap-3 rounded-lg bg-econumo-card px-4 py-3">
            <KeyRound className="size-5 shrink-0 text-muted-foreground" aria-hidden="true" />
            <div className="flex min-w-0 flex-1 flex-col gap-0.5">
              <span className="truncate text-sm">{token.name}</span>
              <span className="text-xs text-muted-foreground">
                {t('user.page.settings.profile.tokens.last_used')} {relativeTime(token.lastUsedAt)}
                {' · '}
                {token.expiresAt
                  ? `${t('user.page.settings.profile.tokens.expires')} ${formatDate(parseUtcDateTime(token.expiresAt))}`
                  : t('user.page.settings.profile.tokens.never_expires')}
              </span>
            </div>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="shrink-0 text-econumo-magenta"
              onClick={() => setConfirmRevoke(token)}
            >
              {t('user.page.settings.profile.tokens.revoke')}
            </Button>
          </div>
        ))}
      </div>

      {/* Create dialog */}
      <ResponsiveDialog
        open={createOpen}
        onOpenChange={(o) => {
          if (!o) {
            setCreateOpen(false)
            resetForm()
          }
        }}
        title={t('user.page.settings.profile.tokens.create.label')}
      >
        <form
          className="flex flex-col gap-4"
          noValidate
          onSubmit={(e) => {
            e.preventDefault()
            submit()
          }}
        >
          <CardField label={t('user.page.settings.profile.tokens.form.name.label')} htmlFor="token-name" error={formError}>
            <Input
              id="token-name"
              className={cardFieldControlClass}
              placeholder={t('user.page.settings.profile.tokens.form.name.placeholder')}
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </CardField>
          <fieldset className="flex flex-col gap-2">
            <legend className="pb-1 text-[11px] font-normal text-muted-foreground">
              {t('user.page.settings.profile.tokens.form.expiry.label')}
            </legend>
            <div className="flex flex-wrap gap-2">
              {expiryOptions.map((option) =>
                option === 'custom' ? (
                  <Popover key={option} open={calendarOpen} onOpenChange={setCalendarOpen}>
                    <PopoverTrigger asChild>
                      <Button
                        type="button"
                        size="sm"
                        variant={expiry === 'custom' ? 'default' : 'secondary'}
                        onClick={() => setExpiry('custom')}
                      >
                        {customDate || t('user.page.settings.profile.tokens.form.expiry.options.custom')}
                      </Button>
                    </PopoverTrigger>
                    <PopoverContent className="w-auto p-0" align="start">
                      <Calendar
                        mode="single"
                        weekStartsOn={1}
                        locale={calendarLocale(i18n.language)}
                        selected={customDate ? parseDateTime(customDate) : undefined}
                        onSelect={(day) => {
                          if (day) {
                            setCustomDate(formatDate(day))
                            setExpiry('custom')
                            setCalendarOpen(false)
                          }
                        }}
                      />
                    </PopoverContent>
                  </Popover>
                ) : (
                  <Button
                    key={option}
                    type="button"
                    size="sm"
                    variant={expiry === option ? 'default' : 'secondary'}
                    onClick={() => setExpiry(option)}
                  >
                    {t(`user.page.settings.profile.tokens.form.expiry.options.${option}`)}
                  </Button>
                ),
              )}
            </div>
          </fieldset>
          <div className={dialogActionsClass}>
            <Button
              type="button"
              variant="secondary"
              onClick={() => {
                setCreateOpen(false)
                resetForm()
              }}
            >
              {t('common.button.cancel.label')}
            </Button>
            <Button type="submit" disabled={createToken.isPending}>
              {t('user.page.settings.profile.tokens.form.submit.label')}
            </Button>
          </div>
        </form>
      </ResponsiveDialog>

      {/* Show-once dialog: the token exists only in this dialog's state. */}
      <ResponsiveDialog
        open={created !== null}
        onOpenChange={(o) => !o && setCreated(null)}
        title={t('user.page.settings.profile.tokens.created_dialog.title')}
        description={t('user.page.settings.profile.tokens.created_dialog.warning')}
      >
        <div className="flex flex-col gap-3">
          <code className="break-all rounded-lg bg-econumo-card px-3 py-2 text-xs" data-testid="created-token">
            {created?.token}
          </code>
          <div className={dialogActionsClass}>
            <Button type="button" variant="secondary" onClick={copy}>
              {copied ? <Check className="size-4" /> : <Copy className="size-4" />}
              {copied
                ? t('user.page.settings.profile.tokens.created_dialog.copied')
                : t('user.page.settings.profile.tokens.created_dialog.copy')}
            </Button>
            <Button type="button" onClick={() => setCreated(null)}>
              {t('user.page.settings.profile.tokens.created_dialog.done')}
            </Button>
          </div>
        </div>
      </ResponsiveDialog>

      <ConfirmDialog
        open={confirmRevoke !== null}
        onClose={() => setConfirmRevoke(null)}
        onConfirm={() => {
          if (confirmRevoke) {
            revokeToken.mutate(confirmRevoke.id)
          }
          setConfirmRevoke(null)
        }}
        question={t('user.page.settings.profile.tokens.confirm_revoke')}
        confirmLabel={t('user.page.settings.profile.tokens.revoke')}
        cancelLabel={t('common.button.cancel.label')}
        destructive
      />
    </SettingsShell>
  )
}
