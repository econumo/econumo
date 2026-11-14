import type { DebtRateChangeDto } from '@/api/dto/debt'

/** the latest rate change dated on or before today, oldest-first input */
export function rateChangeInForce(rateChanges: DebtRateChangeDto[], today: string): DebtRateChangeDto | undefined {
  return rateChanges.filter((r) => r.date <= today).at(-1)
}

/** the regular payment in force today: the latest change's, else the contract's */
export function paymentInForce(rateChanges: DebtRateChangeDto[], today: string, contractPayment: string): string {
  return rateChangeInForce(rateChanges, today)?.payment ?? contractPayment
}
