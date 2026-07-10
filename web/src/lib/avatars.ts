// The avatar value format is "<icon>:<color>" — a Material ligature name plus
// a color slug. The color list mirrors the backend allowlist in
// internal/user/avatar.go (order is contract; a sync test asserts equality).
export const avatarColors = [
  'red', 'orange', 'amber', 'yellow', 'lime', 'green', 'emerald', 'teal',
  'cyan', 'sky', 'blue', 'indigo', 'violet', 'purple', 'fuchsia', 'pink',
] as const
export type AvatarColor = (typeof avatarColors)[number]

// fuchsia is the brand magenta so the migration default reads as Econumo.
export const avatarColorClasses: Record<AvatarColor, string> = {
  red: 'bg-red-500',
  orange: 'bg-orange-500',
  amber: 'bg-amber-500',
  yellow: 'bg-yellow-500',
  lime: 'bg-lime-500',
  green: 'bg-green-500',
  emerald: 'bg-emerald-500',
  teal: 'bg-teal-500',
  cyan: 'bg-cyan-500',
  sky: 'bg-sky-500',
  blue: 'bg-blue-500',
  indigo: 'bg-indigo-500',
  violet: 'bg-violet-500',
  purple: 'bg-purple-500',
  fuchsia: 'bg-econumo-magenta',
  pink: 'bg-pink-500',
}

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
