import { UserCard } from 'web'

// Offline-safe avatar: SVG data URI ending in '#' so the component's
// appended `?s=100` lands in the URL fragment and is ignored.
const makeAvatar = (initials: string, bg: string) =>
  `data:image/svg+xml,${encodeURIComponent(
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 200"><rect width="200" height="200" fill="${bg}"/><text x="100" y="112" font-family="Roboto, Arial, sans-serif" font-size="76" font-weight="500" fill="#ffffff" text-anchor="middle">${initials}</text></svg>`,
  )}#`

const anna = {
  name: 'Anna Kovaleva',
  email: 'anna.kovaleva@fastmail.com',
  avatar: makeAvatar('AK', '#BD51CF'),
}

const james = {
  name: 'James Middleton-Fairbanks',
  email: 'james.middleton.fairbanks@googlemail.com',
  avatar: makeAvatar('JM', '#2E7D32'),
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

export const TruncatedLongText = () => (
  <div className="w-56 rounded-lg border bg-card p-3">
    <UserCard user={james} />
  </div>
)
