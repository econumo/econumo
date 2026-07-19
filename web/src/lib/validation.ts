import { sanitizeInput, validateFormula } from './calculator'

export function isValidHttpUrl(value: string): boolean {
  let url
  try {
    url = new URL(value)
  } catch {
    return false
  }

  return url.protocol === 'http:' || url.protocol === 'https:'
}

export function isValidEmail(value: string): boolean {
  return /.+@.+/.test(value)
}

export function isValidNumber(value: string): boolean {
  if (value === '') {
    return true
  }
  return /^\-?\d+([,\.]{1}\d+)?$/.test(value)
}

export function isValidDecimalNumber(value: string): boolean {
  if (value === '') {
    return true
  }
  return /^-?\d+([.,]\d{1,8})?$/.test(value)
}

export function isValidName(value: string): boolean {
  return value.length >= 2 && value.length <= 64
}

export function isValidFolderName(value: string): boolean {
  return value.length >= 2 && value.length <= 64
}

export function isValidAccountName(value: string): boolean {
  return value.length >= 3 && value.length <= 64
}

export function isValidCategoryName(value: string): boolean {
  return value.length >= 3 && value.length <= 64
}

export function isValidTagName(value: string): boolean {
  return value.length >= 3 && value.length <= 64
}

export function isValidPayeeName(value: string): boolean {
  return value.length >= 3 && value.length <= 64
}

export function isValidBudgetName(value: string): boolean {
  return value.length >= 3 && value.length <= 64
}

export function isValidPassword(value: string): boolean {
  return value.length >= 8 && value.length <= 128
}

export function isValidBudgetFolderName(value: string): boolean {
  return value.length >= 3 && value.length <= 64
}

export function isValidBudgetEnvelopeName(value: string): boolean {
  return value.length >= 3 && value.length <= 64
}

export function isNotEmpty(value: string): boolean {
  return value !== null && value !== ''
}

export function isValidRecoveryCode(value: string): boolean {
  return value.length === 12
}

export function isValidFormula(value: string): boolean {
  if (value.trim() === '') {
    return true
  }
  // sanitizeInput silently strips foreign characters, so "abc" would sanitize
  // to "" and validate as an empty (valid) formula — reject the raw text first
  if (/[^0-9+\-*/=.,\s]/.test(value)) {
    return false
  }
  const sanitized = sanitizeInput(value)
  // non-empty input must still hold a number after sanitizing ("...", "=")
  if (sanitized.replace(/=/g, '') === '') {
    return false
  }
  return validateFormula(sanitized)
}

export function hasIncompleteFormula(value: string): boolean {
  return /[+\-*/]$/.test(value)
}
