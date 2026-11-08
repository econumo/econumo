import { useEffect, useRef, useState } from 'react'
import { getSessionUserId } from '@/lib/storage'
import type { TransactionFormState } from './useTransactionForm'

const DRAFT_KEY = 'econumo.transaction-draft'

// Counted from the moment the form was left, not from when it was opened.
export const DRAFT_TTL_MS = 2 * 60 * 1000

interface StoredDraft {
  savedAt: number
  // A tab left open across a sign-out can still write after the purge, so the
  // draft names whose it is.
  userId: string | null
  form: TransactionFormState
}

// localStorage rather than memory or sessionStorage: an installed iOS app
// reloaded in the background starts cold, and the draft has to survive that.
// Storage can be full (the query cache lives there too) or blocked, and neither
// may break typing in the form, hence the catch-alls.
function saveDraft(form: TransactionFormState, userId: string | null): void {
  try {
    localStorage.setItem(DRAFT_KEY, JSON.stringify({ savedAt: Date.now(), userId, form } satisfies StoredDraft))
  } catch {
    // no draft this time
  }
}

export function clearTransactionDraft(): void {
  try {
    localStorage.removeItem(DRAFT_KEY)
  } catch {
    // nothing to clear
  }
}

export function loadTransactionDraft(): TransactionFormState | null {
  let stored: Partial<StoredDraft> | null = null
  try {
    stored = JSON.parse(localStorage.getItem(DRAFT_KEY) ?? 'null') as Partial<StoredDraft> | null
  } catch {
    // unreadable, dropped below
  }
  if (typeof stored?.savedAt !== 'number' || !stored.form || stored.userId !== getSessionUserId() || Date.now() - stored.savedAt > DRAFT_TTL_MS) {
    clearTransactionDraft()
    return null
  }
  return stored.form
}

// The type, account and date start filled in, so changing only those is not
// worth asking about later.
export function hasDraftContent(form: TransactionFormState): boolean {
  return (
    form.amount.trim() !== '' ||
    form.description.trim() !== '' ||
    form.accountRecipientId !== null ||
    form.categoryId !== null ||
    form.payeeId !== null ||
    form.tagId !== null ||
    form.labelIds.length > 0
  )
}

export function useTransactionDraft(form: TransactionFormState, enabled: boolean) {
  const [offer, setOffer] = useState(() => (enabled ? loadTransactionDraft() : null))
  // Once this form has content it replaces the draft in storage, so the offer
  // goes away for good; emptying the form again must not bring it back.
  if (offer && hasDraftContent(form)) {
    setOffer(null)
  }

  // the user this form was opened for, not whoever holds the session at save time
  const [userId] = useState(getSessionUserId)
  const savedThisForm = useRef(false)
  useEffect(() => {
    if (!enabled) {
      return
    }
    if (!hasDraftContent(form)) {
      // Only a draft this form wrote: an empty form that is still offering the
      // previous one must leave it in place.
      if (savedThisForm.current) {
        clearTransactionDraft()
        savedThisForm.current = false
      }
      return
    }
    saveDraft(form, userId)
    savedThisForm.current = true
    // Restamp when the page goes away, however long the form sat idle. iOS
    // kills a backgrounded app without pagehide; hiding it is the last signal.
    const stamp = () => saveDraft(form, userId)
    const stampIfHidden = () => {
      if (document.visibilityState === 'hidden') {
        stamp()
      }
    }
    window.addEventListener('pagehide', stamp)
    document.addEventListener('visibilitychange', stampIfHidden)
    return () => {
      window.removeEventListener('pagehide', stamp)
      document.removeEventListener('visibilitychange', stampIfHidden)
    }
  }, [enabled, form, userId])

  return {
    offer,
    // Closing the form with the offer still up is the answer to it: the draft
    // goes. A form with content becomes the draft, restamped at this moment
    // however long it sat idle.
    leave: () => {
      if (offer) {
        clearTransactionDraft()
      } else if (enabled && hasDraftContent(form)) {
        saveDraft(form, userId)
      }
    },
  }
}
