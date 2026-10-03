import { useEffect } from 'react'
import { useUiStore } from '@/app/uiStore'

export function useSearchHotkey() {
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (!(e.metaKey || e.ctrlKey) || e.key.toLowerCase() !== 'k') {
        return
      }
      const { searchOpen, openSearch } = useUiStore.getState()
      // never stack over a form or another dialog
      if (searchOpen || document.querySelector('[role="dialog"]')) {
        return
      }
      e.preventDefault()
      openSearch()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [])
}
