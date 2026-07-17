import { enUS, ru, type Locale } from 'react-day-picker/locale'

// react-day-picker needs a date-fns Locale object; map our two-letter UI
// language onto one so calendar captions/weekdays follow the app language.
export function calendarLocale(lang: string): Locale {
  return lang === 'ru' ? ru : enUS
}
