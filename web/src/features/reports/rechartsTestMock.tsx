import { cloneElement, isValidElement } from 'react'
import type { ReactElement, ReactNode } from 'react'

// jsdom lays nothing out, so ResponsiveContainer measures 0x0 and draws no
// chart; tests give the chart a fixed size instead.
export function fixedSizeRecharts<T extends object>(mod: T): T {
  return {
    ...mod,
    ResponsiveContainer: ({ children }: { children: ReactNode }) =>
      isValidElement(children) ? cloneElement(children as ReactElement<{ width: number; height: number }>, { width: 400, height: 240 }) : null,
  }
}
