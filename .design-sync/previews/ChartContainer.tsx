import { ChartContainer, ChartLegendContent, ChartTooltipContent } from 'web'
// Tooltip/Legend must come from the SAME recharts copy as BarChart — web's
// ChartTooltip/ChartLegend re-export the library bundle's recharts copy, which
// this BarChart (bundled from node_modules) does not recognize as children.
import { Bar, BarChart, CartesianGrid, Legend, Tooltip, XAxis } from 'recharts'

const monthlySpending = [
  { month: 'Jan', spending: 1845 },
  { month: 'Feb', spending: 1620 },
  { month: 'Mar', spending: 2110 },
  { month: 'Apr', spending: 1490 },
  { month: 'May', spending: 1975 },
  { month: 'Jun', spending: 1730 },
]

const spendingConfig = {
  spending: { label: 'Spending (USD)', color: 'var(--chart-1)' },
}

export const MonthlySpendingBar = () => (
  <ChartContainer config={spendingConfig} style={{ width: 420, height: 236 }}>
    <BarChart accessibilityLayer width={420} height={236} data={monthlySpending}>
      <CartesianGrid vertical={false} />
      <XAxis dataKey="month" tickLine={false} tickMargin={10} axisLine={false} />
      <Tooltip content={<ChartTooltipContent />} />
      <Bar
        dataKey="spending"
        fill="var(--color-spending)"
        radius={4}
        isAnimationActive={false}
      />
    </BarChart>
  </ChartContainer>
)

const cashflow = [
  { month: 'Jan', income: 4200, expenses: 1845 },
  { month: 'Feb', income: 4200, expenses: 1620 },
  { month: 'Mar', income: 4650, expenses: 2110 },
  { month: 'Apr', income: 4200, expenses: 1490 },
  { month: 'May', income: 4200, expenses: 1975 },
  { month: 'Jun', income: 4380, expenses: 1730 },
]

const cashflowConfig = {
  income: { label: 'Income', color: 'var(--chart-2)' },
  expenses: { label: 'Expenses', color: 'var(--chart-5)' },
}

export const IncomeVsExpensesBar = () => (
  <ChartContainer config={cashflowConfig} style={{ width: 420, height: 260 }}>
    <BarChart accessibilityLayer width={420} height={260} data={cashflow}>
      <CartesianGrid vertical={false} />
      <XAxis dataKey="month" tickLine={false} tickMargin={10} axisLine={false} />
      <Tooltip content={<ChartTooltipContent indicator="dashed" />} />
      <Legend content={<ChartLegendContent />} />
      <Bar
        dataKey="income"
        fill="var(--color-income)"
        radius={4}
        isAnimationActive={false}
      />
      <Bar
        dataKey="expenses"
        fill="var(--color-expenses)"
        radius={4}
        isAnimationActive={false}
      />
    </BarChart>
  </ChartContainer>
)
