import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import en from '../../../locales/en.json'
import ru from '../../../locales/ru.json'
import { locale } from '@/lib/config'

i18n.use(initReactI18next).init({
  lng: locale(),
  fallbackLng: 'en',
  resources: {
    en: { translation: en },
    ru: { translation: ru },
  },
  interpolation: {
    escapeValue: false,
    prefix: '{',
    suffix: '}',
  },
  returnNull: false,
})

document.documentElement.lang = locale()

export default i18n
