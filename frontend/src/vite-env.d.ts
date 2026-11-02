/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** Base URL for the backend API; empty means same-origin. */
  VITE_API_BASE_URL?: string
  /** Dev-server proxy target for /api requests. */
  VITE_API_PROXY_TARGET?: string
}
