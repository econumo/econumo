import { Marker, MarkerContent, MarkerIcon } from 'web'
import { CalendarDays, Tags } from 'lucide-react'

const Dot = ({ color }: { color: string }) => (
  <span
    className="block size-3 rounded-full"
    style={{ backgroundColor: color }}
  />
)

export const CategoryDot = () => (
  <div className="w-72">
    <Marker>
      <MarkerIcon>
        <Dot color="#4CAF50" />
      </MarkerIcon>
      <MarkerContent>Groceries</MarkerContent>
    </Marker>
  </div>
)

export const CategoryLegend = () => (
  <div className="flex w-72 flex-col gap-2">
    <Marker>
      <MarkerIcon>
        <Dot color="#4CAF50" />
      </MarkerIcon>
      <MarkerContent>Groceries</MarkerContent>
      <span className="ml-auto text-sm text-expense">−$385.20</span>
    </Marker>
    <Marker>
      <MarkerIcon>
        <Dot color="#FF9800" />
      </MarkerIcon>
      <MarkerContent>Restaurants</MarkerContent>
      <span className="ml-auto text-sm text-expense">−$142.75</span>
    </Marker>
    <Marker>
      <MarkerIcon>
        <Dot color="#2196F3" />
      </MarkerIcon>
      <MarkerContent>Transport</MarkerContent>
      <span className="ml-auto text-sm text-expense">−$42.50</span>
    </Marker>
    <Marker>
      <MarkerIcon>
        <Dot color="#BD51CF" />
      </MarkerIcon>
      <MarkerContent>Salary</MarkerContent>
      <span className="ml-auto text-sm text-income">+$4,200.00</span>
    </Marker>
  </div>
)

export const SeparatorVariant = () => (
  <div className="w-72">
    <Marker variant="separator">
      <MarkerIcon>
        <CalendarDays />
      </MarkerIcon>
      <MarkerContent>June 2026</MarkerContent>
    </Marker>
  </div>
)

export const BorderVariant = () => (
  <div className="w-72">
    <Marker variant="border">
      <MarkerIcon>
        <Tags />
      </MarkerIcon>
      <MarkerContent>Expense categories</MarkerContent>
    </Marker>
  </div>
)
