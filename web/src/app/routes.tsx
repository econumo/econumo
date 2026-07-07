import { createBrowserRouter } from 'react-router'
import { RequireAuth } from './RequireAuth'
import { LoginLayout } from './layouts/LoginLayout'
import { ApplicationLayout } from './layouts/ApplicationLayout'
import { NotFoundPage } from '@/pages/NotFoundPage'
import { LoginPage } from '@/features/auth/LoginPage'
import { RegistrationPage } from '@/features/auth/RegistrationPage'
import { LogoutPage } from '@/features/auth/LogoutPage'
import { HomePage } from '@/features/home/HomePage'
import { AccountPage } from '@/features/accounts/AccountPage'
import { SettingsPage } from '@/features/settings/SettingsPage'
import { ProfilePage } from '@/features/settings/ProfilePage'
import { ChangePasswordPage } from '@/features/settings/ChangePasswordPage'
import { AccountsSettingsPage } from '@/features/accounts/AccountsSettingsPage'
import { CategoriesPage } from '@/features/classifications/CategoriesPage'
import { PayeesPage } from '@/features/classifications/PayeesPage'
import { TagsPage } from '@/features/classifications/TagsPage'
import { BudgetsPage } from '@/features/budgets/BudgetsPage'
import { ConnectionsPage } from '@/features/connections/ConnectionsPage'
import { OnboardingPage } from '@/features/onboarding/OnboardingPage'
import { BudgetPage } from '@/features/budgets/BudgetPage'

export function createRouter() {
  return createBrowserRouter([
    {
      element: <LoginLayout />,
      children: [
        { path: '/login', element: <LoginPage /> },
        { path: '/register', element: <RegistrationPage /> },
      ],
    },
    { path: '/logout', element: <LogoutPage /> },
    {
      element: <RequireAuth />,
      children: [
        {
          element: <ApplicationLayout />,
          children: [
            { path: '/', element: <HomePage /> },
            { path: '/account/:id', element: <AccountPage /> },
            { path: '/budget', element: <BudgetPage /> },
            { path: '/onboarding', element: <OnboardingPage /> },
            { path: '/settings', element: <SettingsPage /> },
            { path: '/settings/profile', element: <ProfilePage /> },
            { path: '/settings/profile/change-password', element: <ChangePasswordPage /> },
            { path: '/settings/accounts', element: <AccountsSettingsPage /> },
            { path: '/settings/categories', element: <CategoriesPage /> },
            { path: '/settings/payees', element: <PayeesPage /> },
            { path: '/settings/tags', element: <TagsPage /> },
            { path: '/settings/connections', element: <ConnectionsPage /> },
            { path: '/settings/budgets', element: <BudgetsPage /> },
          ],
        },
      ],
    },
    { path: '*', element: <NotFoundPage /> },
  ])
}
