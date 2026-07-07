/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly ECONUMO_VERSION?: string
  readonly WEBSITE_URL?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
