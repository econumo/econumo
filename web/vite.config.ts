/// <reference types="vitest/config" />
import { defineConfig, type Plugin } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import crypto from 'node:crypto'
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
    const env = fs.readFileSync(path.resolve(import.meta.dirname, '../.env'), 'utf8')
    const match = /^\s*PORT\s*=\s*"?(\d+)"?\s*$/m.exec(env)
    if (match) {
      return match[1]
    }
  } catch {
    // no .env — fall through to the default
  }
  return '8181'
}


// Shell files the service worker precaches so the installed app starts
// offline: the bundle, the runtime config, and what index.html and the
// manifest reference. Skipped: legacy .woff (every browser that runs a
// service worker takes .woff2), the unused icon sizes, analytics scripts and
// the Shortcuts download.
const PRECACHE = /^(index\.html|econumo-config\.js|manifest\.json|favicon\.ico|img\/logo\.png|fonts\/.+\.woff2|icons\/(apple-touch-icon\.png|favicon-\d+x\d+\.png|android-chrome-\d+x\d+-precomposed\.png|maskable-\d+x\d+\.png)|assets\/.+\.(js|css|woff2|svg|png))$/

function listFiles(dir: string, base = ''): string[] {
  return fs.readdirSync(path.join(dir, base), { withFileTypes: true }).flatMap((entry) => {
    const rel = base ? `${base}/${entry.name}` : entry.name
    return entry.isDirectory() ? listFiles(dir, rel) : [rel]
  })
}

// Writes dist/sw.js from sw/sw.js once the bundle is on disk. The version is
// a digest of the precached files, so a deploy that changes any of them
// installs a new worker, and one that changes none keeps the cache.
function serviceWorker(): Plugin {
  let outDir = ''
  return {
    name: 'econumo-service-worker',
    apply: 'build',
    configResolved(config) {
      outDir = path.resolve(config.root, config.build.outDir)
    },
    closeBundle() {
      const files = listFiles(outDir).filter((f) => PRECACHE.test(f)).sort()
      const hash = crypto.createHash('sha256')
      for (const f of files) {
        hash.update(f).update(fs.readFileSync(path.join(outDir, f)))
      }
      const header = `self.__ECONUMO_SW__ = ${JSON.stringify({ version: hash.digest('hex').slice(0, 16), files: files.map((f) => (f === 'index.html' ? '/' : `/${f}`)) })};\n`
      const body = fs.readFileSync(path.resolve(import.meta.dirname, 'sw/sw.js'), 'utf8')
      fs.writeFileSync(path.join(outDir, 'sw.js'), header + body)
    },
  }
}

export default defineConfig({
  plugins: [react(), tailwindcss(), serviceWorker()],
  resolve: {
    alias: { '@': path.resolve(import.meta.dirname, 'src') },
  },
  envPrefix: ['VITE_', 'ECONUMO_', 'WEBSITE_'],
  server: {
    port: 9000,
    fs: { allow: ['..'] },
    proxy: {
      '/api': `http://localhost:${backendPort()}`,
      // Server-owned SPA config (BILLING_URL, ALLOW_REGISTRATION, ...) is merged
      // into this file by the Go server; the public/ copy has only dist fallbacks.
      '/econumo-config.js': `http://localhost:${backendPort()}`,
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
