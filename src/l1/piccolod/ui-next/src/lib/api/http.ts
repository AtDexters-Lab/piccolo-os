import { API_BASE_URL } from '$lib/config';

export type ApiError = {
  message: string;
  code?: number;
};

interface RequestOptions extends RequestInit {
  json?: unknown;
}

export async function http<T = unknown>(path: string, options: RequestOptions = {}): Promise<T> {
  const { json, headers, ...rest } = options;
  const body = json !== undefined ? JSON.stringify(json) : options.body;
  const computedHeaders: Record<string, string> = {
    ...(headers as Record<string, string> | undefined)
  };
  if (json !== undefined) {
    computedHeaders['Content-Type'] = 'application/json';
  }

  const response = await fetch(`${API_BASE_URL}${path}`, {
    method: options.method ?? 'GET',
    credentials: 'include',
    headers: computedHeaders,
    body,
    ...rest
  });

  if (!response.ok) {
    let message = response.statusText || 'Request failed';
    try {
      const data = await response.json();
      if (typeof data?.message === 'string') message = data.message;
    } catch {
      // ignore
    }
    const error: ApiError = { message, code: response.status };
    throw error;
  }

  if (response.status === 204) return undefined as T;
  const text = await response.text();
  return (text ? (JSON.parse(text) as T) : (undefined as T));
}
