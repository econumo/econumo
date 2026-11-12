import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { RouterProvider } from 'react-router'
import { PersistQueryClientProvider } from '@tanstack/react-query-persist-client'
import '@/app/i18n'
import './index.css'
import { createRouter } from '@/app/routes'
import { createPersistOptions, refreshRestoredQueries } from '@/lib/queryPersist'
import { queryClient } from '@/app/queryClient'
import { attemptedWritesResolved, holdRestoredReads, startPendingWrites } from '@/features/pendingWrites/sender'
import { setRouter } from '@/app/routerRef'
import { registerServiceWorker } from '@/lib/serviceWorker'
import { listenForInstallPrompt } from '@/features/install/installPrompt'
import { initAppLock } from '@/features/appLock/lockStore'

registerServiceWorker()
listenForInstallPrompt()

initAppLock()
const router = createRouter()
setRouter(router)
createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <PersistQueryClientProvider
      client={queryClient}
      persistOptions={createPersistOptions()}
      onSuccess={async () => {
        startPendingWrites(queryClient)
        const resolved = attemptedWritesResolved()
        // the provider keeps the observers from mounting until this settles
        await holdRestoredReads(queryClient)
        void resolved.then(() => refreshRestoredQueries(queryClient))
      }}
      onError={() => { startPendingWrites(queryClient) }}
    >
      <RouterProvider router={router} />
    </PersistQueryClientProvider>
  </StrictMode>,
)
