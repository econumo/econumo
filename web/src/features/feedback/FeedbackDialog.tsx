import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { Plus, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { CardField, cardFieldControlClass } from '@/components/CardField'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { useUiStore } from '@/app/uiStore'
import type { FeedbackType } from '@/app/uiStore'
import { useUserData } from '@/features/user/queries'
import { submitForm } from '@/lib/analytics'
import { getVersion } from '@/lib/config'
import { analyticsHost, deploymentKind, METRICS, trackEvent } from '@/lib/metrics'

const FEEDBACK_TYPES: readonly FeedbackType[] = ['feedback', 'feature_request', 'bug_report']

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
  const [type, setType] = useState<FeedbackType>(() => useUiStore.getState().feedbackType)
  const topic = useUiStore((s) => s.feedbackTopic)
  const [message, setMessage] = useState('')
  // empty by default: replying is never promised, so the address is opt-in
  const [email, setEmail] = useState('')
  const [messageError, setMessageError] = useState<string | null>(null)
  const suggested = user?.email ?? ''

  const send = useMutation({
    mutationFn: () =>
      submitForm(
        'feedback',
        {
          type,
          topic,
          message: message.trim(),
          email: email.trim(),
          app_version: getVersion(),
          hosting: deploymentKind(),
        },
        { $host: analyticsHost() },
      ),
    onSuccess: () => {
      trackEvent(METRICS.FEEDBACK_SEND, { type, topic, with_email: email.trim() !== '' })
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

        <CardField label={t('feedback.dialog.email')} htmlFor="feedback-email">
          <div className="flex items-center gap-2">
            <Input
              id="feedback-email"
              type="email"
              autoComplete="email"
              className={`${cardFieldControlClass} min-w-0 flex-1`}
              // the suggestion chip stands in for the placeholder while it shows
              placeholder={suggested ? '' : t('feedback.dialog.email_placeholder')}
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
            {email ? (
              <button
                type="button"
                aria-label={t('feedback.dialog.clear_email')}
                title={t('feedback.dialog.clear_email')}
                className="-my-1 grid size-6 shrink-0 place-items-center rounded-full text-muted-foreground hover:bg-econumo-hover hover:text-foreground"
                onClick={() => setEmail('')}
              >
                <X className="size-4" />
              </button>
            ) : suggested ? (
              <button
                type="button"
                aria-label={t('feedback.dialog.use_email', { email: suggested })}
                title={t('feedback.dialog.use_email', { email: suggested })}
                className="-my-1 flex min-w-0 max-w-[70%] shrink items-center gap-1 rounded-full bg-primary/10 px-2.5 py-1 text-xs text-econumo-link hover:bg-primary/15"
                onClick={() => setEmail(suggested)}
              >
                <Plus className="size-3.5 shrink-0" />
                <span className="truncate">{suggested}</span>
              </button>
            ) : null}
          </div>
        </CardField>
      </form>
    </ResponsiveDialog>
  )
}
