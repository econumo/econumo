import { useEffect } from 'react'
import { useUiStore } from '@/app/uiStore'

// iPadOS 13+ also reports itself as Mac (see lib/platform.ts's isIOS); showing
// the Apple glyph there is still correct since it has the same ⌘ hardware key.
export function isMacPlatform(): boolean {
  return /Mac|iPhone|iPad|iPod/.test(navigator.userAgent)
}

export function searchShortcutLabel(): string {
  return isMacPlatform() ? '⌘K' : 'Ctrl+K'
}

export function useSearchHotkey() {
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      // a non-Latin layout (Cyrillic: л) is caught by the physical key; a Latin
      // one that moved K elsewhere (Dvorak: t) keeps its own chord
      const isK = e.key.toLowerCase() === 'k' || (e.code === 'KeyK' && !/^[a-z]$/i.test(e.key))
      if (!(e.metaKey || e.ctrlKey) || !isK) {
        return
      }
      const { searchOpen, openSearch } = useUiStore.getState()
      if (searchOpen) {
        // still ours: left alone the browser would move focus to its own search bar
        e.preventDefault()
        return
      }
      // never stack over a form or another dialog
      if (document.querySelector('[role="dialog"]')) {
        return
      }
      e.preventDefault()
      openSearch()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [])
}
