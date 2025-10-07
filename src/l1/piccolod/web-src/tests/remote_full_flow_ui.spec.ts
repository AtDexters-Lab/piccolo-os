import { test, expect } from '@playwright/test';
import { spawn } from 'child_process';
import fs from 'fs';
import path from 'path';
import os from 'os';
import https from 'https';

const ADMIN_PASSWORD = process.env.PICCOLO_E2E_ADMIN_PASSWORD || 'password';
const NEXUS_ENDPOINT = process.env.PICCOLO_E2E_NEXUS_ENDPOINT || 'wss://nxs.abhishekborar.com:8443/connect';
const NEXUS_SECRET = process.env.PICCOLO_E2E_NEXUS_SECRET || '503f05a677ba7bf94d37240bc28833b65528f1e160611556daa24db554f54b44';
const PICCOLO_TLD = process.env.PICCOLO_E2E_TLD || 'abhishekborar.com';
const PORTAL_SUBDOMAIN = process.env.PICCOLO_E2E_PORTAL || 'piccolo';
const REMOTE_HOST = `${PORTAL_SUBDOMAIN}.${PICCOLO_TLD}`;

const PICCOLO_BIN = path.resolve(__dirname, '../../piccolod');

async function waitForHttps(url: string, timeoutMs: number) {
  const deadline = Date.now() + timeoutMs;
  let lastError: any;
  while (Date.now() < deadline) {
    try {
      await new Promise<void>((resolve, reject) => {
        https.get(url, { rejectUnauthorized: false }, (res) => {
          const ok = res.statusCode && res.statusCode < 500;
          res.resume();
          ok ? resolve() : reject(new Error(`status ${res.statusCode}`));
        }).on('error', reject);
      });
      return;
    } catch (err) {
      lastError = err;
      await new Promise((r) => setTimeout(r, 5000));
    }
  }
  throw lastError ?? new Error(`timeout waiting for ${url}`);
}

test.describe('Remote full flow (UI-driven)', () => {
  let stateDir: string;
  let server: ReturnType<typeof spawn> | undefined;

  test.beforeAll(async () => {
    stateDir = fs.mkdtempSync(path.join(os.tmpdir(), 'piccolod-e2e-ui-'));
    const env = { ...process.env, PICCOLO_STATE_DIR: stateDir, PORT: '8080' };
    server = spawn(PICCOLO_BIN, { env, stdio: 'inherit' });

    const deadline = Date.now() + 30_000;
    while (Date.now() < deadline) {
      try {
        const res = await fetch('http://localhost:8080');
        if (res.ok) break;
      } catch (_) {
        await new Promise((r) => setTimeout(r, 500));
      }
    }
  });

  test.afterAll(async () => {
    if (server) server.kill('SIGINT');
    if (stateDir) fs.rmSync(stateDir, { recursive: true, force: true });
  });

  test('creates admin, configures remote, verifies HTTPS', async ({ page }) => {
    await page.goto('http://localhost:8080/#/setup');
    await page.getByLabel('Password').fill(ADMIN_PASSWORD);
    await page.getByLabel('Confirm password').fill(ADMIN_PASSWORD);
    await page.getByRole('button', { name: 'Create Admin' }).click();
    await page.waitForURL('**/#/');

    // Unlock/auth happens automatically after setup; ensure session is live
    await expect(page.locator('h2', { hasText: 'Dashboard' })).toBeVisible();

    await page.goto('http://localhost:8080/#/remote');
    await page.waitForLoadState('networkidle');

    await page.getByLabel('Nexus endpoint').fill(NEXUS_ENDPOINT);
    await page.getByLabel('JWT signing secret').fill(NEXUS_SECRET);
    await page.getByLabel('Piccolo domain (TLD)').fill(PICCOLO_TLD);
    await page.getByLabel('Use a dedicated portal subdomain').check();
    await page.getByPlaceholder('portal').clear();
    await page.getByPlaceholder('portal').fill(PORTAL_SUBDOMAIN);

    await page.getByRole('button', { name: 'Save & run preflight' }).click();

    const deadline = Date.now() + 10 * 60_000;
    let portalIssued = false;
    while (Date.now() < deadline) {
      const statusText = await page.locator('section:has-text("Certificate inventory")').textContent();
      if (statusText && statusText.includes('portal.') && statusText.includes('OK')) {
        portalIssued = true;
        break;
      }
      await page.reload();
      await page.waitForLoadState('networkidle');
    }
    expect(portalIssued).toBeTruthy();

    await waitForHttps(`https://${REMOTE_HOST}/#/login`, 120_000);
  });
});
