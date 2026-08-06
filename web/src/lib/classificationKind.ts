export const CLASSIFICATION_KINDS = ['tag', 'label'] as const
export type ClassificationKind = (typeof CLASSIFICATION_KINDS)[number]

/** Seeded server-side at create (model.DefaultTagIcon / model.DefaultLabelIcon);
 *  kept here only to preview a not-yet-saved row in the create dialog. Saved
 *  rows always render their stored icon. */
export const DEFAULT_ICON: Record<ClassificationKind, string> = {
  tag: 'tag',
  label: 'label',
}

/** Kept as a function (not a constant) so a future icon/colour picker can
 *  reintroduce per-row variation without re-threading every call site. */
export function kindAccentClass(_kind: ClassificationKind): string {
  return 'text-sky-600 dark:text-sky-400'
}
