export type ErrorResponse = { code?: number; message: string; next_step?: string };

const base = (__DEMO__ ? '/api/v1/demo' : '/api/v1');

let csrfToken: string | null = null;

export async function ensureCsrf(): Promise<string> {
  if (csrfToken) return csrfToken;
  try {
    const res = await fetch(`${base.replace(/\/demo$/, '')}/auth/csrf`);
    if (res.ok) {
      const json = await res.json();
      csrfToken = json.token || null;
    }
  } catch {}
  return csrfToken || '';
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const url = `${base}${path}`;
  const headers = new Headers(init.headers);
  if (init.method && init.method !== 'GET' && init.method !== 'HEAD') {
    const token = await ensureCsrf();
    if (token) headers.set('X-CSRF-Token', token);
    if (!headers.has('Content-Type')) headers.set('Content-Type', 'application/json');
  }
  const resp = await fetch(url, { credentials: 'same-origin', ...init, headers });
  const text = await resp.text();
  const json = text ? JSON.parse(text) : {};

  if (!resp.ok || json?.error) {
    const err: ErrorResponse = json?.error ? { code: json.error.code, message: json.error.message } : { message: resp.statusText, code: resp.status };
    throw err;
  }
  return json as T;
}

export const demo = __DEMO__;

