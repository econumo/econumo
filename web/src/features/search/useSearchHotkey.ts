import { useEffect } from 'react'
import { useUiStore } from '@/app/uiStore'

export function useSearchHotkey() {
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (!(e.metaKey || e.ctrlKey) || e.key.toLowerCase() !== 'k') {
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
