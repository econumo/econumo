import { enUS, pl, ru, type Locale } from 'react-day-picker/locale'

// react-day-picker needs a date-fns Locale object; map our two-letter UI
// language onto one so calendar captions/weekdays follow the app language.
const locales: Record<string, Locale> = { ru, pl }

export function calendarLocale(lang: string): Locale {
  return locales[lang] ?? enUS
}
