import { useEffect, useRef, useState } from 'react'
import type { TransactionFormState } from './useTransactionForm'

const DRAFT_KEY = 'econumo.transaction-draft'

// Counted from the moment the form was left, not from when it was opened.
export const DRAFT_TTL_MS = 2 * 60 * 1000

interface StoredDraft {
  savedAt: number
  form: TransactionFormState
}

// localStorage rather than memory or sessionStorage: an installed iOS app
// reloaded in the background starts cold, and the draft has to survive that.
// Storage can be full (the query cache lives there too) or blocked, and neither
// may break typing in the form, hence the catch-alls.
function saveDraft(form: TransactionFormState): void {
  try {
    localStorage.setItem(DRAFT_KEY, JSON.stringify({ savedAt: Date.now(), form } satisfies StoredDraft))
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
  if (typeof stored?.savedAt !== 'number' || !stored.form || Date.now() - stored.savedAt > DRAFT_TTL_MS) {
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
    saveDraft(form)
    savedThisForm.current = true
    // restamp at the moment the page goes away, however long the form sat idle
    const stamp = () => saveDraft(form)
    window.addEventListener('pagehide', stamp)
    return () => window.removeEventListener('pagehide', stamp)
  }, [enabled, form])

  return {
    offer,
    discard: () => {
      clearTransactionDraft()
      setOffer(null)
    },
    // restamps a form being closed, however long it sat idle
    stamp: () => {
      if (enabled && hasDraftContent(form)) {
        saveDraft(form)
      }
    },
  }
}
