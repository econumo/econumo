import { useState } from 'react'
import type { ComponentProps, KeyboardEvent } from 'react'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { useIsMobile } from '@/hooks/useIsMobile'
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
  const isMobile = useIsMobile()
  const [focused, setFocused] = useState(false)

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
        onFocus={(e) => {
          setFocused(true)
          inputProps.onFocus?.(e)
        }}
        onBlur={(e) => {
          // keep the keypad usable: only hide when focus leaves the widget
          if (!(e.relatedTarget instanceof HTMLElement) || !e.relatedTarget.closest('[data-calculator-keypad]')) {
            setFocused(false)
          }
          inputProps.onBlur?.(e)
        }}
      />
      {isMobile && focused ? (
        <div data-calculator-keypad className="flex gap-1">
          {KEYPAD.map((key) => (
            <Button
              key={key.op}
              type="button"
              variant="secondary"
              size="sm"
              className="flex-1"
              onMouseDown={(e) => e.preventDefault()}
              onClick={() => pressKey(key.op)}
            >
              {key.label}
            </Button>
          ))}
        </div>
      ) : null}
    </div>
  )
}
