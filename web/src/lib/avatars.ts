// The avatar value format is "<icon>:<color>" — a Material ligature name plus
// a color slug. The color list mirrors the backend allowlist in
// internal/user/avatar.go (order is contract; a sync test asserts equality).
export const avatarColors = ['red', 'orange', 'amber', 'emerald', 'teal', 'sky', 'fuchsia'] as const
export type AvatarColor = (typeof avatarColors)[number]

// Avatars render as a soft tint: a pale fill of the hue with the icon in the
// darker shade of the same hue — the large area stays quiet, only the glyph
// carries the color. The rendered values are the muted --color-avatar-*
// brand-harmonized pairs in index.css; the slugs themselves are the frozen
// backend allowlist.
export const avatarColorAccents: Record<AvatarColor, string> = {
  red: 'bg-avatar-red-tint text-avatar-red',
  orange: 'bg-avatar-orange-tint text-avatar-orange',
  amber: 'bg-avatar-amber-tint text-avatar-amber',
  emerald: 'bg-avatar-emerald-tint text-avatar-emerald',
  teal: 'bg-avatar-teal-tint text-avatar-teal',
  sky: 'bg-avatar-sky-tint text-avatar-sky',
  fuchsia: 'bg-avatar-fuchsia-tint text-avatar-fuchsia',
}

// Solid fills for the picker's color swatches (the avatar itself uses
// accents). The deep glyph hue, not a bright primary, so the swatch row
// previews the avatar's actual palette.
export const avatarColorSwatches: Record<AvatarColor, string> = {
  red: 'bg-avatar-red',
  orange: 'bg-avatar-orange',
  amber: 'bg-avatar-amber',
  emerald: 'bg-avatar-emerald',
  teal: 'bg-avatar-teal',
  sky: 'bg-avatar-sky',
  fuchsia: 'bg-avatar-fuchsia',
}

// The avatar picker's icon choices — a single IconPicker page (9 cols × 4
// rows), curated for character: expressive faces, animals, abstract shapes,
// and a few fun objects. Every name must exist in availableIcons (which is
// bounded by the bundled Material Symbols font), and the backend's
// RandomAvatarIcons mirrors this list exactly, names and order (both
// asserted by the sync test).
export const avatarIcons = [
  'face', 'sentiment_very_satisfied', 'sentiment_very_dissatisfied', 'sick',
  'psychology', 'smart_toy',
  'owl', 'cruelty_free', 'flutter_dash', 'emoji_nature', 'bug_report',
  'savings', 'egg',
  'auto_awesome', 'all_inclusive', 'extension', 'toys', 'gesture', 'hive',
  'blur_on',
  'rocket_launch', 'planet', 'cloud', 'bolt', 'wb_sunny', 'nightlight',
  'ac_unit', 'water_drop', 'local_fire_department', 'star', 'favorite',
  'diamond', 'crown', 'chess_knight', 'motorcycle', 'swords',
]

export const defaultAvatar = 'diamond:sky'

export function joinAvatar(icon: string, color: string): string {
  return `${icon}:${color}`
}

const isAvatarColor = (v: string): v is AvatarColor => (avatarColors as readonly string[]).includes(v)

export function splitAvatar(avatar: string): { icon: string; color: AvatarColor } {
  const at = avatar.lastIndexOf(':')
  const icon = at > 0 ? avatar.slice(0, at) : avatar
  const color = at > 0 ? avatar.slice(at + 1) : ''
  return { icon, color: isAvatarColor(color) ? color : 'sky' }
}
