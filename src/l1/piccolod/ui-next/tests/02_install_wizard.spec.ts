import { expect, test, type Page } from '@playwright/test';

const installPath = process.env.PICCOLO_INSTALL_PATH ?? '/install';
const baseURL = (process.env.PICCOLO_BASE_URL ?? 'http://piccolo.local').replace(/\/$/, '');
const apiBase = `${baseURL}/api/v1`;
const testPassword = process.env.PICCOLO_E2E_PASSWORD ?? 'Supersafe123!';
let csrfToken: string | null = null;

async function ensureCsrf(page: Page): Promise<string | null> {
  if (csrfToken) return csrfToken;
  const response = await page.request.fetch(`${apiBase}/auth/csrf`, { method: 'GET', failOnStatusCode: false });
  if (!response.ok()) {
    return null;
  }
  const data = await response.json();
  const token = typeof data?.token === 'string' ? data.token : null;
  if (token) {
    csrfToken = token;
  }
  return token;
}

type RequestOptions = {
  method?: 'GET' | 'POST';
  data?: Record<string, unknown>;
  allowedStatuses?: number[];
  useCsrf?: boolean;
};

async function apiRequest(
  page: Page,
  path: string,
  { method = 'GET', data, allowedStatuses = [], useCsrf = true }: RequestOptions = {}
) {
  const headers: Record<string, string> = {};
  if (method !== 'GET') {
    if (useCsrf) {
      const token = await ensureCsrf(page);
      if (token) {
        headers['X-CSRF-Token'] = token;
      }
    }
    headers['Content-Type'] = 'application/json';
  }
  const response = await page.request.fetch(`${apiBase}${path}`, {
    method,
    data,
    headers,
    failOnStatusCode: false
  });
  if (!response.ok() && !allowedStatuses.includes(response.status())) {
    throw new Error(`API ${method} ${path} failed with ${response.status()}: ${response.statusText()}`);
  }
  const text = await response.text();
  return { status: response.status(), data: text ? JSON.parse(text) : undefined };
}

async function ensureSetupCompleted(page: Page) {
  const { data: crypto } = await apiRequest(page, '/crypto/status');
  const initialized = Boolean(crypto?.initialized);
  const locked = Boolean(crypto?.locked);

  if (!initialized) {
    const setupResponse = await apiRequest(page, '/crypto/setup', {
      method: 'POST',
      data: { password: testPassword },
      allowedStatuses: [400],
      useCsrf: false
    });
    if (setupResponse.status === 400) {
      const statusCheck = await apiRequest(page, '/crypto/status');
      if (!statusCheck.data?.initialized) {
        throw new Error('Crypto setup failed to initialize the device');
      }
    }
    await apiRequest(page, '/crypto/unlock', { method: 'POST', data: { password: testPassword }, useCsrf: false });
  } else if (locked) {
    await apiRequest(page, '/crypto/unlock', { method: 'POST', data: { password: testPassword }, useCsrf: false });
  }

  let authReady = false;
  for (let attempt = 0; attempt < 2 && !authReady; attempt++) {
    const authResult = await apiRequest(page, '/auth/initialized', { allowedStatuses: [423] });
    if (authResult.status === 423) {
      await apiRequest(page, '/crypto/unlock', { method: 'POST', data: { password: testPassword }, useCsrf: false });
      continue;
    }
    if (!authResult.data?.initialized) {
      await apiRequest(page, '/auth/setup', { method: 'POST', data: { password: testPassword } });
    }
    authReady = true;
  }
}

async function loginViaUi(page: Page, redirectPath: string) {
  const redirectQuery = encodeURIComponent(redirectPath);
  await page.goto(`/login?redirect=${redirectQuery}`, { waitUntil: 'domcontentloaded' });
  await page.getByLabel(/Admin password/i).fill(testPassword);
  await page.getByRole('button', { name: /(Continue|Sign in)/i }).click();
  await page.waitForURL((url) => url.pathname === redirectPath);
}

test('install wizard scaffold renders and advances to disk step', async ({ page }) => {
  await ensureSetupCompleted(page);
  await loginViaUi(page, installPath);

  await expect(page.getByTestId('install-wizard')).toBeVisible();
  await expect(page.getByRole('heading', { name: /New disk install wizard/i })).toBeVisible();

  await page.getByRole('button', { name: /Begin install/i }).click();

  await expect(page.getByRole('heading', { name: /Choose the installation target/i })).toBeVisible();
});
