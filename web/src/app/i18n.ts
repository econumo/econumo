import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import en from '../../../locales/en.json'
import ru from '../../../locales/ru.json'
import de from '../../../locales/de.json'
import fr from '../../../locales/fr.json'
import it from '../../../locales/it.json'
import pt from '../../../locales/pt.json'
import pl from '../../../locales/pl.json'
import nl from '../../../locales/nl.json'
import uk from '../../../locales/uk.json'
import es from '../../../locales/es.json'
import zh from '../../../locales/zh.json'
import { locale } from '@/lib/config'

i18n.use(initReactI18next).init({
  lng: locale(),
  fallbackLng: 'en',
  resources: {
    en: { translation: en },
    ru: { translation: ru },
    de: { translation: de },
    fr: { translation: fr },
    it: { translation: it },
    pt: { translation: pt },
    pl: { translation: pl },
    nl: { translation: nl },
    uk: { translation: uk },
    es: { translation: es },
    zh: { translation: zh },
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
