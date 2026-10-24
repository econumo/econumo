import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { CardField, cardFieldControlClass } from '@/components/CardField'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { useUiStore } from '@/app/uiStore'
import { useUserData } from '@/features/user/queries'
import { submitForm } from '@/lib/analytics'
import { getVersion } from '@/lib/config'
import { analyticsHost, deploymentKind, METRICS, trackEvent } from '@/lib/metrics'

const FEEDBACK_TYPES = ['feedback', 'feature_request', 'bug_report'] as const
type FeedbackType = (typeof FEEDBACK_TYPES)[number]

const MESSAGE_MAX = 4000

export function FeedbackDialog() {
  const open = useUiStore((s) => s.feedbackOpen)
  const close = useUiStore((s) => s.closeFeedback)
  // remounted per opening, so a sent or abandoned form starts empty
  return open ? <FeedbackForm onDone={close} /> : null
}

function FeedbackForm({ onDone }: { onDone: () => void }) {
  const { t } = useTranslation()
  const { data: user } = useUserData()
  const [type, setType] = useState<FeedbackType>('feedback')
  const [message, setMessage] = useState('')
  // empty by default: replying is never promised, so the address is opt-in
  const [email, setEmail] = useState('')
  const [messageError, setMessageError] = useState<string | null>(null)

  const send = useMutation({
    mutationFn: () =>
      submitForm(
        'feedback',
        {
          type,
          message: message.trim(),
          email: email.trim(),
          app_version: getVersion(),
          hosting: deploymentKind(),
        },
        { $host: analyticsHost() },
      ),
    onSuccess: () => {
      trackEvent(METRICS.FEEDBACK_SEND, { type, with_email: email.trim() !== '' })
      toast.success(t('feedback.dialog.sent'))
      onDone()
    },
    // the typed message stays in the form, so a retry loses nothing
    onError: () => toast.error(t('feedback.dialog.failed')),
  })

  const submit = () => {
    if (!message.trim()) {
      setMessageError(t('feedback.dialog.message_required'))
      return
    }
    send.mutate()
  }

  return (
    <ResponsiveDialog
      open
      onOpenChange={(o) => !o && onDone()}
      title={t('feedback.dialog.title')}
      description={t('feedback.dialog.no_reply')}
      footer={
        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={onDone}>
            {t('common.button.cancel.label')}
          </Button>
          <Button type="submit" form="feedback-form" disabled={send.isPending}>
            {t('feedback.dialog.send')}
          </Button>
        </div>
      }
    >
      <form
        id="feedback-form"
        className="flex flex-col gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <div className="flex rounded-lg bg-econumo-card p-1" role="radiogroup" aria-label={t('feedback.dialog.type')}>
          {FEEDBACK_TYPES.map((option) => (
            <button
              key={option}
              type="button"
              role="radio"
              aria-checked={type === option}
              className={`flex-1 rounded-md px-2 py-2 text-[13px] transition-colors ${
                type === option ? 'bg-econumo-magenta text-white' : 'text-muted-foreground hover:text-foreground'
              }`}
              onClick={() => setType(option)}
            >
              {t(`feedback.type.${option}`)}
            </button>
          ))}
        </div>

        <CardField label={t('feedback.dialog.message')} htmlFor="feedback-message" error={messageError}>
          <Textarea
            id="feedback-message"
            autoFocus
            className={`${cardFieldControlClass} min-h-32 resize-none`}
            placeholder={t(`feedback.placeholder.${type}`)}
            maxLength={MESSAGE_MAX}
            value={message}
            onChange={(e) => {
              setMessage(e.target.value)
              setMessageError(null)
            }}
          />
        </CardField>

        <div className="flex flex-col gap-1">
          <CardField label={t('feedback.dialog.email')} htmlFor="feedback-email">
            <Input
              id="feedback-email"
              type="email"
              autoComplete="email"
              className={cardFieldControlClass}
              placeholder={t('feedback.dialog.email_placeholder')}
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
          </CardField>
          {user?.email && email !== user.email ? (
            <button type="button" className="self-start px-1 text-sm text-econumo-link hover:underline" onClick={() => setEmail(user.email)}>
              {t('feedback.dialog.use_my_email')}
            </button>
          ) : null}
        </div>
      </form>
    </ResponsiveDialog>
  )
}
