import { useCallback, useSyncExternalStore } from 'react'
import { onlineManager } from '@tanstack/react-query'

export function useOnline(): boolean {
  return useSyncExternalStore(
    useCallback((onChange: () => void) => onlineManager.subscribe(onChange), []),
    () => onlineManager.isOnline(),
  )
}
