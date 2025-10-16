import { test, expect } from '@playwright/test';

const ADMIN_PASSWORD = process.env.PICCOLO_E2E_ADMIN_PASSWORD || 'password';
const NEXUS_ENDPOINT = process.env.PICCOLO_E2E_NEXUS_ENDPOINT || 'wss://localhost:8443/connect';
const NEXUS_SECRET = process.env.PICCOLO_E2E_NEXUS_SECRET || 'local-secret';
const PICCOLO_TLD = process.env.PICCOLO_E2E_TLD || 'example.com';
const PORTAL_SUBDOMAIN = process.env.PICCOLO_E2E_PORTAL || 'portal-e2e';
const REMOTE_HOST = `${PORTAL_SUBDOMAIN}.${PICCOLO_TLD}`;

test.describe('Remote portal certificate issuance (Pebble HTTP-01)', () => {
  test.skip(process.env.E2E_REMOTE_STACK !== '1', 'E2E_REMOTE_STACK=1 required');

  test('issue portal cert', async ({ request }) => {
    test.setTimeout(3 * 60_000);
    // Setup admin and login
    await request.post('/api/v1/crypto/setup', { data: { password: ADMIN_PASSWORD } }).catch(() => {});
    await request.post('/api/v1/auth/setup', { data: { password: ADMIN_PASSWORD } }).catch(() => {});
    await request.post('/api/v1/crypto/unlock', { data: { password: ADMIN_PASSWORD } }).catch(() => {});
    await request.post('/api/v1/auth/login', { data: { username: 'admin', password: ADMIN_PASSWORD } });
    const csrf = await request.get('/api/v1/auth/csrf').then(r => r.json()).then(j => j.token as string);

    // Configure remote if not yet configured
    await request.post('/api/v1/remote/configure', {
      headers: { 'X-CSRF-Token': csrf },
      data: { endpoint: NEXUS_ENDPOINT, device_secret: NEXUS_SECRET, solver: 'http-01', tld: PICCOLO_TLD, portal_hostname: REMOTE_HOST },
    }).catch(() => {});

    // Poll for portal certificate status ok
    const deadline = Date.now() + 3 * 60_000; // 3 minutes should be plenty for Pebble
    let ok = false, last = '';
    while (Date.now() < deadline) {
      const resp = await request.get('/api/v1/remote/certificates');
      if (!resp.ok()) {
        await new Promise(r => setTimeout(r, 500));
        continue;
      }
      const body = await resp.json();
      const portal = (body.certificates || []).find((c: any) => c.id === 'portal');
      if (portal) last = JSON.stringify(portal);
      if (portal && portal.status === 'ok') { ok = true; break; }
      if (portal) {
        console.log('portal cert status:', portal.status, portal.failure_reason || '');
      }
      await new Promise(r => setTimeout(r, 1000));
    }
    expect(ok, `portal certificate status not ok: ${last}`).toBeTruthy();
  });
});
