import { EntityIcon } from '@/components/EntityIcon'
import { avatarColorAccents, splitAvatar } from '@/lib/avatars'
import { cn } from '@/lib/utils'

// One size per current render site: xs=connection preview (20px), sm=share
// dialog + onboarding (32px), md=connection rows + sidebar rail (40px),
// card=sidebar user card (48px), xl=profile page (96px). The xl border widens
// so it doesn't read as a hairline at 96px.
const sizeClasses = {
  xs: 'size-5 rounded-full text-sm border',
  sm: 'size-8 rounded-full text-lg border-2',
  md: 'size-10 rounded-full text-xl border-2',
  card: 'size-12 rounded-xl text-2xl border-2',
  xl: 'size-24 rounded-3xl text-5xl border-4',
} as const

interface UserAvatarProps {
  avatar: string
  size?: keyof typeof sizeClasses
  className?: string
}

// The single avatar render path: the "<icon>:<color>" value as a Material
// glyph in the accent color on a white circle with a matching border.
// Decorative — the adjacent user name carries the accessible label.
export function UserAvatar({ avatar, size = 'md', className }: UserAvatarProps) {
  const { icon, color } = splitAvatar(avatar)
  return (
    <span
      aria-hidden="true"
      data-testid="user-avatar"
      data-avatar={avatar}
      className={cn(
        'flex shrink-0 select-none items-center justify-center bg-white',
        sizeClasses[size],
        avatarColorAccents[color],
        className,
      )}
    >
      <EntityIcon name={icon} />
    </span>
  )
}
