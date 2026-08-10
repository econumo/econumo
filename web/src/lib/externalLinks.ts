import { isNativeApp, nativePlugin } from './platform'

interface BrowserPlugin {
  open(o: { url: string }): Promise<void>
}

// In-app navigation uses relative hrefs (react-router), so any absolute
// http(s) anchor is by definition external and must leave the WebView.
export function installExternalLinkInterceptor(): void {
  if (!isNativeApp()) {
    return
  }
  document.addEventListener('click', (e) => {
    if (e.defaultPrevented) {
      return
    }
    const anchor = (e.target as Element | null)?.closest?.('a[href]')
    if (!anchor) {
      return
    }
    const href = anchor.getAttribute('href') ?? ''
    if (!/^https?:\/\//.test(href)) {
      return
    }
    e.preventDefault()
    void nativePlugin<BrowserPlugin>('Browser')?.open({ url: href })
  })
}
