// Reporting leads: it is the kind most users want (many per transaction, no
// budget maths), so it is offered first and selected by default.
export const CLASSIFICATION_KINDS = ['label', 'tag'] as const
export type ClassificationKind = (typeof CLASSIFICATION_KINDS)[number]

/** Seeded server-side at create (model.DefaultTagIcon / model.DefaultLabelIcon);
 *  kept here only to preview a not-yet-saved row in the create dialog. Saved
 *  rows always render their stored icon. */
export const DEFAULT_ICON: Record<ClassificationKind, string> = {
  tag: 'tag',
  label: 'label',
}
