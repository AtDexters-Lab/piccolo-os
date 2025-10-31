import { test, expect, request as playwrightRequest } from '@playwright/test';
import { spawn, spawnSync, type ChildProcess } from 'child_process';
import fs from 'fs';
import os from 'os';
import path from 'path';
import net from 'net';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const PICCOLO_BIN = path.resolve(__dirname, '../../piccolod');
const ADMIN_PASSWORD = process.env.PICCOLO_E2E_ADMIN_PASSWORD || 'password';
const REMOTE_ENDPOINT = process.env.PICCOLO_E2E_NEXUS_ENDPOINT || 'wss://stub/connect';
const REMOTE_SECRET = process.env.PICCOLO_E2E_NEXUS_SECRET || 'stub-secret';
const REMOTE_TLD = process.env.PICCOLO_E2E_TLD || 'example.com';
const REMOTE_HOST = `${process.env.PICCOLO_E2E_PORTAL || 'portal-e2e'}.${REMOTE_TLD}`;

function sleep(ms: number) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function getFreePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const srv = net.createServer();
    srv.unref();
    srv.on('error', reject);
    srv.listen(0, () => {
      const address = srv.address();
      if (address && typeof address === 'object') {
        const { port } = address;
        srv.close(() => resolve(port));
      } else {
        srv.close(() => reject(new Error('unable to acquire port')));
      }
    });
  });
}

async function waitForReady(api: any, proc: ChildProcess, timeoutMs = 5000) {
  let exited = false;
  const onExit = () => {
    exited = true;
  };
  proc.once('exit', onExit);
  const deadline = Date.now() + timeoutMs;
  try {
    while (Date.now() < deadline) {
      if (exited) {
        throw new Error('piccolod exited before readiness check completed');
      }
      try {
        const res = await api.get('/api/v1/health/ready');
        if (res.ok()) {
          return;
        }
      } catch (err) {
        // ignore and retry
      }
      await sleep(200);
    }
    throw new Error('timed out waiting for piccolod readiness');
  } finally {
    proc.removeListener('exit', onExit);
  }
}

async function stopServer(proc: ChildProcess | null) {
  if (!proc) return;
  await new Promise((resolve) => {
    const timer = setTimeout(() => {
      proc.kill('SIGKILL');
      resolve(undefined);
    }, 2000);
    proc.once('exit', () => {
      clearTimeout(timer);
      resolve(undefined);
    });
    proc.kill('SIGINT');
  });
}

function unmountControlVolume(stateDir: string) {
  if (!stateDir) return;
  const mountDir = path.join(stateDir, 'mounts', 'control');
  try {
    if (fs.existsSync(mountDir)) {
      spawnSync('fusermount3', ['-u', mountDir], { stdio: 'ignore' });
    }
  } catch (err) {
    // best effort; swallow errors so cleanup can proceed
  }
}

test.describe('remote persistence across restart', () => {
  let server: ChildProcess | null = null;
  let stateDir = '';

  test.afterEach(async () => {
    await stopServer(server);
    server = null;
    if (stateDir) {
      unmountControlVolume(stateDir);
      fs.rmSync(stateDir, { recursive: true, force: true });
      stateDir = '';
    }
  });

  test('remote configuration survives restart', async ({ request }) => {
    stateDir = fs.mkdtempSync(path.join(os.tmpdir(), 'piccolod-remote-persist-'));
    const port1 = await getFreePort();
    server = spawn(PICCOLO_BIN, [], {
      cwd: path.resolve(__dirname, '../..'),
      env: {
        ...process.env,
        PORT: String(port1),
        PICCOLO_STATE_DIR: stateDir,
        PICCOLO_DISABLE_MDNS: '1',
        PICCOLO_REMOTE_FAKE_ACME: '1',
        PICCOLO_NEXUS_USE_STUB: '1',
        PICCOLO_ALLOW_UNMOUNTED_TESTS: '1',
      },
      stdio: 'ignore',
    });

    const api1 = await playwrightRequest.newContext({ baseURL: `http://127.0.0.1:${port1}` });
    await waitForReady(api1, server!);

    await api1.post('/api/v1/auth/setup', { data: { password: ADMIN_PASSWORD } }).catch(() => {});
    await api1.post('/api/v1/crypto/setup', { data: { password: ADMIN_PASSWORD } }).catch(() => {});
    await api1.post('/api/v1/crypto/unlock', { data: { password: ADMIN_PASSWORD } }).catch(() => {});
    const login = await api1.post('/api/v1/auth/login', { data: { username: 'admin', password: ADMIN_PASSWORD } });
    expect(login.ok()).toBeTruthy();
    const csrfToken = await api1.get('/api/v1/auth/csrf').then((r: any) => r.json()).then((j: any) => j.token as string);

    const cfg = await api1.post('/api/v1/remote/configure', {
      headers: { 'X-CSRF-Token': csrfToken },
      data: {
        endpoint: REMOTE_ENDPOINT,
        device_secret: REMOTE_SECRET,
        solver: 'http-01',
        tld: REMOTE_TLD,
        portal_hostname: REMOTE_HOST,
      },
    });
    expect(cfg.ok()).toBeTruthy();

    const statusBefore = await api1.get('/api/v1/remote/status').then((r: any) => r.json());
    expect(statusBefore.enabled).toBeTruthy();
    expect(statusBefore.portal_hostname).toBe(REMOTE_HOST);
    const configPath = path.join(stateDir, 'remote', 'config.json');
    if (fs.existsSync(configPath)) {
      const diskConfigBefore = JSON.parse(fs.readFileSync(configPath, 'utf8'));
      expect(diskConfigBefore.enabled).toBeTruthy();
      expect(diskConfigBefore.portal_hostname).toBe(REMOTE_HOST);
    } else {
      const controlDbPath = path.join(stateDir, 'mounts', 'control', 'control.db');
      expect(fs.existsSync(controlDbPath)).toBeTruthy();
    }

    await api1.dispose();
    await stopServer(server);
    server = null;

    const port2 = await getFreePort();
    server = spawn(PICCOLO_BIN, [], {
      cwd: path.resolve(__dirname, '../..'),
      env: {
        ...process.env,
        PORT: String(port2),
        PICCOLO_STATE_DIR: stateDir,
        PICCOLO_DISABLE_MDNS: '1',
        PICCOLO_REMOTE_FAKE_ACME: '1',
        PICCOLO_NEXUS_USE_STUB: '1',
        PICCOLO_ALLOW_UNMOUNTED_TESTS: '1',
      },
      stdio: 'ignore',
    });

    const api2 = await playwrightRequest.newContext({ baseURL: `http://127.0.0.1:${port2}` });
    await waitForReady(api2, server!);
    await api2.post('/api/v1/crypto/unlock', { data: { password: ADMIN_PASSWORD } }).catch(() => {});
    const statusAfter = await api2.get('/api/v1/remote/status').then((r: any) => r.json());
    expect(statusAfter.enabled).toBeTruthy();
    expect(statusAfter.portal_hostname).toBe(REMOTE_HOST);
    if (fs.existsSync(configPath)) {
      const diskConfigAfter = JSON.parse(fs.readFileSync(configPath, 'utf8'));
      expect(diskConfigAfter.enabled).toBeTruthy();
      expect(diskConfigAfter.portal_hostname).toBe(REMOTE_HOST);
    } else {
      const controlDbPath = path.join(stateDir, 'mounts', 'control', 'control.db');
      expect(fs.existsSync(controlDbPath)).toBeTruthy();
    }
    await api2.dispose();
  });
});
