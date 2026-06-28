/* eslint-disable */

declare namespace NodeJS {
  interface ProcessEnv {
    readonly NODE_ENV: string;
    readonly VUE_ROUTER_MODE: 'hash' | 'history' | 'abstract' | undefined;
    readonly VUE_ROUTER_BASE: string | undefined;
    readonly WEBSITE_URL: string;
  }
}
