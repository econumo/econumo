import { useTranslation } from 'react-i18next'
import { Sankey } from 'recharts'
import type { LinkProps, NodeProps } from 'recharts/types/chart/Sankey'
import type { FlowReportDto } from '@/api/dto/report'
import { ChartContainer } from '@/components/ui/chart'
import { CHART_OTHER } from '@/components/ui/chartPalette'
import {
  FLOW_BALANCE, FLOW_HUB, FLOW_OTHER_EXPENSE, FLOW_OTHER_INCOME, FLOW_SAVED, FLOW_STAYED, FLOW_TO_SAVINGS, flowGraph,
} from './reportMath'
import type { FlowNode } from './reportMath'
import { seriesColors } from './reportColors'
import { rowDisplayName } from './reportRows'

const INCOME = 'var(--income)'
// outside the categorical palette, so no expense row can share them
const SAVED = 'oklch(0.68 0.15 237)'

interface DrawnNode {
  name: string
  amount: string
  color: string
}

export function SankeyFlow({ report, budget, money }: {
  report: FlowReportDto
  budget: boolean
  money: (amount: string) => string
}) {
  const { t } = useTranslation()
  const graph = flowGraph(report, budget)
  const expenseColors = seriesColors(graph.nodes.filter((n) => n.kind === 'expense' && n.id !== FLOW_OTHER_EXPENSE).map((n) => n.id))
  const special: Record<string, { name: string; color: string }> = {
    [FLOW_HUB]: { name: t('reports.cashflow.income'), color: INCOME },
    [FLOW_BALANCE]: { name: t('reports.cashflow.from_balance'), color: CHART_OTHER },
    [FLOW_SAVED]: { name: t('reports.cashflow.saved'), color: SAVED },
    [FLOW_TO_SAVINGS]: { name: t('reports.cashflow.to_savings'), color: SAVED },
    [FLOW_STAYED]: { name: t('reports.cashflow.stayed'), color: INCOME },
    [FLOW_OTHER_INCOME]: { name: t('reports.cashflow.other_income'), color: CHART_OTHER },
    [FLOW_OTHER_EXPENSE]: { name: t('reports.spending.other'), color: CHART_OTHER },
  }
  const draw = (n: FlowNode): DrawnNode => {
    const s = special[n.id]
    if (s) return { name: s.name, amount: money(n.value), color: s.color }
    return {
      name: rowDisplayName({ id: n.id, name: n.name, kind: n.rowKind }, t),
      amount: money(n.value),
      color: n.kind === 'income' ? INCOME : expenseColors[n.id],
    }
  }
  const drawn = graph.nodes.map(draw)
  const data = {
    nodes: drawn.map((d) => ({ name: d.name })),
    links: graph.links.map((l) => ({ source: l.source, target: l.target, value: Number(l.value) })),
  }
  const height = Math.max(320, Math.max(graph.nodes.filter((n) => n.kind !== 'income').length, 4) * 58)

  const renderNode = ({ x, y, width, height: h, index, payload }: NodeProps) => {
    const d = drawn[index]
    if (!d) return <g />
    const hub = graph.nodes[index].id === FLOW_HUB
    const left = payload.depth === 0
    const tx = hub ? x + width / 2 : left ? x - 6 : x + width + 6
    const anchor = hub ? 'middle' : left ? 'end' : 'start'
    const ty = hub ? y - 26 : y + h / 2 - 6
    return (
      <g>
        <rect x={x} y={y} width={width} height={Math.max(h, 1)} fill={d.color} rx={2} />
        <text x={tx} y={ty} textAnchor={anchor} dominantBaseline="middle" className="fill-foreground text-xs">
          <tspan x={tx}>{d.name}</tspan>
          <tspan x={tx} dy={14} className="fill-muted-foreground">{d.amount}</tspan>
        </text>
      </g>
    )
  }

  const renderLink = (props: LinkProps) => {
    const { sourceX, targetX, sourceY, targetY, sourceControlX, targetControlX, linkWidth, index } = props
    const link = graph.links[index]
    // a link takes the colour of the node it explains: the income row on the
    // way in, the expense row or savings on the way out
    const color = link ? drawn[graph.nodes[link.source].id === FLOW_HUB ? link.target : link.source]?.color : CHART_OTHER
    return (
      <path
        d={`M${sourceX},${sourceY}C${sourceControlX},${sourceY} ${targetControlX},${targetY} ${targetX},${targetY}`}
        fill="none"
        stroke={color}
        strokeWidth={Math.max(linkWidth, 1)}
        strokeOpacity={0.3}
      />
    )
  }

  if (graph.nodes.length === 0) return null
  return (
    <figure aria-label={t('reports.cashflow.flow_chart')} className="flex flex-col gap-2">
      <ChartContainer config={{}} className="aspect-auto w-full" style={{ height }}>
        <Sankey
          data={data}
          node={renderNode}
          link={renderLink}
          nodeWidth={10}
          nodePadding={36}
          margin={{ top: 40, right: 190, bottom: 8, left: 150 }}
          sort={false}
          iterations={0}
        />
      </ChartContainer>
    </figure>
  )
}
