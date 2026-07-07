import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { RouterProvider } from 'react-router'
import { PersistQueryClientProvider } from '@tanstack/react-query-persist-client'
import '@/app/i18n'
import './index.css'
import { createRouter } from '@/app/routes'
import { createAppQueryClient, createPersistOptions, refreshRestoredQueries } from '@/lib/queryPersist'

const queryClient = createAppQueryClient()

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <PersistQueryClientProvider
      client={queryClient}
      persistOptions={createPersistOptions()}
      onSuccess={() => refreshRestoredQueries(queryClient)}
    >
      <RouterProvider router={createRouter()} />
    </PersistQueryClientProvider>
  </StrictMode>,
)
