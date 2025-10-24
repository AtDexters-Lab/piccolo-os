import { defineConfig, devices } from '@playwright/test';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

// Build the server command dynamically so we can switch between
// a stubbed-remote lane (default, offline) and a full-remote lane
// (E2E_REMOTE_STACK=1), keeping everything else identical.
const stubEnv = 'PICCOLO_DISABLE_MDNS=1 PICCOLO_NEXUS_USE_STUB=1 PICCOLO_REMOTE_FAKE_ACME=1';
const baseEnv = 'PICCOLO_DISABLE_MDNS=1';
const stateEnv = 'PORT=8080 PICCOLO_STATE_DIR=.e2e-state';
const stackRoot = path.resolve(__dirname, '../tools/remote-stack');
const pebbleCA = path.resolve(stackRoot, 'pebble.minica.pem');
const combinedCa = path.resolve(stackRoot, 'combined-ca.pem');
const acmeEnv = 'PICCOLO_ACME_DIR_URL=https://localhost:14000/dir';
const caEnv = `LEGO_CA_CERTIFICATES=${pebbleCA}`;
const sslEnv = `SSL_CERT_FILE=${combinedCa}`;
const serverCmd = process.env.E2E_REMOTE_STACK === '1'
  ? `bash -c "rm -rf .e2e-state && ${baseEnv} ${stateEnv} ${acmeEnv} ${caEnv} ${sslEnv} ./piccolod"`
  : `bash -c "rm -rf .e2e-state && ${stubEnv} ${stateEnv} ./piccolod"`;

const lockSensitiveSpecs = [
  'tests/catalog_real.spec.ts',
  'tests/crypto_real.spec.ts',
  'tests/crypto_recovery_real.spec.ts',
  'tests/services_install_http.spec.ts',
  'tests/unlock_first_real.spec.ts',
  'tests/crypto_ui_real.spec.ts',
  'tests/auth_csrf_real.spec.ts',
  'tests/dashboard_real.spec.ts',
  'tests/services_real.spec.ts',
];

export default defineConfig({
  testDir: './tests',
  globalSetup: './global-setup.ts',
  globalTeardown: './global-teardown.ts',
  use: {
    baseURL: process.env.BASE_URL || 'http://localhost:8080',
    headless: true,
    viewport: { width: 1280, height: 800 },
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
    trace: 'retain-on-failure',
  },
  reporter: [
    ['list'],
    ['json', { outputFile: 'test-results/report.json' }],
    // HTML report must not live inside the tests output folder to avoid clashes
    ['html', { open: 'never', outputFolder: 'playwright-report' }],
  ],
  webServer: {
    command: serverCmd,
    port: 8080,
    timeout: 120000,
    reuseExistingServer: false,
    cwd: path.resolve(__dirname, '..'),
  },
  projects: [
    { name: 'chromium', testMatch: ['tests/**/*.spec.ts'], testIgnore: ['tests/mobile.spec.ts', ...lockSensitiveSpecs], use: { ...devices['Desktop Chrome'] } },
    { name: 'chromium-lock', testMatch: lockSensitiveSpecs, use: { ...devices['Desktop Chrome'] }, workers: 1, retries: 0, fullyParallel: false, dependencies: ['chromium', 'mobile-chromium'] },
    { name: 'mobile-chromium', testMatch: ['tests/mobile.spec.ts'], use: { ...devices['Pixel 5'] } },
  ],
});
