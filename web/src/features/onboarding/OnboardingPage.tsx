import { useState } from 'react'
import type { ReactNode } from 'react'
import { Check, ChevronLeft, ChevronRight, Lightbulb } from 'lucide-react'
import { Trans, useTranslation } from 'react-i18next'
import { Link, useNavigate } from 'react-router'
import { Button } from '@/components/ui/button'
import { UserAvatar } from '@/components/UserAvatar'
import { AvatarPickerDialog } from '@/components/AvatarPickerDialog'
import { RouterPage } from '@/app/router-pages'
import { useUiStore } from '@/app/uiStore'
import { useIsCompact } from '@/hooks/useIsCompact'
import { useAccounts, useFolders } from '@/features/accounts/queries'
import { useTransactions } from '@/features/transactions/queries'
import { useCategories, usePayees, useTags } from '@/features/classifications/queries'
import { useConnections } from '@/features/connections/queries'
import { useBudgets } from '@/features/budgets/queries'
import { useCompleteOnboarding, useUserData } from '@/features/user/queries'
import { ImportCsvDialog } from '@/features/transactions/ImportCsvDialog'
import { ImportResultDialog } from '@/features/transactions/ImportResultDialog'
import type { AggregatedImportResult } from '@/features/transactions/importCsv'

function Step({
  id,
  done,
  avatar,
  idleIcon,
  title,
  guideText,
  guideHref,
  children,
}: {
  id: string
  done: boolean
  avatar?: string
  idleIcon?: ReactNode
  title: string
  guideText: string
  guideHref: string
  children: ReactNode
}) {
  return (
    <li data-testid={`step-${id}`} data-done={done} className="flex gap-3">
      <div className="flex flex-col items-center">
        <span
          className={`flex size-8 shrink-0 items-center justify-center rounded-full border ${done ? 'border-econumo-purple bg-econumo-purple text-white' : 'text-muted-foreground'}`}
        >
          {avatar ? (
            <UserAvatar avatar={avatar} size="sm" />
          ) : done ? (
            <Check className="size-4" />
          ) : (
            (idleIcon ?? <ChevronRight className="size-4" />)
          )}
        </span>
        <span className="w-px flex-1 bg-econumo-magenta-light" />
      </div>
      <div className="flex flex-col gap-1 pb-8">
        <a
          href={guideHref}
          target="_blank"
          rel="noreferrer"
          className="text-xs font-medium uppercase tracking-wide text-econumo-purple underline underline-offset-2"
        >
          {guideText}
        </a>
        <h2 className="text-lg font-semibold">{title}</h2>
        <div className="text-sm text-foreground/80">{children}</div>
      </div>
    </li>
  )
}

export function OnboardingPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const isCompact = useIsCompact()
  const openAccountModal = useUiStore((s) => s.openAccountModal)

  const { data: user } = useUserData()
  const { data: accounts = [] } = useAccounts()
  const { data: folders = [] } = useFolders()
  const { data: transactions = [] } = useTransactions()
  const { data: categories = [] } = useCategories()
  const { data: payees = [] } = usePayees()
  const { data: tags = [] } = useTags()
  const { data: connections = [] } = useConnections()
  const { data: budgets = [] } = useBudgets()
  const completeOnboarding = useCompleteOnboarding()

  const [importOpen, setImportOpen] = useState(false)
  const [importResult, setImportResult] = useState<AggregatedImportResult | null>(null)
  const [avatarOpen, setAvatarOpen] = useState(false)

  const isAccountCreated = accounts.length > 0
  const isTransactionsEntered = accounts.length > 0 && categories.length > 0 && transactions.length > 0
  const isClassificationsCreated = categories.length > 0 && (tags.length > 0 || payees.length > 0)
  const isConnectionsEstablished = connections.length > 0
  const isBudgetCreated = budgets.length > 0

  // Trans fills the children from the catalogue string's <tag>label</tag>
  const settingsLink = (to: string) => <Link to={to} className="text-econumo-purple underline underline-offset-2" />

  const handleComplete = () => {
    completeOnboarding.mutate(undefined, { onSuccess: () => navigate(RouterPage.BUDGET) })
  }

  return (
    <div className="flex h-full flex-col gap-3 p-4">
      {isCompact ? (
        <header className="flex items-center gap-2">
          <Button type="button" variant="ghost" size="icon" aria-label="back" onClick={() => navigate(RouterPage.HOME)}>
            <ChevronLeft className="size-5" />
          </Button>
          <h1 className="flex-1 truncate text-center text-lg uppercase">{t('onboarding.header')}</h1>
          <span className="min-w-9" />
        </header>
      ) : (
        <h1 className="border-b pb-2 text-[22px] uppercase tracking-wide">{t('onboarding.header')}</h1>
      )}

      <div className="min-h-0 flex-1 overflow-y-auto">
        <h2 className="pb-5 text-[32px] font-normal">{t('onboarding.title')}</h2>
        <ul className="flex flex-col">
          <Step
            id="accounts"
            done={isAccountCreated}
            title={t('onboarding.steps.accounts.title')}
            guideText={t('onboarding.user_guide.accounts')}
            guideHref="https://econumo.com/docs/user-guide/accounts"
          >
            <p>
              <Trans
                i18nKey="onboarding.steps.accounts.text"
                components={{
                  settings: settingsLink(RouterPage.SETTINGS),
                  accounts: settingsLink(RouterPage.SETTINGS_ACCOUNTS),
                }}
              />
            </p>
            <Button
              type="button"
              size="sm"
              className="mt-2 bg-econumo-yellow text-econumo-yellow-text hover:bg-econumo-yellow/85"
              onClick={() => openAccountModal({ folderId: folders[0]?.id ?? null })}
            >
              {t('onboarding.add_account')}
            </Button>
          </Step>

          <Step
            id="transactions"
            done={isTransactionsEntered}
            title={t('onboarding.steps.transactions.title')}
            guideText={t('onboarding.user_guide.transactions')}
            guideHref="https://econumo.com/docs/user-guide/transactions"
          >
            <p>
              <Trans
                i18nKey="onboarding.steps.transactions.text"
                components={{ action: <span className="font-medium text-foreground" /> }}
              />
              <br />
              {t('onboarding.steps.transactions.hint')}
            </p>
            <Button
              type="button"
              size="sm"
              className="mt-2 bg-econumo-yellow text-econumo-yellow-text hover:bg-econumo-yellow/85"
              onClick={() => setImportOpen(true)}
            >
              {t('onboarding.import_transactions')}
            </Button>
          </Step>

          <Step
            id="classifications"
            done={isClassificationsCreated}
            idleIcon={<Lightbulb className="size-4" />}
            title={t('onboarding.steps.classifications.title')}
            guideText={t('onboarding.user_guide.classifications')}
            guideHref="https://econumo.com/docs/user-guide/classifications"
          >
            <p>
              <Trans
                i18nKey="onboarding.steps.classifications.text"
                components={{
                  settings: settingsLink(RouterPage.SETTINGS),
                  categories: settingsLink(RouterPage.SETTINGS_CATEGORIES),
                  tags: settingsLink(RouterPage.SETTINGS_TAGS),
                  payees: settingsLink(RouterPage.SETTINGS_PAYEES),
                }}
              />
            </p>
          </Step>

          <Step
            id="avatar"
            done={false}
            avatar={user?.avatar}
            title={t('onboarding.steps.avatar.title')}
            guideText={t('onboarding.user_guide.user_profile')}
            guideHref="https://econumo.com/docs/user-guide/user-profile"
          >
            <p>
              <Trans
                i18nKey="onboarding.steps.avatar.text"
                components={{
                  choose: (
                    <button
                      type="button"
                      onClick={() => setAvatarOpen(true)}
                      className="text-econumo-purple underline underline-offset-2"
                    />
                  ),
                }}
              />
            </p>
          </Step>

          <Step
            id="connections"
            done={isConnectionsEstablished}
            title={t('onboarding.steps.connections.title')}
            guideText={t('onboarding.user_guide.shared_access')}
            guideHref="https://econumo.com/docs/user-guide/shared-access"
          >
            <p>
              <Trans
                i18nKey="onboarding.steps.connections.text"
                components={{
                  settings: settingsLink(RouterPage.SETTINGS),
                  connections: settingsLink(RouterPage.SETTINGS_CONNECTIONS),
                }}
              />
            </p>
          </Step>

          <Step
            id="budget"
            done={isBudgetCreated}
            title={t('onboarding.steps.budget.title')}
            guideText={t('onboarding.user_guide.budgets')}
            guideHref="https://econumo.com/docs/user-guide/budgets"
          >
            <p>
              <Trans
                i18nKey="onboarding.steps.budget.text"
                components={{ budget: settingsLink(RouterPage.BUDGET) }}
              />
              <br />
              <Trans
                i18nKey="onboarding.steps.budget.text_secondary"
                components={{
                  settings: settingsLink(RouterPage.SETTINGS),
                  budgets: settingsLink(RouterPage.SETTINGS_BUDGETS),
                }}
              />
            </p>
            <Button type="button" size="sm" className="mt-2" disabled={completeOnboarding.isPending} onClick={handleComplete}>
              {t('onboarding.complete')}
            </Button>
          </Step>
        </ul>
      </div>

      <ImportCsvDialog open={importOpen} onClose={() => setImportOpen(false)} onComplete={setImportResult} />
      <ImportResultDialog open={importResult !== null} result={importResult} onClose={() => setImportResult(null)} />
      <AvatarPickerDialog open={avatarOpen} onClose={() => setAvatarOpen(false)} />
    </div>
  )
}
