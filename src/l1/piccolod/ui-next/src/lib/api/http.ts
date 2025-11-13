import { API_BASE_URL } from '$lib/config';

export type ApiError = {
  message: string;
  code?: number;
  retryAfter?: number;
  details?: unknown;
};

interface RequestOptions extends RequestInit {
  json?: unknown;
  skipCsrf?: boolean;
}

const NON_MUTATING_METHODS = new Set(['GET', 'HEAD', 'OPTIONS']);
let csrfToken: string | null = null;
let csrfPromise: Promise<string | null> | null = null;

async function fetchCsrfToken(): Promise<string | null> {
  const response = await fetch(`${API_BASE_URL}/auth/csrf`, {
    method: 'GET',
    credentials: 'include'
  });
  if (!response.ok) {
    return null;
  }
  try {
    const data = await response.json();
    return typeof data?.token === 'string' ? data.token : null;
  } catch {
    return null;
  }
}

async function ensureCsrfToken(): Promise<string | null> {
  if (csrfToken) return csrfToken;
  if (!csrfPromise) {
    csrfPromise = fetchCsrfToken()
      .then((token) => {
        csrfToken = token;
        return token;
      })
      .finally(() => {
        csrfPromise = null;
      });
  }
  return csrfPromise;
}

function shouldAttachCsrf(method: string, { skipCsrf }: RequestOptions): boolean {
  if (skipCsrf) return false;
  return !NON_MUTATING_METHODS.has(method.toUpperCase());
}

export async function http<T = unknown>(path: string, options: RequestOptions = {}): Promise<T> {
  const { json, headers, skipCsrf, ...rest } = options;
  const body = json !== undefined ? JSON.stringify(json) : options.body;
  const computedHeaders: Record<string, string> = {
    ...(headers as Record<string, string> | undefined)
  };
  if (json !== undefined) {
    computedHeaders['Content-Type'] = 'application/json';
  }

  const method = (options.method ?? 'GET').toUpperCase();
  if (shouldAttachCsrf(method, { skipCsrf })) {
    const token = await ensureCsrfToken();
    if (token) {
      computedHeaders['X-CSRF-Token'] = token;
    }
  }

  const response = await fetch(`${API_BASE_URL}${path}`, {
    method,
    credentials: 'include',
    headers: computedHeaders,
    body,
    ...rest
  });

  if (response.status === 403) {
    // CSRF tokens can expire; force refresh on next request
    csrfToken = null;
  }

  if (!response.ok) {
    let message = response.statusText || 'Request failed';
    let details: unknown;
    try {
      const data = await response.json();
      if (typeof data?.message === 'string') message = data.message;
      details = data;
    } catch {
      // ignore
    }
    const retryAfterHeader = response.headers.get('Retry-After');
    const retryAfter = retryAfterHeader ? Number(retryAfterHeader) : undefined;
    const error: ApiError = { message, code: response.status, details, retryAfter: Number.isFinite(retryAfter) ? retryAfter : undefined };
    throw error;
  }

  if (response.status === 204) return undefined as T;
  const text = await response.text();
  return text ? (JSON.parse(text) as T) : (undefined as T);
}

export async function primeCsrfToken(): Promise<void> {
  await ensureCsrfToken();
}

export function resetCsrfToken(): void {
  csrfToken = null;
}
