import { describe, expect, it } from 'vitest'

// Offline, only transactions and limits are written (they queue). Every other
// write needs the server, so a source file that fires a mutation must read
// useOnline and disable the control, or be listed here with the reason it
// doesn't need to.
const OFFLINE_EXEMPT: Record<string, string> = {
  'src/features/auth/LoginPage.tsx': 'signed-out flow: nothing to sync, sign-in needs the server anyway',
  'src/features/auth/RegistrationPage.tsx': 'signed-out flow',
  'src/features/auth/RecoveryDialog.tsx': 'signed-out flow',
  'src/features/auth/VerifyEmailDialog.tsx': 'signed-out flow',
  'src/features/auth/OAuthCallbackPage.tsx': 'signed-out flow, runs on a redirect from the provider',
  'src/features/auth/ProviderButtons.tsx': 'signed-out flow',
  'src/features/whatsNew/WhatsNewNotice.tsx': 'dismissal is best effort and fires no visible write control',
}

// Files whose every write sits behind a shared component that reads useOnline
// itself: ConfirmDialog/PromptDialog disable their confirm button offline unless
// the caller passes offlineOk (a queued write), ClassificationList disables
// create, reorder, switches and row actions, AccessLevelDialog its choices.
const OFFLINE_VIA_SHARED: Record<string, string[]> = {
  'src/features/accounts/useAccountActions.tsx': ['<ConfirmDialog', '<AccessLevelDialog'],
  'src/features/classifications/CategoriesPage.tsx': ['<ClassificationList'],
  'src/features/classifications/PayeesPage.tsx': ['<ClassificationList'],
  'src/features/classifications/TagsPage.tsx': ['<ClassificationList'],
  'src/features/imports/ImportRulesPage.tsx': ['<ClassificationList'],
}

const sources = import.meta.glob('/src/**/*.tsx', {
  eager: true,
  query: '?raw',
  import: 'default',
}) as Record<string, string>

describe('offline coverage', () => {
  const writers = Object.entries(sources)
    .filter(([path]) => !/\.test\.tsx$/.test(path) && !path.startsWith('/src/test/'))
    .filter(([, content]) => /\.mutate(Async)?\(/.test(content))
    .map(([path, content]) => [path.slice(1), content] as const)

  it('every file that fires a mutation reads useOnline (or is exempt with a reason)', () => {
    const missing = writers
      .filter(([path]) => !(path in OFFLINE_EXEMPT))
      // offlineOk: the only write in the file is a queued transaction delete
      .filter(([, content]) => !content.includes('useOnline') && !content.includes('offlineOk'))
      .filter(([path, content]) => !(OFFLINE_VIA_SHARED[path] ?? []).some((marker) => content.includes(marker)))
      .map(([path]) => path)
    expect(missing).toEqual([])
  })

  it('shared-component entries still render a gated component', () => {
    const byPath = new Map(Object.entries(sources).map(([p, c]) => [p.slice(1), c]))
    for (const [path, markers] of Object.entries(OFFLINE_VIA_SHARED)) {
      expect(markers.some((m) => byPath.get(path)?.includes(m)), path).toBe(true)
    }
  })

  it('exemptions name a reason and a file that still writes without useOnline', () => {
    const byPath = new Map(writers)
    for (const [path, reason] of Object.entries(OFFLINE_EXEMPT)) {
      expect(reason.length).toBeGreaterThan(0)
      expect(byPath.has(path), `${path} no longer fires a mutation`).toBe(true)
      expect(byPath.get(path)).not.toContain('useOnline')
    }
  })
})
