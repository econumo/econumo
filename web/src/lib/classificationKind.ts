export const CLASSIFICATION_KINDS = ['tag', 'label'] as const
export type ClassificationKind = (typeof CLASSIFICATION_KINDS)[number]

/** Seeded server-side at create (model.DefaultTagIcon / model.DefaultLabelIcon);
 *  kept here only to preview a not-yet-saved row in the create dialog. Saved
 *  rows always render their stored icon. */
export const DEFAULT_ICON: Record<ClassificationKind, string> = {
  tag: 'tag',
  label: 'label',
}

/** Colour encodes the KIND (budgeting vs reporting), not identity, so it is a
 *  constant rather than a stored field. */
export function kindAccentClass(kind: ClassificationKind): string {
  return kind === 'tag' ? 'text-sky-600 dark:text-sky-400' : 'text-violet-600 dark:text-violet-400'
}
