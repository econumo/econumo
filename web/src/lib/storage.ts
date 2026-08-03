import { mirrorWrite } from './appStorage'

const TOKEN_KEY = 'token'

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY)
}

export function hasToken(): boolean {
  return getToken() !== null
}

export function setToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token)
  mirrorWrite(TOKEN_KEY, token)
}

export function removeToken(): void {
  localStorage.removeItem(TOKEN_KEY)
  mirrorWrite(TOKEN_KEY, null)
}

export function getItem(key: string): unknown {
  const value = localStorage.getItem(key)
  if (value === null) {
    return null
  }
  try {
    return JSON.parse(value)
  } catch {
    return null
  }
}

export function setItem(key: string, value: unknown): void {
  const encoded = JSON.stringify(value)
  localStorage.setItem(key, encoded)
  mirrorWrite(key, encoded)
}

export function removeItem(key: string): void {
  localStorage.removeItem(key)
  mirrorWrite(key, null)
}
