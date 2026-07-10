import { useState } from 'react'
import type { ReactNode } from 'react'
import { Check, ChevronLeft, ChevronRight, Lightbulb } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate } from 'react-router'
import { Button } from '@/components/ui/button'
import { UserAvatar } from '@/components/UserAvatar'
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

  const isAccountCreated = accounts.length > 0
  const isTransactionsEntered = accounts.length > 0 && categories.length > 0 && transactions.length > 0
  const isClassificationsCreated = categories.length > 0 && (tags.length > 0 || payees.length > 0)
  const isConnectionsEstablished = connections.length > 0
  const isBudgetCreated = budgets.length > 0

  const settingsLink = (to: string, label: string) => (
    <Link to={to} className="text-econumo-purple underline underline-offset-2">
      {label}
    </Link>
  )

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
          <h1 className="flex-1 truncate text-center text-lg uppercase">{t('modules.user.pages.onboarding.header')}</h1>
          <span className="min-w-9" />
        </header>
      ) : (
        <h1 className="border-b pb-2 text-[22px] uppercase tracking-wide">{t('modules.user.pages.onboarding.header')}</h1>
      )}

      <div className="min-h-0 flex-1 overflow-y-auto">
        <h2 className="pb-5 text-[32px] font-normal">{t('modules.user.pages.onboarding.title')}</h2>
        <ul className="flex flex-col">
          <Step
            id="accounts"
            done={isAccountCreated}
            title="Add your accounts"
            guideText={t('modules.user.pages.onboarding.user_guide.accounts')}
            guideHref="https://econumo.com/docs/user-guide/accounts"
          >
            <p>
              To start, you can add an account by clicking the button below. Alternatively, you can always navigate to the{' '}
              {settingsLink(RouterPage.SETTINGS, t('pages.settings.settings.menu_item'))} {'->'}{' '}
              {settingsLink(RouterPage.SETTINGS_ACCOUNTS, t('pages.settings.accounts.menu_item'))} page to manage your accounts and
              arrange them into folders.
            </p>
            <Button
              type="button"
              size="sm"
              className="mt-2 bg-econumo-yellow text-econumo-yellow-text hover:bg-econumo-yellow/85"
              onClick={() => openAccountModal({ folderId: folders[0]?.id ?? null })}
            >
              {t('modules.user.pages.onboarding.add_account')}
            </Button>
          </Step>

          <Step
            id="transactions"
            done={isTransactionsEntered}
            title="Enter your first transaction"
            guideText={t('modules.user.pages.onboarding.user_guide.transactions')}
            guideHref="https://econumo.com/docs/user-guide/transactions"
          >
            <p>
              You can enter transactions by selecting any account in the left sidebar and clicking the{' '}
              <span className="font-medium text-foreground">Add Transaction</span> button.
              <br />
              You can create categories, tags, and payees directly from the transaction modal by entering their names and pressing
              Enter.
            </p>
            <Button
              type="button"
              size="sm"
              className="mt-2 bg-econumo-yellow text-econumo-yellow-text hover:bg-econumo-yellow/85"
              onClick={() => setImportOpen(true)}
            >
              {t('modules.user.pages.onboarding.import_transactions')}
            </Button>
          </Step>

          <Step
            id="classifications"
            done={isClassificationsCreated}
            idleIcon={<Lightbulb className="size-4" />}
            title="Manage categories, tags, and payees"
            guideText={t('modules.user.pages.onboarding.user_guide.classifications')}
            guideHref="https://econumo.com/docs/user-guide/classifications"
          >
            <p>
              To manage categories, tags, and payees, navigate to {settingsLink(RouterPage.SETTINGS, t('pages.settings.settings.menu_item'))}{' '}
              {'->'} {settingsLink(RouterPage.SETTINGS_CATEGORIES, t('modules.classifications.categories.pages.settings.menu_item'))},{' '}
              {settingsLink(RouterPage.SETTINGS_TAGS, t('modules.classifications.tags.pages.settings.menu_item'))}, or{' '}
              {settingsLink(RouterPage.SETTINGS_PAYEES, t('modules.classifications.payees.pages.settings.menu_item'))}. You can also
              sort or archive them as necessary.
            </p>
          </Step>

          <Step
            id="avatar"
            done={false}
            avatar={user?.avatar}
            title="Update your avatar"
            guideText={t('modules.user.pages.onboarding.user_guide.user_profile')}
            guideHref="https://econumo.com/docs/user-guide/user-profile"
          >
            <p>
              Econumo pulls your avatar from{' '}
              <a href="https://gravatar.com" target="_blank" rel="nofollow noreferrer" className="text-econumo-purple underline underline-offset-2">
                Gravatar
              </a>
              , linked to your email address. To change your avatar, please visit{' '}
              <a href="https://gravatar.com" target="_blank" rel="nofollow noreferrer" className="text-econumo-purple underline underline-offset-2">
                Gravatar
              </a>
              .
            </p>
          </Step>

          <Step
            id="connections"
            done={isConnectionsEstablished}
            title="Connect with your partner"
            guideText={t('modules.user.pages.onboarding.user_guide.shared_access')}
            guideHref="https://econumo.com/docs/user-guide/shared-access"
          >
            <p>
              To connect with your partner and manage shared access to your budget or accounts, please visit{' '}
              {settingsLink(RouterPage.SETTINGS, t('pages.settings.settings.menu_item'))} {'->'}{' '}
              {settingsLink(RouterPage.SETTINGS_CONNECTIONS, t('modules.connections.pages.settings.menu_item'))}.
            </p>
          </Step>

          <Step
            id="budget"
            done={isBudgetCreated}
            title="Create your budget"
            guideText={t('modules.user.pages.onboarding.user_guide.budgets')}
            guideHref="https://econumo.com/docs/user-guide/budgets"
          >
            <p>
              You can create your first budget on the {settingsLink(RouterPage.BUDGET, t('blocks.main.budget'))} page.
              <br />
              Additionally, you can access the {settingsLink(RouterPage.SETTINGS, t('pages.settings.settings.menu_item'))} {'->'}{' '}
              {settingsLink(RouterPage.SETTINGS_BUDGETS, t('modules.budget.page.settings.menu_item'))} page to manage your budgets,
              shared access, and more.
            </p>
            <Button type="button" size="sm" className="mt-2" disabled={completeOnboarding.isPending} onClick={handleComplete}>
              {t('modules.user.pages.onboarding.complete')}
            </Button>
          </Step>
        </ul>
      </div>

      <ImportCsvDialog open={importOpen} onClose={() => setImportOpen(false)} onComplete={setImportResult} />
      <ImportResultDialog open={importResult !== null} result={importResult} onClose={() => setImportResult(null)} />
    </div>
  )
}
