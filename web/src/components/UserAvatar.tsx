import { EntityIcon } from '@/components/EntityIcon'
import { avatarColorClasses, splitAvatar } from '@/lib/avatars'
import { cn } from '@/lib/utils'

// One size per current render site: xs=connection preview (20px), sm=share
// dialog + onboarding (32px), md=connection rows + sidebar rail (40px),
// card=sidebar user card (48px), xl=profile page (96px).
const sizeClasses = {
  xs: 'size-5 rounded-full text-sm',
  sm: 'size-8 rounded-full text-lg',
  md: 'size-10 rounded-full text-xl',
  card: 'size-12 rounded-xl text-2xl',
  xl: 'size-24 rounded-3xl text-5xl',
} as const

interface UserAvatarProps {
  avatar: string
  size?: keyof typeof sizeClasses
  className?: string
}

// The single avatar render path: the "<icon>:<color>" value as a Material
// glyph on a colored square. Decorative — the adjacent user name carries the
// accessible label.
export function UserAvatar({ avatar, size = 'md', className }: UserAvatarProps) {
  const { icon, color } = splitAvatar(avatar)
  return (
    <span
      aria-hidden="true"
      data-testid="user-avatar"
      data-avatar={avatar}
      className={cn(
        'flex shrink-0 select-none items-center justify-center text-white',
        sizeClasses[size],
        avatarColorClasses[color],
        className,
      )}
    >
      <EntityIcon name={icon} />
    </span>
  )
}
