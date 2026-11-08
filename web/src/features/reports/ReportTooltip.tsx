import type { Id } from '@/api/types'

export interface TooltipEntry {
  dataKey?: string | number
  value?: number | string | null
  payload?: Record<string, unknown>
}

export function TooltipBox({ active, payload, series, money, extra }: {
  active?: boolean
  payload?: readonly TooltipEntry[]
  series: { id: Id; name: string; color: string }[]
  money: (a: string) => string
  extra?: (data: Record<string, unknown>) => { label: string; value: string }[]
}) {
  if (!active || !payload?.length) return null
  const data = payload[0].payload ?? {}
  const shown = series.filter((s) => data[s.id] !== null && data[s.id] !== undefined)
  return (
    <div className="grid min-w-36 gap-1 rounded-lg border bg-background px-2.5 py-1.5 text-xs shadow-xl">
      <span>{String(data.title ?? data.label ?? '')}</span>
      {shown.map((s) => (
        <span key={s.id} className="flex items-center gap-2">
          <span className="size-2.5 shrink-0 rounded-sm" style={{ background: s.color }} />
          <span className="flex-1 text-muted-foreground">{s.name}</span>
          <span className="tabular-nums">{money(String(data[s.id]))}</span>
        </span>
      ))}
      {(extra?.(data) ?? []).map((e) => (
        <span key={e.label} className="flex items-center gap-2">
          <span className="flex-1 text-muted-foreground">{e.label}</span>
          <span className="tabular-nums">{e.value}</span>
        </span>
      ))}
    </div>
  )
}
