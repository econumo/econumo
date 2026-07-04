import { Outlet } from 'react-router'

export function ApplicationLayout() {
  return (
    <div className="flex min-h-svh">
      <main className="flex-1">
        <Outlet />
      </main>
    </div>
  )
}
