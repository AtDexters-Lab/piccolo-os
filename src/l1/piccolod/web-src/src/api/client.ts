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
    // If sending JSON, default content type; if body is FormData, let the browser set boundary
    const isFormData = typeof FormData !== 'undefined' && (init as any).body instanceof FormData;
    if (!headers.has('Content-Type') && !isFormData) headers.set('Content-Type', 'application/json');
  }
  const resp = await fetch(url, { credentials: 'same-origin', ...init, headers });
  const text = await resp.text();
  let json: any = {};
  try { json = text ? JSON.parse(text) : {}; } catch { json = {}; }

  // Normalize demo errors that return 200 with an error body
  const hasErrorBody = json && (typeof json.error !== 'undefined' || typeof json.message === 'string' && !resp.ok);
  if (!resp.ok || hasErrorBody) {
    let code: number | undefined = resp.status || json.code;
    let message = resp.statusText || '';
    if (typeof json.error === 'string') {
      // Demo fixtures: { error: "Too Many Requests", code: 429, message: "Try again later" }
      message = json.message || json.error || message;
      code = json.code ?? code;
    } else if (json && typeof json.error === 'object') {
      message = json.error?.message || json.message || message;
      code = json.error?.code ?? json.code ?? code;
    } else if (json && typeof json.message === 'string') {
      message = json.message || message;
    }
    const err: ErrorResponse = { code, message: message || 'Request failed' };
    if (code === 401 || code === 403) {
      try { window.dispatchEvent(new Event('piccolo-session-expired')); } catch {}
    }
    throw err;
  }
  return json as T;
}

export const demo = __DEMO__;
