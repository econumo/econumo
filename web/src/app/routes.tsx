import { createBrowserRouter } from 'react-router'
import { RootLayout } from './layouts/RootLayout'
import { RequireAuth } from './RequireAuth'
import { LoginLayout } from './layouts/LoginLayout'
import { ApplicationLayout } from './layouts/ApplicationLayout'
import { NotFoundPage } from '@/pages/NotFoundPage'
import { LoginPage } from '@/features/auth/LoginPage'
import { RegistrationPage } from '@/features/auth/RegistrationPage'
import { LogoutPage } from '@/features/auth/LogoutPage'
import { OAuthCallbackPage } from '@/features/auth/OAuthCallbackPage'
import { AppReturnPage } from '@/features/auth/AppReturnPage'
import { HomePage } from '@/features/home/HomePage'
import { AccountPage } from '@/features/accounts/AccountPage'
import { SettingsPage } from '@/features/settings/SettingsPage'
import { ProfilePage } from '@/features/settings/ProfilePage'
import { ChangePasswordPage } from '@/features/settings/ChangePasswordPage'
import { ChangeEmailPage } from '@/features/settings/ChangeEmailPage'
import { SessionsPage } from '@/features/settings/SessionsPage'
import { PersonalTokensPage } from '@/features/settings/PersonalTokensPage'
import { LinkedAccountsPage } from '@/features/settings/LinkedAccountsPage'
import { AccountsSettingsPage } from '@/features/accounts/AccountsSettingsPage'
import { CategoriesPage } from '@/features/classifications/CategoriesPage'
import { PayeesPage } from '@/features/classifications/PayeesPage'
import { TagsPage } from '@/features/classifications/TagsPage'
import { CurrenciesPage } from '@/features/currencies/CurrenciesPage'
import { BudgetsPage } from '@/features/budgets/BudgetsPage'
import { ConnectionsPage } from '@/features/connections/ConnectionsPage'
import { RecurringSettingsPage } from '@/features/recurring/RecurringSettingsPage'
import { ImportsDataPage } from '@/features/imports/ImportsDataPage'
import { AppleWalletHistoryPage } from '@/features/imports/AppleWalletHistoryPage'
import { AppleWalletPage } from '@/features/imports/AppleWalletPage'
import { SimpleFINHistoryPage } from '@/features/imports/SimpleFINHistoryPage'
import { SimpleFINPage } from '@/features/imports/SimpleFINPage'
import { InboxPage } from '@/features/inbox/InboxPage'
import { ImportRunPage } from '@/features/imports/ImportRunPage'
import { ImportRulesPage } from '@/features/imports/ImportRulesPage'
import { OnboardingPage } from '@/features/onboarding/OnboardingPage'
import { BudgetPage } from '@/features/budgets/BudgetPage'
import { ReportsPage } from '@/features/reports/ReportsPage'
import { ConnectedAppsPage } from '@/features/authserver/ConnectedAppsPage'
import { ConsentPage } from '@/features/authserver/ConsentPage'
import { isAppleWalletImportEnabled, isSimpleFINImportEnabled, isTransactionImportEnabled } from '@/lib/config'

export function createRouter() {
  return createBrowserRouter([
    {
      element: <RootLayout />,
      children: [
        {
          element: <LoginLayout />,
          children: [
            { path: '/login', element: <LoginPage /> },
            { path: '/register', element: <RegistrationPage /> },
          ],
        },
        { path: '/logout', element: <LogoutPage /> },
        { path: '/oauth/callback', element: <OAuthCallbackPage /> },
        { path: '/oauth/app-return', element: <AppReturnPage /> },
        {
          element: <RequireAuth />,
          children: [
            { path: '/oauth/authorize', element: <ConsentPage /> },
            {
              element: <ApplicationLayout />,
              children: [
                { path: '/', element: <HomePage /> },
                { path: '/account/:id', element: <AccountPage /> },
                { path: '/budget', element: <BudgetPage key="budget" mode="budget" /> },
                { path: '/plan', element: <BudgetPage key="plan" mode="plan" /> },
                { path: '/reports', element: <ReportsPage /> },
                { path: '/onboarding', element: <OnboardingPage /> },
                { path: '/settings', element: <SettingsPage /> },
                { path: '/settings/profile', element: <ProfilePage /> },
                { path: '/settings/profile/change-password', element: <ChangePasswordPage /> },
                { path: '/settings/profile/change-email', element: <ChangeEmailPage /> },
                { path: '/settings/profile/sessions', element: <SessionsPage /> },
                { path: '/settings/profile/tokens', element: <PersonalTokensPage /> },
                { path: '/settings/profile/connected-apps', element: <ConnectedAppsPage /> },
                { path: '/settings/profile/linked-accounts', element: <LinkedAccountsPage /> },
                { path: '/settings/accounts', element: <AccountsSettingsPage /> },
                { path: '/settings/categories', element: <CategoriesPage /> },
                { path: '/settings/payees', element: <PayeesPage /> },
                { path: '/settings/tags', element: <TagsPage /> },
                { path: '/settings/currencies', element: <CurrenciesPage /> },
                { path: '/settings/connections', element: <ConnectionsPage /> },
                { path: '/settings/budgets', element: <BudgetsPage /> },
                { path: '/settings/recurring', element: <RecurringSettingsPage /> },
                { path: '/settings/data', element: <ImportsDataPage /> },
                { path: '/inbox', element: <InboxPage /> },
                ...(isAppleWalletImportEnabled()
                  ? [
                      { path: '/settings/apple-wallet', element: <AppleWalletPage /> },
                      { path: '/settings/apple-wallet/history', element: <AppleWalletHistoryPage /> },
                    ]
                  : []),
                ...(isSimpleFINImportEnabled()
                  ? [
                      { path: '/settings/simplefin', element: <SimpleFINPage /> },
                      { path: '/settings/simplefin/history', element: <SimpleFINHistoryPage /> },
                      { path: '/settings/simplefin/history/:id', element: <ImportRunPage /> },
                    ]
                  : []),
                ...(isTransactionImportEnabled()
                  ? [
                      { path: '/settings/import-rules', element: <ImportRulesPage /> },
                    ]
                  : []),
              ],
            },
          ],
        },
        { path: '*', element: <NotFoundPage /> },
      ],
    },
  ])
}
