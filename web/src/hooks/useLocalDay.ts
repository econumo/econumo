import { useEffect, useState } from 'react'

function startOfDay(d: Date): Date {
  const start = new Date(d)
  start.setHours(0, 0, 0, 0)
  return start
}

function msToNextMidnight(now: Date): number {
  const next = new Date(now)
  next.setHours(24, 0, 0, 0)
  return next.getTime() - now.getTime()
}

/** The start of the local day, a new value at local midnight, so what depends on "today" is computed again. */
export function useLocalDay(): Date {
  const [day, setDay] = useState(() => startOfDay(new Date()))
  useEffect(() => {
    let timer: ReturnType<typeof setTimeout>
    const arm = () => {
      timer = setTimeout(() => {
        const start = startOfDay(new Date())
        setDay((current) => (current.getTime() === start.getTime() ? current : start))
        // a timer can fire a moment early: the next one covers what is left of the day
        arm()
      }, msToNextMidnight(new Date()))
    }
    arm()
    return () => clearTimeout(timer)
  }, [])
  return day
}
