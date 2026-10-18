export const themePreferences = ['system', 'light', 'dark'] as const
export type ThemePreference = (typeof themePreferences)[number]

// Missing (including accounts created before themes) or unknown options use
// the device preference. The database is authoritative; localStorage only
// caches the last appearance to avoid a flash while the user is loading.
export function themePreference(value: string | null | undefined): ThemePreference {
  return value === 'light' || value === 'dark' ? value : 'system'
}
