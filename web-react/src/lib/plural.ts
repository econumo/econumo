// The catalog keeps vue-i18n pipe plurals verbatim ("one | many"); i18next
// doesn't parse them, so pick the variant here.
export function pluralPick(catalogValue: string, count: number): string {
  const variants = catalogValue.split(' | ')
  const chosen = count === 1 ? variants[0] : variants[variants.length - 1]
  return chosen.replaceAll('{count}', String(count))
}
