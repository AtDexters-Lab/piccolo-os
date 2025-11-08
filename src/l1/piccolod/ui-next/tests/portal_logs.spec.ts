import { expect, test } from '@playwright/test';

const portalPath = process.env.PICCOLO_PORTAL_PATH ?? '/';

/**
 * Opens the Piccolo portal, captures browser console + network logs,
 * and exposes them as Playwright attachments. Use PICCOLO_BASE_URL
 * to point at a different host and PICCOLO_PORTAL_PATH for subpaths.
 */
test('portal renders and logs client activity', async ({ page }, testInfo) => {
  const consoleLogs: Array<{
    type: string;
    text: string;
    location: { url?: string; lineNumber?: number; columnNumber?: number };
  }> = [];
  const networkLogs: Array<{
    url: string;
    method: string;
    status?: number;
    statusText?: string;
    failure?: string;
  }> = [];

  page.on('console', (msg) => {
    consoleLogs.push({
      type: msg.type(),
      text: msg.text(),
      location: msg.location(),
    });
  });

  page.on('requestfinished', async (request) => {
    const response = await request.response();
    networkLogs.push({
      url: request.url(),
      method: request.method(),
      status: response?.status(),
      statusText: response?.statusText(),
    });
  });

  page.on('requestfailed', (request) => {
    networkLogs.push({
      url: request.url(),
      method: request.method(),
      failure: request.failure()?.errorText,
    });
  });

  await page.goto(portalPath, { waitUntil: 'domcontentloaded' });
  await page.waitForLoadState('networkidle');

  // Basic smoke assertion: body exists and has some rendered content.
  await expect(page.locator('body')).toBeVisible();

  await testInfo.attach('console-log.json', {
    body: JSON.stringify(consoleLogs, null, 2),
    contentType: 'application/json',
  });

  await testInfo.attach('network-log.json', {
    body: JSON.stringify(networkLogs, null, 2),
    contentType: 'application/json',
  });
});
