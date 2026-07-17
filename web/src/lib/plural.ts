// The catalog keeps pipe plurals verbatim ("one | many", ru: "one | few | many");
// i18next doesn't parse them, so pick the variant here.
export function pluralPick(catalogValue: string, count: number, lang = 'en'): string {
  const variants = catalogValue.split(' | ')
  let chosen: string
  if (variants.length >= 3) {
    const rule = new Intl.PluralRules(lang).select(count)
    chosen = rule === 'one' ? variants[0] : rule === 'few' ? variants[1] : variants[variants.length - 1]
  } else {
    chosen = count === 1 ? variants[0] : variants[variants.length - 1]
  }
  return chosen.replaceAll('{count}', String(count))
}
