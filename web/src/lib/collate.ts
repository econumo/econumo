// Bare localeCompare() collates with the HOST's locale, not the app language,
// so a name list sorts by whatever the browser happens to be set to. The gap is
// invisible in Latin scripts but not elsewhere: Chinese collates by pinyin, an
// order no host-default comparison produces.
const collators = new Map<string, Intl.Collator>()

export function compareNames(a: string, b: string, lang = 'en'): number {
  let collator = collators.get(lang)
  if (!collator) {
    collator = new Intl.Collator(lang)
    collators.set(lang, collator)
  }
  return collator.compare(a, b)
}
