import { test, expect } from '@playwright/test';
import { spawn, type ChildProcess } from 'child_process';
import fs from 'fs';
import os from 'os';
import path from 'path';
import net from 'net';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const PICCOLO_BIN = path.resolve(__dirname, '../../piccolod');

function wait(ms: number) {
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

test.describe('First-run setup redirect', () => {
  test.describe.configure({ mode: 'serial' });

  let server: ChildProcess | null = null;
  let baseURL = '';
  let stateDir = '';

  async function startFreshServer(request: any) {
    stateDir = fs.mkdtempSync(path.join(os.tmpdir(), 'piccolo-setup-'));
    const port = await getFreePort();
    baseURL = `http://127.0.0.1:${port}`;

    server = spawn(PICCOLO_BIN, [], {
      cwd: path.resolve(__dirname, '../..'),
      env: {
        ...process.env,
        PORT: String(port),
        PICCOLO_STATE_DIR: stateDir,
        PICCOLO_DISABLE_MDNS: '1',
        PICCOLO_REMOTE_FAKE_ACME: '1',
        PICCOLO_NEXUS_USE_STUB: '1',
      },
      stdio: 'ignore',
    });

    const deadline = Date.now() + 15000;
    while (Date.now() < deadline) {
      try {
        const res = await request.get(`${baseURL}/api/v1/auth/initialized`, { failOnStatusCode: false });
        if (res.status() >= 200) {
          return;
        }
      } catch (err) {
        // ignore and retry
      }
      await wait(200);
    }
    throw new Error('timed out waiting for fresh piccolod instance');
  }

  async function stopServer() {
    if (server) {
      await new Promise((resolve) => {
        const timer = setTimeout(() => {
          server?.kill('SIGKILL');
          resolve(undefined);
        }, 2000);
        server!.once('exit', () => {
          clearTimeout(timer);
          resolve(undefined);
        });
        server!.kill('SIGINT');
      });
      server = null;
    }
    if (stateDir) {
      fs.rmSync(stateDir, { recursive: true, force: true });
      stateDir = '';
    }
  }

  test.afterEach(async () => {
    await stopServer();
  });

  test('setup route is accessible on a fresh instance', async ({ page, request }) => {
    await startFreshServer(request);
    await page.context().clearCookies();
    const init = await request.get(`${baseURL}/api/v1/auth/initialized`).then(r => r.json());
    expect(init.initialized).toBeFalsy();
    await page.goto(`${baseURL}/#/setup`);
    await expect(page.getByRole('heading', { level: 2, name: 'Create Admin' })).toBeVisible({ timeout: 10000 });
  });

  test('login shows unlock prompt before setup', async ({ page, request }) => {
    await startFreshServer(request);
    await page.context().clearCookies();
    await page.goto(`${baseURL}/#/login`);
    await expect(page.getByPlaceholder('admin password')).toBeVisible({ timeout: 10000 });
    await expect(page.getByText(/is locked/i)).toBeVisible({ timeout: 10000 });
  });

});
