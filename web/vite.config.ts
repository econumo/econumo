/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import fs from 'node:fs'
import path from 'node:path'

// The dev proxy must target the Go backend, whose listen port comes from the
// repo-root .env (PORT=...). Shell env wins, matching the backend's own
// precedence; fallback 8181.
function backendPort(): string {
  if (process.env.PORT) {
    return process.env.PORT
  }
  try {
    const env = fs.readFileSync(path.resolve(__dirname, '../.env'), 'utf8')
    const match = /^\s*PORT\s*=\s*"?(\d+)"?\s*$/m.exec(env)
    if (match) {
      return match[1]
    }
  } catch {
    // no .env — fall through to the default
  }
  return '8181'
}

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: { '@': path.resolve(__dirname, 'src') },
  },
  envPrefix: ['VITE_', 'ECONUMO_', 'WEBSITE_'],
  server: {
    port: 9000,
    fs: { allow: ['..'] },
    proxy: {
      '/api': `http://localhost:${backendPort()}`,
    },
  },
  test: {
    environment: 'jsdom',
    environmentOptions: {
      jsdom: { url: 'http://localhost:9000/' },
    },
    globals: true,
    setupFiles: './src/test/setup.ts',
  },
})
