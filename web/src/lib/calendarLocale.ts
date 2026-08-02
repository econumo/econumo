import { de, enUS, fr, it, pt, ru, type Locale } from 'react-day-picker/locale'

// react-day-picker needs a date-fns Locale object; map our two-letter UI
// language onto one so calendar captions/weekdays follow the app language.
const locales: Record<string, Locale> = { ru, de, fr, it, pt }

export function calendarLocale(lang: string): Locale {
  return locales[lang] ?? enUS
}
