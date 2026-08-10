import type { createBrowserRouter } from 'react-router'

type AppRouter = ReturnType<typeof createBrowserRouter>

// Lets non-React modules (the axios 401 interceptor) navigate through the SPA
// router. Full page loads to non-root paths are fragile in a packaged WebView
// and waste a reload on the web.
let router: AppRouter | null = null

export function setRouter(r: AppRouter | null): void {
  router = r
}

export function navigateTo(path: string): void {
  if (router) {
    void router.navigate(path)
    return
  }
  window.location.assign(path)
}
