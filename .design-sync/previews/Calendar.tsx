import { Calendar } from 'web'

// Fixed dates keep the capture deterministic (the app composes Calendar with
// weekStartsOn={1} inside the transaction-date popover).
const selectedDay = new Date(2026, 6, 9)

export const SingleDate = () => (
  <Calendar mode="single" weekStartsOn={1} selected={selectedDay} defaultMonth={selectedDay} className="rounded-lg border" />
)

export const DateRange = () => (
  <Calendar
    mode="range"
    weekStartsOn={1}
    selected={{ from: new Date(2026, 6, 6), to: new Date(2026, 6, 12) }}
    defaultMonth={selectedDay}
    className="rounded-lg border"
  />
)

export const DropdownCaption = () => (
  <Calendar
    mode="single"
    captionLayout="dropdown"
    weekStartsOn={1}
    selected={selectedDay}
    defaultMonth={selectedDay}
    startMonth={new Date(2020, 0)}
    endMonth={new Date(2030, 11)}
    className="rounded-lg border"
  />
)

export const DisabledDays = () => (
  <Calendar
    mode="single"
    weekStartsOn={1}
    selected={selectedDay}
    defaultMonth={selectedDay}
    disabled={{ after: new Date(2026, 6, 15) }}
    className="rounded-lg border"
  />
)
