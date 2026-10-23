import { CHART_OTHER, CHART_PALETTE } from '@/components/ui/chartPalette'
import { OTHER_ID, seriesColors } from './reportColors'

it('a row keeps its colour whatever else is on the chart', () => {
  const alone = seriesColors(['cat-food'])['cat-food']
  expect(CHART_PALETTE).toContain(alone)
  expect(seriesColors(['cat-a', 'cat-food', 'cat-b'])['cat-food']).toBe(seriesColors(['cat-food', 'cat-c'])['cat-food'])
})

it('rows on one chart never share a colour', () => {
  const ids = ['a', 'b', 'c', 'd', 'e', 'f', 'g', 'h']
  expect(new Set(Object.values(seriesColors(ids))).size).toBe(8)
})

it('everything else is the neutral grey', () => {
  expect(seriesColors(['a', OTHER_ID])[OTHER_ID]).toBe(CHART_OTHER)
})
