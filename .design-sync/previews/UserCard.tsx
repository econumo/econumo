import { UserCard } from 'web'

// user.avatar is the app's "<icon>:<color>" value (Material glyph + color
// slug), rendered by the embedded UserAvatar — never an image URL.
const anna = {
  name: 'Anna Kovaleva',
  email: 'anna.kovaleva@fastmail.com',
  avatar: 'owl:teal',
}

const james = {
  name: 'James Middleton-Fairbanks',
  email: 'james.middleton.fairbanks@googlemail.com',
  avatar: 'rocket_launch:sky',
}

export const Sidebar = () => (
  <div className="w-64 rounded-lg border bg-card p-3">
    <UserCard user={anna} />
  </div>
)

export const ProfileWithLogout = () => (
  <div className="w-96 px-1 py-3">
    <UserCard user={anna} size="lg">
      <button type="button" className="self-start text-sm text-econumo-magenta underline">
        Logout
      </button>
    </UserCard>
  </div>
)

export const AvatarOpensPicker = () => (
  <div className="w-96 px-1 py-3">
    <UserCard user={james} size="lg" onAvatarClick={() => {}} avatarLabel="Change avatar" />
  </div>
)

export const TruncatedLongText = () => (
  <div className="w-56 rounded-lg border bg-card p-3">
    <UserCard user={james} />
  </div>
)
