import { test, expect } from '@playwright/test';
import { fileURLToPath } from 'url';
import path from 'path';
import fs from 'fs';
import os from 'os';
import { login, ADMIN_PASSWORD } from './support/session';

const NEXUS_ENDPOINT = process.env.PICCOLO_E2E_NEXUS_ENDPOINT || 'wss://stub/connect';
const NEXUS_SECRET = process.env.PICCOLO_E2E_NEXUS_SECRET || 'stub-secret';
const PICCOLO_TLD = process.env.PICCOLO_E2E_TLD || 'example.com';
const PORTAL_SUBDOMAIN = process.env.PICCOLO_E2E_PORTAL || 'portal';
const REMOTE_HOST = `${PORTAL_SUBDOMAIN}.${PICCOLO_TLD}`;

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const PICCOLO_BIN = path.resolve(__dirname, '../../piccolod');

test.describe.configure({ timeout: 10 * 60 * 1000 });
test.describe('Remote full flow (UI-driven)', () => {
  let stateDir: string;
  let server: ReturnType<typeof spawn> | undefined;



  test('creates admin, configures remote, verifies HTTPS', async ({ page }) => {
    const initStatus = await page.request.get('/api/v1/auth/initialized').then(r => r.json()).catch(() => ({}));
    if (!initStatus?.initialized) {
      await page.goto('http://localhost:8080/#/setup');
      await page.getByPlaceholder('New password').fill(ADMIN_PASSWORD);
      await page.getByPlaceholder('Confirm password').fill(ADMIN_PASSWORD);
      await page.getByRole('button', { name: 'Create Admin' }).click();
      try {
        await page.waitForURL('**/#/', { timeout: 15000 });
      } catch {
        if (await page.locator('text=Locked').first().isVisible()) {
          await page.goto('http://localhost:8080/#/login');
          await page.getByPlaceholder('admin password').fill(ADMIN_PASSWORD);
          await page.getByRole('button', { name: /unlock/i }).click();
          await page.waitForURL('**/#/');
        } else {
          throw new Error('Create admin did not navigate to dashboard and no lock message found');
        }
      }
    } else {
      await login(page, ADMIN_PASSWORD);
    }

    const csrfToken = await page.request.get('/api/v1/auth/csrf').then(r => r.json()).then(j => j.token as string).catch(() => '');
    if (csrfToken) {
      await page.request.post('/api/v1/remote/disable', { headers: { 'X-CSRF-Token': csrfToken } }).catch(() => {});
    }

    await expect(page.locator('h2', { hasText: 'Dashboard' })).toBeVisible();

    await page.goto('http://localhost:8080/#/remote');
    await page.waitForLoadState('networkidle');

    await page.getByLabel('Nexus endpoint').fill(NEXUS_ENDPOINT);
    await page.getByLabel('JWT signing secret').fill(NEXUS_SECRET);
    await page.getByLabel('Piccolo domain (TLD)').clear();
    await page.getByLabel('Piccolo domain (TLD)').fill(PICCOLO_TLD);
    await expect(page.getByLabel('Piccolo domain (TLD)')).toHaveValue(PICCOLO_TLD);
    await page.getByLabel('Use a dedicated portal subdomain').check();
    await page.getByPlaceholder('portal').clear();
    await page.getByPlaceholder('portal').fill(PORTAL_SUBDOMAIN);
    await expect(page.getByPlaceholder('portal')).toHaveValue(PORTAL_SUBDOMAIN);
    await expect(page.getByText(`Full host: ${REMOTE_HOST}`)).toBeVisible();

    await page.getByRole('button', { name: 'Save & run preflight' }).click();

    const deadline = Date.now() + 10 * 60_000;
    let portalIssued = false;
    let portalCerts: any = null;
    while (Date.now() < deadline) {
      portalCerts = await page.evaluate(async () => {
        const res = await fetch('/api/v1/remote/certificates', { credentials: 'same-origin' });
        if (!res.ok) throw new Error('failed to fetch certificates');
        return res.json();
      });
      const portal = (portalCerts.certificates || []).find((c: any) => c.id === 'portal');
      if (portal?.status === 'ok') {
        portalIssued = true;
        break;
      }
      await page.waitForTimeout(1000);
    }
    expect(portalIssued).toBeTruthy();
    const wildcard = (portalCerts?.certificates || []).find((c: any) => c.id === 'wildcard');
    expect(wildcard).toBeFalsy();

    const acceptableHosts = new Set([REMOTE_HOST, `portal.${PICCOLO_TLD}`]);
    await expect.poll(async () => {
      const resp = await page.request.get('/api/v1/remote/status');
      if (!resp.ok()) {
        throw new Error('status fetch failed');
      }
      const body = await resp.json();
      return acceptableHosts.has(body.portal_hostname as string);
    }, { timeout: 15000 }).toBeTruthy();

    const certsResp = await page.request.get('/api/v1/remote/certificates');
    expect(certsResp.ok()).toBeTruthy();
    const certs = await certsResp.json();
    expect((certs.certificates || []).some((c: any) => c.id === 'portal')).toBeTruthy();
  });
});
