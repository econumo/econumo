export const THEME_STORAGE_KEY = 'econumo.theme'
export const themePreferences = ['system', 'light', 'dark'] as const
export type ThemePreference = (typeof themePreferences)[number]

// Appearance belongs to this browser or app installation. An absent or
// unknown local preference follows the device's system appearance.
export function themePreference(value: string | null | undefined): ThemePreference {
  return value === 'light' || value === 'dark' ? value : 'system'
}

export function storedThemePreference(): ThemePreference {
  try {
    return themePreference(localStorage.getItem(THEME_STORAGE_KEY))
  } catch {
    return 'system'
  }
}
