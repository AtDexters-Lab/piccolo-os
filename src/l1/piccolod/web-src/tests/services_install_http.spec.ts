import { test, expect } from '@playwright/test';

// Installs a minimal HTTP app (nginx) via the real API and verifies
// the managed proxy serves it locally. Requires Podman with network access.
// Fails fast with clear messaging if container runtime is unavailable.

test.describe('Service install (nginx) and local proxy', () => {
  const adminPass = process.env.PICCOLO_E2E_ADMIN_PASSWORD || 'password';
  const appName = process.env.PICCOLO_E2E_APP || 'nginxdemo';

  test('install, reach via proxy, uninstall', async ({ request }) => {
    // Admin + session
    await request.post('/api/v1/auth/setup', { data: { password: adminPass } }).catch(() => {});
    const login = await request.post('/api/v1/auth/login', { data: { username: 'admin', password: adminPass } });
    expect(login.ok()).toBeTruthy();
    const csrf = await request.get('/api/v1/auth/csrf').then(r => r.json()).then(j => j.token as string);

    // Ensure crypto initialized and unlocked
    const st = await request.get('/api/v1/crypto/status').then(r => r.json());
    if (!st.initialized) {
      const setup = await request.post('/api/v1/crypto/setup', { headers: { 'X-CSRF-Token': csrf }, data: { password: adminPass } });
      expect(setup.ok()).toBeTruthy();
    }
    const st2 = await request.get('/api/v1/crypto/status').then(r => r.json());
    if (st2.locked) {
      const u = await request.post('/api/v1/crypto/unlock', { headers: { 'X-CSRF-Token': csrf }, data: { password: adminPass } });
      expect(u.ok()).toBeTruthy();
    }

    // Install nginx (simple HTTP listener)
    const yaml = [
      `name: ${appName}`,
      'image: docker.io/library/nginx:alpine',
      'listeners:',
      '  - name: http',
      '    guest_port: 80',
      '    flow: tcp',
      '    protocol: http',
      ''
    ].join('\n');

    const install = await request.post('/api/v1/apps', {
      headers: { 'Content-Type': 'application/x-yaml', 'X-CSRF-Token': csrf },
      data: yaml,
    });
    // If Podman is missing or pull fails, this will be 500 with error text.
    expect(install.ok(), `install failed: ${await install.text()}`).toBeTruthy();

    // Discover proxy port and verify HTTP 200
    const deadline = Date.now() + 120_000; // up to 2 minutes for image pull
    let publicPort = 0;
    while (Date.now() < deadline) {
      const services = await request.get('/api/v1/services').then(r => r.json());
      const svc = (services.services || []).find((s: any) => s.app === appName && s.name === 'http');
      if (svc && svc.public_port) { publicPort = svc.public_port; break; }
      await new Promise(r => setTimeout(r, 1000));
    }
    expect(publicPort, 'allocated public port').toBeGreaterThan(0);

    const resp = await fetch(`http://localhost:${publicPort}`, { method: 'GET' });
    expect(resp.ok).toBeTruthy();

    // Cleanup (best-effort)
    await request.delete(`/api/v1/apps/${appName}?purge=true`, { headers: { 'X-CSRF-Token': csrf } });
  });
});

