import { useTranslation } from 'react-i18next'
import { NextStepItem } from '@/features/onboarding/GettingStartedCards'
import { useGettingStartedActions } from '@/features/onboarding/useGettingStartedActions'
import { isOpenStep, useGettingStarted } from '@/features/onboarding/useGettingStarted'
import { InboxSection } from './InboxSection'

// Nothing here waits on the user, so it stays out of the Inbox count.
export function NextStepsSection() {
  const { t } = useTranslation()
  const gettingStarted = useGettingStarted()
  const actions = useGettingStartedActions('inbox')
  const open = gettingStarted.inboxSteps.filter(isOpenStep)
  return (
    <>
      {gettingStarted.active && open.length > 0 ? (
        <InboxSection title={t('inbox.sections.next_steps')}>
          <div className="flex flex-col gap-2">
            {open.map((s) => (
              <NextStepItem key={s.key} step={s.key} onOpen={() => actions.open(s.key)} onDismiss={() => gettingStarted.dismiss(s.key)} />
            ))}
          </div>
        </InboxSection>
      ) : null}
      {actions.dialogs}
    </>
  )
}
