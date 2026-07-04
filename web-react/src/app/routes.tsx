import { createBrowserRouter } from 'react-router'
import { RequireAuth } from './RequireAuth'
import { LoginLayout } from './layouts/LoginLayout'
import { ApplicationLayout } from './layouts/ApplicationLayout'
import { NotFoundPage } from '@/pages/NotFoundPage'
import { LoginPage } from '@/features/auth/LoginPage'
import { RegistrationPage } from '@/features/auth/RegistrationPage'
import { LogoutPage } from '@/features/auth/LogoutPage'
import { HomePage } from '@/features/home/HomePage'

// Pages land here as Plans 2-6 build them; until then guarded paths show the empty shell.
const EmptyPage = () => <div />

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
            { path: '/account/:id', element: <EmptyPage /> },
            { path: '/budget', element: <EmptyPage /> },
            { path: '/onboarding', element: <EmptyPage /> },
            { path: '/settings', element: <EmptyPage /> },
            { path: '/settings/profile', element: <EmptyPage /> },
            { path: '/settings/profile/change-password', element: <EmptyPage /> },
            { path: '/settings/accounts', element: <EmptyPage /> },
            { path: '/settings/categories', element: <EmptyPage /> },
            { path: '/settings/payees', element: <EmptyPage /> },
            { path: '/settings/tags', element: <EmptyPage /> },
            { path: '/settings/connections', element: <EmptyPage /> },
            { path: '/settings/budgets', element: <EmptyPage /> },
          ],
        },
      ],
    },
    { path: '*', element: <NotFoundPage /> },
  ])
}
