import type { BudgetDto } from '@/api/dto/budget'

// Test helper: the API layer passes amount strings through verbatim now, so this
// is just a deep clone of the wire fixture typed as a BudgetDto.
export function coerceBudgetFixture(wire: unknown): BudgetDto {
  return JSON.parse(JSON.stringify(wire)) as BudgetDto
}
