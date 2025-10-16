import { test, expect } from '@playwright/test';

const ADMIN_PASSWORD = process.env.PICCOLO_E2E_ADMIN_PASSWORD || 'password';
const NEXUS_ENDPOINT = process.env.PICCOLO_E2E_NEXUS_ENDPOINT || 'wss://localhost:8443/connect';
const NEXUS_SECRET = process.env.PICCOLO_E2E_NEXUS_SECRET || 'local-secret';
const PICCOLO_TLD = process.env.PICCOLO_E2E_TLD || 'example.com';
const PORTAL_SUBDOMAIN = process.env.PICCOLO_E2E_PORTAL || 'portal-e2e';
const REMOTE_HOST = `${PORTAL_SUBDOMAIN}.${PICCOLO_TLD}`;

test.describe('Remote stack (local Nexus + Pebble) connectivity', () => {
  test.skip(process.env.E2E_REMOTE_STACK !== '1', 'E2E_REMOTE_STACK=1 required');

  test('configure remote and reach enabled state', async ({ request }) => {
    // Setup admin and login
    await request.post('/api/v1/crypto/setup', { data: { password: ADMIN_PASSWORD } }).catch(() => {});
    await request.post('/api/v1/auth/setup', { data: { password: ADMIN_PASSWORD } }).catch(() => {});
    await request.post('/api/v1/crypto/unlock', { data: { password: ADMIN_PASSWORD } }).catch(() => {});
    const login = await request.post('/api/v1/auth/login', { data: { username: 'admin', password: ADMIN_PASSWORD } });
    expect(login.ok()).toBeTruthy();
    const csrf = await request.get('/api/v1/auth/csrf').then(r => r.json()).then(j => j.token as string);

    // Configure remote pointing to local Nexus
    const cfg = await request.post('/api/v1/remote/configure', {
      headers: { 'X-CSRF-Token': csrf },
      data: {
        endpoint: NEXUS_ENDPOINT,
        device_secret: NEXUS_SECRET,
        solver: 'http-01',
        tld: PICCOLO_TLD,
        portal_hostname: REMOTE_HOST,
      },
    });
    expect(cfg.ok(), await cfg.text()).toBeTruthy();

    // Poll status for enabled
    const deadline = Date.now() + 20_000;
    let enabled = false;
    while (Date.now() < deadline) {
      const st = await request.get('/api/v1/remote/status').then(r => r.json());
      if (st.enabled && st.portal_hostname === REMOTE_HOST) { enabled = true; break; }
      await new Promise(r => setTimeout(r, 500));
    }
    expect(enabled, 'remote should enable with local stack').toBeTruthy();
  });
});
