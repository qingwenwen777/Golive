import axios from 'axios';

export const CSRF_HEADER = 'X-CSRF-Token';

let csrfToken: string | null = null;
let csrfPromise: Promise<string> | null = null;

export function clearCsrfToken() {
  csrfToken = null;
  csrfPromise = null;
}

export function needsCsrfToken(method: string | undefined): boolean {
  const normalized = (method || 'get').toLowerCase();
  return normalized !== 'get' && normalized !== 'head' && normalized !== 'options';
}

export async function getCsrfToken(force = false): Promise<string> {
  if (csrfToken && !force) return csrfToken;
  if (csrfPromise && !force) return csrfPromise;

  csrfPromise = axios
    .get<{ token: string }>(`${import.meta.env.VITE_API_BASE}/csrf-token`, {
      timeout: 10_000,
      withCredentials: true,
    })
    .then(({ data }) => {
      csrfToken = data.token;
      return data.token;
    })
    .finally(() => {
      csrfPromise = null;
    });

  return csrfPromise;
}
