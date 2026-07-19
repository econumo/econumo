import { CalculatorInput } from 'web'

export const Amount = () => (
  <div className="w-72">
    <CalculatorInput value="385.20" onChange={() => {}} />
  </div>
)

export const PendingFormula = () => (
  <div className="w-72">
    <CalculatorInput value="1250+340.20" onChange={() => {}} />
  </div>
)

export const Placeholder = () => (
  <div className="w-72">
    <CalculatorInput value="" placeholder="Amount" onChange={() => {}} />
  </div>
)
