import { UserAvatar } from 'web'

// avatar is "<icon>:<color>": a Material ligature name + one of the 7
// backend-allowlisted color slugs (red orange amber emerald teal sky fuchsia).

export const Sizes = () => (
  <div className="flex items-end gap-3">
    <UserAvatar avatar="owl:teal" size="xs" />
    <UserAvatar avatar="owl:teal" size="sm" />
    <UserAvatar avatar="owl:teal" size="md" />
    <UserAvatar avatar="owl:teal" size="card" />
    <UserAvatar avatar="owl:teal" size="xl" />
  </div>
)

const palette = [
  'face:red',
  'savings:orange',
  'star:amber',
  'emoji_nature:emerald',
  'owl:teal',
  'rocket_launch:sky',
  'diamond:fuchsia',
]

export const Colors = () => (
  <div className="flex items-center gap-2">
    {palette.map((avatar) => (
      <UserAvatar key={avatar} avatar={avatar} size="card" />
    ))}
  </div>
)
