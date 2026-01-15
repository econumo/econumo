import { sanitizeInput, validateFormula } from './calculator';

export function isValidHttpUrl(value: string): boolean {
  let url;
  try {
    url = new URL(value);
  } catch (_) {
    return false;
  }

  return url.protocol === 'http:' || url.protocol === 'https:';
}

export function isValidEmail(value: string) {
  return /.+@.+/.test(value);
}

export function isValidNumber(value: string) {
  if (value === '') {
    return true;
  }
  return /^\-?\d+([,\.]{1}\d+)?$/.test(value);
}

export function isValidDecimalNumber(value: string) {
  if (value === '') {
    return true;
  }
  return /^-?\d+([.,]\d{1,8})?$/.test(value);
}

export function isValidName(value: string) {
  return value.length >= 2 && value.length <= 64;
}

export function isValidFolderName(value: string) {
  return value.length >= 2 && value.length <= 64;
}

export function isValidAccountName(value: string) {
  return value.length >= 3 && value.length <= 64;
}

export function isValidCategoryName(value: string) {
  return value.length >= 3 && value.length <= 64;
}

export function isValidTagName(value: string) {
  return value.length >= 3 && value.length <= 64;
}

export function isValidPayeeName(value: string) {
  return value.length >= 3 && value.length <= 64;
}

export function isValidBudgetName(value: string) {
  return value.length >= 3 && value.length <= 64;
}

export function isValidPassword(value: string) {
  return value.length >= 4;
}

export function isValidBudgetFolderName(value: string) {
  return value.length >= 3 && value.length <= 64;
}

export function isValidBudgetEnvelopeName(value: string) {
  return value.length >= 3 && value.length <= 64;
}

export function isNotEmpty(value: string) {
  return value !== null && value !== '';
}

export function isValidRecoveryCode(value: string) {
  return value.length === 12;
}

export function isValidFormula(value: string) {
  return validateFormula(sanitizeInput(value));
}

export function hasIncompleteFormula(value: string) {
  return /[+\-*/]$/.test(value);
}
