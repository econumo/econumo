import type { ComponentProps, KeyboardEvent } from 'react'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { evaluateFormula, sanitizeInput, validateFormula } from '@/lib/calculator'

interface CalculatorInputProps extends Omit<ComponentProps<typeof Input>, 'value' | 'onChange'> {
  value: string
  onChange: (value: string) => void
}

const KEYPAD: { label: string; op: string }[] = [
  { label: '+', op: '+' },
  { label: '−', op: '-' },
  { label: '×', op: '*' },
  { label: '÷', op: '/' },
  { label: '=', op: '=' },
]

export function CalculatorInput({ value, onChange, ...inputProps }: CalculatorInputProps) {
  const handleChange = (raw: string) => {
    if (raw.endsWith('=')) {
      const sanitized = sanitizeInput(raw.slice(0, -1))
      if (validateFormula(sanitized)) {
        onChange(evaluateFormula(sanitized + '='))
        return
      }
    }
    onChange(raw)
  }

  const handleKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key !== 'Enter') {
      return
    }
    const sanitized = sanitizeInput(value)
    if (/[+\-*/]/.test(sanitized) && validateFormula(sanitized)) {
      // a pending formula: evaluate instead of submitting the form
      e.preventDefault()
      onChange(evaluateFormula(sanitized + '='))
    }
  }

  const pressKey = (op: string) => {
    if (op === '=') {
      const sanitized = sanitizeInput(value)
      if (validateFormula(sanitized)) {
        onChange(evaluateFormula(sanitized + '='))
      }
      return
    }
    onChange(sanitizeInput(value + op))
  }

  return (
    <div className="flex flex-col gap-1">
      <Input
        inputMode="decimal"
        pattern="[0-9+\-\*\.=,]*"
        autoComplete="off"
        {...inputProps}
        value={value}
        onChange={(e) => handleChange(e.target.value)}
        onKeyDown={handleKeyDown}
      />
      {/* always mounted: showing it on focus shifted the layout mid-tap, so the
          first tap on any control below landed on moved ground and was lost */}
      <div data-calculator-keypad className="flex gap-1">
        {KEYPAD.map((key) => (
          <Button
            key={key.op}
            type="button"
            variant="secondary"
            size="sm"
            // pointer/tap targets only: keyboard users type the operator directly,
            // and Tab must reach the next form field, not walk the keypad
            tabIndex={-1}
            className="flex-1 h-10"
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => pressKey(key.op)}
          >
            {key.label}
          </Button>
        ))}
      </div>
    </div>
  )
}
