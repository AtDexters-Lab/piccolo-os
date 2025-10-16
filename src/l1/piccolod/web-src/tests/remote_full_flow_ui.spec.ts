import { test, expect } from '@playwright/test';
import { spawn } from 'child_process';
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';
import os from 'os';

const ADMIN_PASSWORD = process.env.PICCOLO_E2E_ADMIN_PASSWORD || 'password';
const NEXUS_ENDPOINT = process.env.PICCOLO_E2E_NEXUS_ENDPOINT || 'wss://stub/connect';
const NEXUS_SECRET = process.env.PICCOLO_E2E_NEXUS_SECRET || 'stub-secret';
const PICCOLO_TLD = process.env.PICCOLO_E2E_TLD || 'example.com';
const PORTAL_SUBDOMAIN = process.env.PICCOLO_E2E_PORTAL || 'portal-e2e';
const REMOTE_HOST = `${PORTAL_SUBDOMAIN}.${PICCOLO_TLD}`;

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const PICCOLO_BIN = path.resolve(__dirname, '../../piccolod');

test.describe.configure({ timeout: 10 * 60 * 1000 });
test.describe('Remote full flow (UI-driven)', () => {
  let stateDir: string;
  let server: ReturnType<typeof spawn> | undefined;



  test('creates admin, configures remote, verifies HTTPS', async ({ page }) => {
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

    const status = await page.evaluate(async () => {
      const res = await fetch('/api/v1/remote/status', { credentials: 'same-origin' });
      if (!res.ok) throw new Error('failed to fetch status');
      return res.json();
    });
    expect(status.portal_hostname).toBe(REMOTE_HOST);
    expect((status.certificates || []).some((c: any) => c.id === 'portal')).toBeTruthy();
  });
});
