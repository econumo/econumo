import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import enUS from '@/locales/en-US'
import { locale } from '@/lib/config'

i18n.use(initReactI18next).init({
  lng: locale(),
  fallbackLng: 'en',
  resources: {
    en: { translation: enUS },
  },
  interpolation: {
    escapeValue: false,
    prefix: '{',
    suffix: '}',
  },
  returnNull: false,
})

export default i18n
