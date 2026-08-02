import { enUS, es, ru, type Locale } from 'react-day-picker/locale'

// react-day-picker needs a date-fns Locale object; map our two-letter UI
// language onto one so calendar captions/weekdays follow the app language.
export function calendarLocale(lang: string): Locale {
  switch (lang) {
    case 'ru':
      return ru
    case 'es':
      return es
    default:
      return enUS
  }
}
