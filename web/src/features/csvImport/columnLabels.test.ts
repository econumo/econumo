import { describe, expect, it } from 'vitest'
import { profileColumns } from './columns'
import { displaySamples } from './columnLabels'
import { parseCsvText } from './parse'

const first = (csv: string) => profileColumns(parseCsvText(csv))[0]

describe('displaySamples', () => {
  it('shows a ";" list apart, hashtags joined by commas too', () => {
    expect(displaySamples(first('l\n"Mexico2025;#oregon2410"\n'), 'en')).toEqual(['Mexico2025 · #oregon2410'])
    expect(displaySamples(first('l\n"#a, #b"\n'), 'en')).toEqual(['#a · #b'])
  })

  it('leaves plain text and ordinary commas as written', () => {
    expect(displaySamples(first('l\n"Coffee, large"\nSalary\n'), 'en')).toEqual(['Coffee, large', 'Salary'])
  })

  it('does not touch what is uploaded', () => {
    const p = first('l\n"Mexico2025;#oregon2410"\n')
    displaySamples(p, 'en')
    expect(p.samples).toEqual(['Mexico2025;#oregon2410'])
  })
})
