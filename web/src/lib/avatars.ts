// The avatar value format is "<icon>:<color>" — a Material ligature name plus
// a color slug. The color list mirrors the backend allowlist in
// internal/user/avatar.go (order is contract; a sync test asserts equality).
export const avatarColors = ['red', 'orange', 'amber', 'emerald', 'teal', 'sky', 'fuchsia'] as const
export type AvatarColor = (typeof avatarColors)[number]

// Avatars render as the colored icon on a white circle with a colored border,
// so each slug maps to accent classes (border + text), not a fill.
// fuchsia is the brand magenta so the migration default reads as Econumo.
export const avatarColorAccents: Record<AvatarColor, string> = {
  red: 'border-red-500 text-red-500',
  orange: 'border-orange-500 text-orange-500',
  amber: 'border-amber-500 text-amber-500',
  emerald: 'border-emerald-500 text-emerald-500',
  teal: 'border-teal-500 text-teal-500',
  sky: 'border-sky-500 text-sky-500',
  fuchsia: 'border-econumo-magenta text-econumo-magenta',
}

// Solid fills for the picker's color swatches (the avatar itself uses accents).
export const avatarColorSwatches: Record<AvatarColor, string> = {
  red: 'bg-red-500',
  orange: 'bg-orange-500',
  amber: 'bg-amber-500',
  emerald: 'bg-emerald-500',
  teal: 'bg-teal-500',
  sky: 'bg-sky-500',
  fuchsia: 'bg-econumo-magenta',
}

// The avatar picker's icon choices — exactly one IconPicker page (9 cols × 4
// rows). Every name must exist in availableIcons, and the backend's
// RandomAvatarIcons must be a subset (both asserted by the sync test).
export const avatarIcons = [
  'face', 'account_circle', 'pets', 'stars', 'celebration', 'lightbulb',
  'extension', 'support', 'savings', 'wallet', 'home', 'alarm',
  'fingerprint', 'shopping_basket', 'credit_card', 'store',
  'favorite', 'smart_toy', 'sports_esports', 'music_note', 'movie', 'cake',
  'school', 'science', 'construction', 'fitness_center', 'spa', 'hiking',
  'sailing', 'motorcycle', 'directions_bike', 'flight', 'castle', 'park',
  'local_florist', 'emoji_events',
]

export const defaultAvatar = 'face:fuchsia'

export function joinAvatar(icon: string, color: string): string {
  return `${icon}:${color}`
}

const isAvatarColor = (v: string): v is AvatarColor => (avatarColors as readonly string[]).includes(v)

export function splitAvatar(avatar: string): { icon: string; color: AvatarColor } {
  const at = avatar.lastIndexOf(':')
  const icon = at > 0 ? avatar.slice(0, at) : avatar
  const color = at > 0 ? avatar.slice(at + 1) : ''
  return { icon, color: isAvatarColor(color) ? color : 'fuchsia' }
}
