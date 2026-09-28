/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_API_BASE: string;
  readonly VITE_WS_BASE: string;
  readonly VITE_FLV_BASE: string;
  readonly VITE_RTMP_BASE: string;
  readonly VITE_GOOGLE_CLIENT_ID: string;
  readonly VITE_TOPUP_CURRENCY?: string;
  readonly VITE_COINS_PER_CURRENCY_UNIT?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
