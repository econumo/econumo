export function parseBalance(raw: string): string | null {
  const s = raw.replace(/[\s ]/g, '')
  if (s === '') return '0'
  const m = /^(-?)(\d*)(?:[.,](\d+))?$/.exec(s)
  if (!m || (m[2] === '' && m[3] === undefined)) return null
  return `${m[1]}${m[2] || '0'}${m[3] !== undefined ? `.${m[3]}` : ''}`
}
